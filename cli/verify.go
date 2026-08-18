package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// spr verify.
//
// The ledger says what each capture yielded on the day it was measured. This
// command reads the same pages again and says whether they still do.
//
// Which pages it read is stated on every run and repeated on every finding.
// That is not decoration. On the last tool built this way, a capture had lost a
// region in the page cache and not in reality, the report said the extractor
// had regressed, and it took a live refetch to prove that nothing had changed.
// The lesson was not to fix the cache, it was to say which one you read, every
// time.

func verifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Read the ledger's pages again and say whether they still read the same",
		Long: "verify reads the fourteen pages the capture ledger is built from and compares what\n" +
			"comes out with what was recorded.\n\n" +
			"It reads the page cache by default and makes no request at all. --live refetches\n" +
			"every page instead, which is the only way to tell a site that changed apart from a\n" +
			"cache entry that went stale, and every line of output says which of the two it read.\n\n" +
			"Fewer fields set is a regression and is this tool's fault. More fields set is an\n" +
			"improvement and is reported until somebody records it. A page carrying one fewer\n" +
			"vocabulary, or its two access declarations disagreeing, is the site restating a fact\n" +
			"and needs a person. A change in unread regions is drift, which is Springer shipping a\n" +
			"component, and is reported without failing.\n\n" +
			"--vocab is the other half: for every page, what each vocabulary claims about the facts\n" +
			"more than one of them states. They agreed on all fourteen, which is exactly why a\n" +
			"disagreement is worth printing.",
		Args: cobra.NoArgs,
		Example: "  spr verify\n" +
			"  spr verify --live\n" +
			"  spr verify --vocab\n" +
			"  spr verify --capture article_oa --live",
	}

	var (
		live    bool
		vocab   bool
		capture []string
	)

	f := cmd.Flags()
	f.BoolVar(&live, "live", false, "refetch every page instead of reading the page cache")
	f.BoolVar(&vocab, "vocab", false, "cross-check what each vocabulary claims about the same fact")
	f.StringSliceVar(&capture, "capture", nil, "only these captures, by name, repeatable")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		errw := cmd.ErrOrStderr()

		wanted, err := chosenCaptures(capture)
		if err != nil {
			return exit(CodeUsage, err)
		}
		if g.noCache && !live {
			return exit(CodeUsage, errNoCacheToVerify)
		}

		c := client(errw)
		source := "the page cache at " + g.cache
		var store *spr.Cache
		if live {
			// The cache comes out of the client rather than being left in to
			// answer the very question being asked. It is held to one side and
			// written back to, so that a later run without --live reads the
			// pages this one saw rather than nothing at all.
			store, c.Cache = c.Cache, nil
			source = "a live refetch"
		}

		if vocab {
			return runVocab(cmd, out, c, store, wanted, live, source)
		}
		return runVerify(cmd, out, c, store, wanted, live, source)
	}
	return cmd
}

var errNoCacheToVerify = fmt.Errorf(
	"verify reads the page cache and --no-cache turned it off, so there is nothing to read; pass --live to refetch instead")

// chosenCaptures resolves the --capture names, or returns all of them.
func chosenCaptures(names []string) ([]spr.Capture, error) {
	if len(names) == 0 {
		return spr.Captures, nil
	}
	out := make([]spr.Capture, 0, len(names))
	for _, n := range names {
		c, ok := spr.CaptureNamed(n)
		if !ok {
			return nil, fmt.Errorf("no capture is named %q; the ledger holds %s", n, joinCaptureNames())
		}
		out = append(out, c)
	}
	return out, nil
}

func joinCaptureNames() string {
	names := make([]string, len(spr.Captures))
	for i, c := range spr.Captures {
		names[i] = strings.TrimSuffix(c.File, ".html")
	}
	return strings.Join(names, ", ")
}

