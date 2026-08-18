package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// spr sitemap.
//
// Two commands wearing one name, because the site has two kinds of sitemap and
// they cost three orders of magnitude apart.
//
// The eight static maps are eight requests for every journal, series and
// collection Springer publishes. Those just run.
//
// The dated shards are 10,408 requests and five and three quarter hours, and
// piping them into spr work is a crawl of unknown size. Those are billed from
// the index that was just fetched, and above a hundred shards the command
// stops and asks to be told the window.

// walkBillAt is how many shards a walk may cover before it prints its bill and
// gets on with it. Ten shards is twenty seconds and up to fifty thousand urls,
// which is the point at which somebody watching a quiet terminal starts to
// wonder.
const walkBillAt = 10

func sitemapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sitemap",
		Short: "Enumerate what the site publishes, from its own sitemaps",
		Long: "sitemap reads the maps Springer publishes about itself, which are the only complete\n" +
			"enumeration of what is on the site.\n\n" +
			"With no flags it fetches the index and says what is in it. --static fetches one of the\n" +
			"eight maps that are not dated shards, and those eight are the whole point: eight\n" +
			"requests for every journal, series and collection there is. --list prints the child\n" +
			"sitemap urls. --since, --until and --kind walk the shards and print the urls in them,\n" +
			"one per line, which is what makes spr sitemap | spr work a pipeline.\n\n" +
			"The date in a shard's file name is a bucket and not a publication date. The first of\n" +
			"January is where everything known only to its year is filed, which is why 2020-01-01\n" +
			"has 66 shards and roughly 330,000 works, and why the entries inside the first of them\n" +
			"carry 173 different lastmod values of which none is 2020-01-01. This command calls that\n" +
			"field bucket and there is no flag that turns it into a date.\n\n" +
			"A walk of everything is 10,408 requests and close to six hours. Above a hundred shards\n" +
			"the bill is printed and nothing is fetched until you pass --yes, and the bill is\n" +
			"computed from the index in hand rather than from a number written into this help.",
		Args: cobra.NoArgs,
		Example: "  spr sitemap\n" +
			"  spr sitemap --list\n" +
			"  spr sitemap --static journals\n" +
			"  spr sitemap --kind article --since 2026-08-01\n" +
			"  spr sitemap --kind book --since 2026-01-01 | spr work\n" +
			"  spr sitemap --all --yes --resume",
	}

	var (
		list   bool
		static string
		kinds  []string
		since  string
		until  string
		all    bool
		yes    bool
		limit  int
		resume bool
	)

	f := cmd.Flags()
	f.BoolVar(&list, "list", false, "print the url of every child sitemap rather than what is in them")
	f.StringVar(&static, "static", "", "one of the static maps: "+strings.Join(spr.StaticNames, ", "))
	f.StringArrayVar(&kinds, "kind", nil, "keep only these kinds, repeatable: "+strings.Join(spr.EntryKinds, ", "))
	f.StringVar(&since, "since", "", "earliest bucket to read: 2026, 2026-08 or 2026-08-01")
	f.StringVar(&until, "until", "", "latest bucket to read, at the same three precisions")
	f.BoolVar(&all, "all", false, "walk every shard in the index, which needs --yes")
	f.BoolVar(&yes, "yes", false, "proceed with a walk that was billed rather than stopping")
	f.IntVar(&limit, "limit", 0, "stop after this many urls")
	f.BoolVar(&resume, "resume", false, "skip the shards an earlier run of this same selection finished")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		errw := cmd.ErrOrStderr()
		note := func(s string) { fmt.Fprintln(errw, "sitemap: "+s) }

		if static != "" {
			if !spr.KnownStatic(static) {
				return exit(CodeUsage, fmt.Errorf("--static %q is not one of %s", static, strings.Join(spr.StaticNames, ", ")))
			}
			if list || all || since != "" || until != "" {
				return exit(CodeUsage, errors.New("--static reads one map and the shard flags walk the index, so pick one"))
			}
			return runStatic(cmd, static, kinds, limit, note)
		}

		from, err := parseSince(since)
		if err != nil {
			return err
		}
		to, err := parseUntil(until)
		if err != nil {
			return err
		}
		if !from.IsZero() && !to.IsZero() && from.After(to) {
			return exit(CodeUsage, fmt.Errorf("--since %s is after --until %s", since, until))
		}
		for _, k := range kinds {
			if _, ok := spr.NormalizeKind(k); !ok {
				return exit(CodeUsage, fmt.Errorf("--kind %q is not one of %s", k, strings.Join(spr.EntryKinds, ", ")))
			}
		}
		if list && len(kinds) > 0 {
			return exit(CodeUsage, errors.New("--kind filters the urls inside the shards and --list prints the shards, so --list has nothing to filter"))
		}
		if limit < 0 {
			return exit(CodeUsage, fmt.Errorf("--limit %d asks for no urls", limit))
		}
		if resume && g.noCache {
			return exit(CodeUsage, errors.New("--resume keeps its state in the cache directory and --no-cache turned that off"))
		}

		c := client(errw)
		idx, err := c.SitemapIndex(cmd.Context())
		if err != nil {
			return exit(CodeTransport, err)
		}
		if len(idx.Children) == 0 {
			return exit(CodeNoData, fmt.Errorf("%s holds no child sitemaps, which is a shape change rather than an empty site", idx.URL))
		}

		pace := effectivePace()
		selected := idx.Select(from, to)
		if list {
			return printList(out, idx, selected, limit)
		}

		walking := all || since != "" || until != "" || len(kinds) > 0
		if !walking {
			printIndex(out, idx, pace)
			return nil
		}
		if len(selected) == 0 {
			oldest, newest := idx.Span()
			return exit(CodeNoData, fmt.Errorf("no shard is bucketed in that window, and the index runs %s to %s", oldest, newest))
		}

		// The bill, and the two things it can lead to. Above the threshold the
		// walk stops and asks to be told the window, because the difference
		// between the last three days and everything since 1850 is one flag.
		// Above ten shards it is printed and the walk proceeds, because a bill
		// that interrupts a pipeline is worse than a long wait.
		cost := spr.EnumCost(len(selected), pace)
		switch {
		case !yes && (all || len(selected) > spr.ShardThreshold):
			printCost(errw, cost, pace, len(idx.Children), all)
			return exit(CodeUsage, nil)
		case len(selected) > walkBillAt:
			printCost(errw, cost, pace, len(idx.Children), all)
		}

		return runWalk(cmd, c, idx, selected, kinds, limit, resume, note)
	}
	return cmd
}

// runStatic fetches one of the eight maps that are not dated shards.
func runStatic(cmd *cobra.Command, name string, kinds []string, limit int, note func(string)) error {
	out := cmd.OutOrStdout()
	c := client(cmd.ErrOrStderr())

	set, err := c.Static(cmd.Context(), name, note)
	if err != nil {
		return exit(CodeTransport, err)
	}
	if len(kinds) > 0 {
		set.Entries = filterKinds(set.Entries, kinds)
	}
	if limit > 0 && len(set.Entries) > limit {
		set.Entries = set.Entries[:limit]
	}
	if len(set.Entries) == 0 {
		return exit(CodeNoData, nil)
	}

	if g.format == "json" {
		return encode(out, set)
	}
	for _, e := range set.Entries {
		fmt.Fprintln(out, e.URL)
	}
	note(fmt.Sprintf("%s: %s urls from %d %s", set.Name, group(len(set.Entries)), len(set.Maps), plural(len(set.Maps), "map")))
	return nil
}

// runWalk reads the selected shards and prints their urls as they arrive.
//
// The urls go to stdout as one per line, or as one json object per line, and
// everything else goes to stderr. That is what lets the output be a pipe: a
// walk of a thousand shards prints its first url in two seconds and its summary
// half an hour later, and neither of them is buffered waiting for the other.
func runWalk(cmd *cobra.Command, c *spr.Client, idx *spr.Sitemap, selected []spr.Child, kinds []string, limit int, resume bool, note func(string)) error {
	out := cmd.OutOrStdout()
	asJSON := g.format == "json"

	var state *spr.Resume
	if resume {
		key := spr.ResumeKey(idx.URL, selectionOf(selected), strings.Join(kinds, ","))
		s, err := spr.OpenResume(g.cache, key)
		if err != nil {
			return exit(CodeUsage, err)
		}
		defer func() { _ = s.Close() }()
		state = s
		if n := s.Count(); n > 0 {
			note(fmt.Sprintf("resuming, %s of %s shards were finished by an earlier run", group(n), group(len(selected))))
		} else {
			note(fmt.Sprintf("no earlier run of this selection to resume, so this is all %s shards", group(len(selected))))
		}
	}

	enc := json.NewEncoder(out)
	started := time.Now()
	stats, err := c.Enumerate(cmd.Context(), selected, spr.EnumOptions{
		Kinds:  kinds,
		Limit:  limit,
		Resume: state,
		Note:   note,
		Each: func(e spr.Entry) error {
			if asJSON {
				return enc.Encode(e)
			}
			_, err := fmt.Fprintln(out, e.URL)
			return err
		},
	})
	// The summary is printed whenever a shard was read, and not only when
	// something matched. A walk that read a thousand shards and kept nothing has
	// spent half an hour, and exit code 3 on its own does not say whether it
	// found nothing or fetched nothing.
	if stats != nil && stats.Read+stats.Skipped > 0 {
		note(summary(stats, time.Since(started)))
	}
	if err != nil {
		return exit(CodeTransport, err)
	}
	if stats.Matched == 0 {
		return exit(CodeNoData, nil)
	}
	if stats.Failed > 0 {
		// Some of what was asked for was not read. The urls that did arrive are
		// on stdout and correct, and the exit code is what says the walk has a
		// hole in it, since nothing in the output can show an absence.
		return exit(CodeTransport, fmt.Errorf("%d of %d shards did not answer, so this walk is incomplete", stats.Failed, stats.Shards))
	}
	return nil
}