// verifyResult is one capture's outcome, and is what -o json emits.
type verifyResult struct {
	Capture  string          `json:"capture"`
	URL      string          `json:"url"`
	Source   string          `json:"source"`
	Verdict  string          `json:"verdict"`
	Findings []string        `json:"findings,omitempty"`
	Diff     *spr.LedgerDiff `json:"diff,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func runVerify(cmd *cobra.Command, out io.Writer, c *spr.Client, store *spr.Cache, wanted []spr.Capture, live bool, source string) error {
	recorded := spr.Ledger()
	results := make([]verifyResult, 0, len(wanted))
	counts := map[string]int{}

	for _, capture := range wanted {
		r := verifyResult{Capture: capture.File, URL: capture.URL, Source: source}

		resp, err := fetchForVerify(cmd, c, store, capture, live)
		switch {
		case err != nil:
			r.Verdict, r.Error = "unread", err.Error()
		default:
			got, err := spr.ReadCapture(resp, capture)
			if err != nil {
				r.Verdict, r.Error = "unread", err.Error()
				break
			}
			want, ok := recorded[capture.File]
			if !ok {
				r.Verdict, r.Error = "unread", "not in the ledger"
				break
			}
			d := spr.CompareLedger(want, got)
			r.Verdict, r.Findings = string(d.Verdict()), d.Lines()
			if d.Verdict() != spr.VerdictOK {
				diff := d
				r.Diff = &diff
			}
		}
		counts[r.Verdict]++
		results = append(results, r)
	}

	if g.format == "json" {
		return finishVerify(encode(out, results), counts)
	}

	fmt.Fprintf(out, "%-10s %s\n", "source", source)
	fmt.Fprintf(out, "%-10s %d %s recorded in the ledger this binary was built with\n",
		"ledger", len(spr.Captures), plural(len(spr.Captures), "capture"))
	fmt.Fprintln(out)
	for _, r := range results {
		fmt.Fprintf(out, "%-11s %s\n", r.Verdict, r.Capture)
		if r.Error != "" {
			fmt.Fprintf(out, "            %s, read from %s\n", r.Error, source)
		}
		for _, line := range r.Findings {
			fmt.Fprintf(out, "            %s, read from %s\n", line, source)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, verdictSummary(counts))
	return finishVerify(nil, counts)
}

// finishVerify turns the counts into an exit code. Drift never fails, because a
// component appearing on a page is news about the site rather than a bug here.
func finishVerify(err error, counts map[string]int) error {
	if err != nil {
		return err
	}
	switch {
	case counts[string(spr.VerdictRegression)] > 0,
		counts[string(spr.VerdictChanged)] > 0,
		counts[string(spr.VerdictImprovement)] > 0:
		return exit(CodeDrift, nil)
	case counts["unread"] > 0 && counts[string(spr.VerdictOK)] == 0:
		return exit(CodeNoData, nil)
	}
	return nil
}

func verdictSummary(counts map[string]int) string {
	order := []string{
		string(spr.VerdictOK), string(spr.VerdictDrift), string(spr.VerdictImprovement),
		string(spr.VerdictChanged), string(spr.VerdictRegression), "unread",
	}
	var parts []string
	for _, k := range order {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return "nothing was checked"
	}
	return strings.Join(parts, ", ")
}

// fetchForVerify reads one capture's page, from the cache or from the site.
//
// The cache path deliberately does not fall through to a request. A verify run
// that quietly fetched what it could not find would be a verify run that says
// cache in its header and means something else.
func fetchForVerify(cmd *cobra.Command, c *spr.Client, store *spr.Cache, capture spr.Capture, live bool) (*spr.Response, error) {
	if !live {
		resp, ok := c.Cache.Get(capture.URL)
		if !ok {
			return nil, fmt.Errorf("not in the cache; read it once, or pass --live")
		}
		resp.Status = spr.Classify(resp.Code, resp.Header, resp.Body, spr.KindHTML)
		return resp, nil
	}
	resp, err := c.Get(cmd.Context(), capture.URL, spr.KindHTML)
	if err != nil {
		return nil, err
	}
	if resp.Status != spr.StatusOK && resp.Status != spr.StatusRestricted {
		return nil, fmt.Errorf("the site answered %s", resp.Status)
	}
	// The page the run just read is worth keeping, so that the next run without
	// --live has something to compare and does not report fourteen pages unread.
	_ = store.Put(resp)
	return resp, nil
}

// vocabResult is one capture's vocabulary cross-check.
type vocabResult struct {
	Capture string             `json:"capture"`
	Source  string             `json:"source"`
	Facts   []spr.VocabReading `json:"facts,omitempty"`
	Error   string             `json:"error,omitempty"`
}

func runVocab(cmd *cobra.Command, out io.Writer, c *spr.Client, store *spr.Cache, wanted []spr.Capture, live bool, source string) error {
	results := make([]vocabResult, 0, len(wanted))
	disagreements, checked := 0, 0

	for _, capture := range wanted {
		r := vocabResult{Capture: capture.File, Source: source}
		resp, err := fetchForVerify(cmd, c, store, capture, live)
		if err != nil {
			r.Error = err.Error()
			results = append(results, r)
			continue
		}
		rows, err := spr.ReadVocabularies(resp)
		if err != nil {
			r.Error = err.Error()
			results = append(results, r)
			continue
		}
		r.Facts = rows
		for _, row := range rows {
			checked++
			if !row.Agree {
				disagreements++
			}
		}
		results = append(results, r)
	}

	if g.format == "json" {
		if err := encode(out, results); err != nil {
			return err
		}
		if disagreements > 0 {
			return exit(CodeDrift, nil)
		}
		return nil
	}

	fmt.Fprintf(out, "%-10s %s\n", "source", source)
	fmt.Fprintln(out)
	for _, r := range results {
		fmt.Fprintln(out, r.Capture)
		if r.Error != "" {
			fmt.Fprintf(out, "  %s, read from %s\n", r.Error, source)
			continue
		}
		if len(r.Facts) == 0 {
			fmt.Fprintln(out, "  no fact on this page is stated by more than one vocabulary")
			continue
		}
		for _, row := range r.Facts {
			mark := "agree"
			if !row.Agree {
				mark = "DISAGREE"
			}
			fmt.Fprintf(out, "  %-9s %-12s %s\n", mark, row.Fact, claimLine(row.Claims))
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%d %s across %d %s, %s\n",
		checked, plural(checked, "fact"), len(results), plural(len(results), "page"),
		countDisagreements(disagreements))
	if disagreements > 0 {
		return exit(CodeDrift, nil)
	}
	return nil
}

func countDisagreements(n int) string {
	if n == 0 {
		return "and every one of them agrees"
	}
	return fmt.Sprintf("and %d of them do not agree, which needs a person", n)
}

// claimLine writes what each vocabulary said, in name order so that two runs
// over one page print the same line.
func claimLine(claims map[string]string) string {
	keys := make([]string, 0, len(claims))
	for k := range claims {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%q", k, claims[k])
	}
	return strings.Join(parts, "  ")
}