// summary is the line a walk ends on: what it cost and what it found.
func summary(s *spr.EnumStats, took time.Duration) string {
	parts := []string{fmt.Sprintf("%s of %s shards read", group(s.Read), group(s.Shards))}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%s skipped as already done", group(s.Skipped)))
	}
	parts = append(parts, fmt.Sprintf("%s urls", group(s.Entries)))
	if s.Matched != s.Entries {
		parts = append(parts, fmt.Sprintf("%s kept", group(s.Matched)))
	}
	if s.Empty > 0 {
		parts = append(parts, fmt.Sprintf("%s empty", group(s.Empty)))
	}
	parts = append(parts, humanBytes(int64(s.Bytes)), estimate(took))
	return strings.Join(parts, ", ")
}

// printIndex is what spr sitemap with no flags prints: one request, and what it
// bought.
func printIndex(out io.Writer, idx *spr.Sitemap, pace time.Duration) {
	line := field(out)
	line("index", idx.URL)
	line("children", fmt.Sprintf("%s child sitemaps", group(len(idx.Children))))

	shapes := idx.Shapes()
	line("buckets", fmt.Sprintf("%s named for a day, %s for a month, %s for a year",
		group(shapes[spr.PrecisionDay]), group(shapes[spr.PrecisionMonth]), group(shapes[spr.PrecisionYear])))
	oldest, newest := idx.Span()
	line("span", oldest.String()+" to "+newest.String())

	// The one thing about this index somebody is most likely to reach for and
	// most likely to be wrong about.
	line("lastmod", fmt.Sprintf("on the %s day shards only, where it restates the bucket", group(shapes[spr.PrecisionDay])))
	line("bucket", "where a record is filed, and not when it was published")
	line("static", strings.Join(spr.StaticNames, ", ")+", read with --static")

	cost := spr.EnumCost(len(idx.Children), pace)
	line("full walk", fmt.Sprintf("%s requests at %s, %s, bounded above by %s urls and %s",
		group(cost.Requests), pace, estimate(cost.Duration), group(cost.MaxURLs), humanBytes(cost.MaxBytes)))
	printEnvelope(out, idx.Envelope, false)
}

// printList prints the child sitemap urls, which is one request and the whole
// shape of the site.
func printList(out io.Writer, idx *spr.Sitemap, selected []spr.Child, limit int) error {
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	if len(selected) == 0 {
		return exit(CodeNoData, nil)
	}
	if g.format == "json" {
		return encode(out, struct {
			URL      string       `json:"url"`
			Children []spr.Child  `json:"children"`
			Envelope spr.Envelope `json:"envelope"`
		}{idx.URL, selected, idx.Envelope})
	}
	for _, c := range selected {
		fmt.Fprintln(out, c.URL)
	}
	return nil
}

// printCost is the bill, and it is printed before anything is fetched.
//
// Every number in it comes from the index that was just fetched. A guard that
// carried a compiled in figure would be right on the day it was written and
// quietly wrong for the rest of the tool's life, since the index grows daily.
func printCost(out io.Writer, cost spr.Cost, pace time.Duration, children int, all bool) {
	fmt.Fprintf(out, "sitemap: %s child %s at %s pace is %s\n",
		group(cost.Requests), plural(cost.Requests, "sitemap"), pace, estimate(cost.Duration))
	fmt.Fprintf(out, "         a full shard is %s urls and %s, so this is bounded above by %s urls and %s\n",
		group(spr.ShardCeiling), humanBytes(spr.ShardBytes), group(cost.MaxURLs), humanBytes(cost.MaxBytes))
	if all {
		fmt.Fprintf(out, "         that is the whole index, all %s %s of it\n", group(children), plural(children, "shard"))
	}
	fmt.Fprintln(out, "         narrow it with --since and --until, or pass --yes to proceed")
}

// parseSince reads a window edge as the first instant it covers.
func parseSince(s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, nil
	}
	d, err := spr.ParseDate(s)
	if err != nil {
		return time.Time{}, exit(CodeUsage, fmt.Errorf("--since %q is not a year, a month or a day", s))
	}
	return d.Value, nil
}

// parseUntil reads the other edge as the last instant it covers.
//
// The precision is why this is not the same function as parseSince. --until
// 2026 means the end of 2026 and not the first of January, and reading both
// edges the same way would turn a whole year into one day without saying so.
func parseUntil(s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, nil
	}
	d, err := spr.ParseDate(s)
	if err != nil {
		return time.Time{}, exit(CodeUsage, fmt.Errorf("--until %q is not a year, a month or a day", s))
	}
	switch d.Precision {
	case spr.PrecisionYear:
		return d.Value.AddDate(1, 0, -1), nil
	case spr.PrecisionMonth:
		return d.Value.AddDate(0, 1, -1), nil
	default:
		return d.Value, nil
	}
}

// selectionOf names a selection for the resume key. The first and last shard
// plus the count identify a window exactly, and they do it without depending on
// how the window was spelled on the command line.
func selectionOf(selected []spr.Child) string {
	if len(selected) == 0 {
		return "empty"
	}
	return fmt.Sprintf("%s..%s/%d", selected[0].Name, selected[len(selected)-1].Name, len(selected))
}

func filterKinds(entries []spr.Entry, kinds []string) []spr.Entry {
	want := map[string]bool{}
	for _, k := range kinds {
		if norm, ok := spr.NormalizeKind(k); ok {
			want[norm] = true
		}
	}
	out := entries[:0]
	for _, e := range entries {
		if want[e.Kind] {
			out = append(out, e)
		}
	}
	return out
}

// effectivePace is the pace the client will actually use, floor included, so
// that a bill computed with --pace 100ms is not four times more optimistic than
// the run it is billing.
func effectivePace() time.Duration {
	if g.pace < spr.PaceFloor {
		return spr.PaceFloor
	}
	return g.pace
}
