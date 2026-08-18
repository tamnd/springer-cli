package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

func workCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work <doi, url or path>",
		Short: "Read one article, chapter, protocol or reference work entry",
		Long: "work reads a single work page and prints the record it produced, along with the envelope\n" +
			"that says where each field came from.\n\n" +
			"A DOI is enough. The prefix does not say what a work is, so the suffix is used to order\n" +
			"the paths it could live under and they are tried until one answers, which costs one\n" +
			"request per miss and usually costs none.\n\n" +
			"A restricted page is read rather than refused. Everything except the body is in the head\n" +
			"of a paywalled page, so the record is printed, the body is named in the envelope with the\n" +
			"page's own sentence for why, and the exit code says access was withheld.",
		Args: cobra.ExactArgs(1),
		Example: "  spr work 10.1007/s10994-021-05946-3\n" +
			"  spr work /chapter/10.1007/978-3-030-58607-2_1\n" +
			"  spr work -o json 10.1007/s10994-021-05946-3 | jq .envelope",
	}

	var (
		text     bool
		envelope bool
	)
	cmd.Flags().BoolVar(&text, "text", false, "print the body text of each section as well as the tree")
	cmd.Flags().BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		c := client(cmd.ErrOrStderr())
		resp, err := findWork(cmd.Context(), c, args[0])
		if err != nil {
			return err
		}

		if resp.Status == spr.StatusChallenged {
			fmt.Fprintln(cmd.ErrOrStderr(), explain(resp.Status))
			return exit(CodeChallenged, nil)
		}

		w, err := spr.ExtractWork(resp)
		if err != nil {
			if errors.Is(err, spr.ErrNotAWork) {
				return exit(CodeUsage, fmt.Errorf("%s is not a work page, so there is no work record to read from it", resp.URL))
			}
			return exit(CodeNoData, err)
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(w); err != nil {
				return exit(CodeTransport, err)
			}
			return statusExit(resp.Status)
		}

		printWork(out, w, text, envelope)
		return statusExit(resp.Status)
	}
	return cmd
}

// findWork turns one argument into one fetched page.
//
// A url or a path is fetched as given, because the caller already said where to
// look. A DOI is different: the registrant prefix says who issued it and nothing
// about what it is, so the candidate paths are tried in the order the suffix
// suggests and the first page that is not a 404 wins. Only a 404 moves on to the
// next candidate. A challenge or a restricted page is an answer about this work
// and trying the other four paths would just ask the same question four more
// times.
func findWork(ctx context.Context, c *spr.Client, arg string) (*spr.Response, error) {
	if strings.Contains(arg, "/") && (strings.HasPrefix(arg, "/") || strings.Contains(arg, "://")) {
		resp, err := c.Get(ctx, arg, spr.KindHTML)
		if err != nil {
			return nil, exit(CodeTransport, err)
		}
		if resp.Status == spr.StatusNotFound {
			return nil, exit(CodeNoData, fmt.Errorf("%s: there is no such page", arg))
		}
		return resp, nil
	}

	doi, err := spr.ParseDOI(arg)
	if err != nil {
		return nil, exit(CodeUsage, err)
	}
	paths := doi.Paths()
	for _, p := range paths {
		resp, err := c.Get(ctx, p, spr.KindHTML)
		if err != nil {
			return nil, exit(CodeTransport, err)
		}
		if resp.Status != spr.StatusNotFound {
			return resp, nil
		}
	}
	return nil, exit(CodeNoData, fmt.Errorf("%s is not published at any of %s on link.springer.com", doi, strings.Join(paths, ", ")))
}

// printWork writes the human form. It leads with what the work is, then who
// wrote it, then what was in it, and puts provenance last, because somebody
// running this at a prompt wants the title before they want the ladder.
func printWork(out io.Writer, w *spr.Work, withText, withEnvelope bool) {
	line := func(label, value string) {
		if value != "" {
			fmt.Fprintf(out, "%-13s %s\n", label, value)
		}
	}

	line("doi", w.DOI)
	kind := w.Type
	if w.ArticleType != "" && !strings.EqualFold(w.ArticleType, w.Type) {
		kind += ", " + w.ArticleType
	}
	line("type", kind)
	line("title", w.Title)
	line("url", w.URL)
	line("language", w.Language)

	container := w.ContainerTitle
	switch {
	case w.Volume != "" && w.Issue != "":
		container += fmt.Sprintf(" %s(%s)", w.Volume, w.Issue)
	case w.Volume != "":
		container += " " + w.Volume
	}
	if w.FirstPage != "" {
		container += " pp " + w.FirstPage
		if w.LastPage != "" {
			container += "-" + w.LastPage
		}
	}
	line("published in", strings.TrimSpace(container))
	line("publisher", w.Publisher)
	if len(w.ISSN) > 0 {
		line("issn", strings.Join(w.ISSN, ", "))
	}
	line("isbn", w.ISBN)

	// Four dates and they are four different facts, so the ones the page stated
	// are each printed under their own name and the ones it did not state are
	// simply not here.
	if w.Published != nil {
		line("published", w.Published.String())
	}
	if w.Online != nil {
		line("online", w.Online.String())
	}
	if w.CoverDate != nil {
		line("cover date", w.CoverDate.String())
	}
	if w.Modified != nil {
		line("modified", w.Modified.String())
	}

	line("access", accessLine(w.Access))
	line("license", w.License)
	line("copyright", w.Copyright)
	line("pdf", w.PDFURL)

	if len(w.Authors) > 0 {
		fmt.Fprintf(out, "\nauthors (%d)\n", len(w.Authors))
		for _, a := range w.Authors {
			fmt.Fprintf(out, "  %2d  %s", a.Position+1, a.Name)
			if a.ORCID != "" {
				fmt.Fprintf(out, "  %s", a.ORCID)
			}
			if a.Email != "" {
				fmt.Fprintf(out, "  %s", a.Email)
			}
			fmt.Fprintln(out)
			for _, af := range a.Affiliations {
				fmt.Fprintf(out, "      %s\n", strings.TrimSpace(af.Name+", "+af.Address))
			}
		}
	}

	if len(w.Keywords) > 0 {
		fmt.Fprintf(out, "\nkeywords     %s\n", strings.Join(w.Keywords, ", "))
	}
	if len(w.Subjects) > 0 {
		fmt.Fprintf(out, "subjects     %s\n", strings.Join(w.Subjects, ", "))
	}
	if w.Abstract != "" {
		fmt.Fprintf(out, "\nabstract\n  %s\n", wrap(w.Abstract, 76, "  "))
	}

	if len(w.Sections) > 0 {
		fmt.Fprintf(out, "\nsections (%d)\n", len(w.Sections))
		for _, s := range w.Sections {
			fmt.Fprintf(out, "  %s%s\n", strings.Repeat("  ", s.Depth), strings.TrimSpace(s.Number+" "+s.Title))
			if withText && s.Text != "" {
				fmt.Fprintf(out, "  %s%s\n", strings.Repeat("  ", s.Depth+1), wrap(s.Text, 72, "  "+strings.Repeat("  ", s.Depth+1)))
			}
		}
	}

	counts := []string{}
	if len(w.Figures) > 0 {
		counts = append(counts, fmt.Sprintf("%d figures", len(w.Figures)))
	}
	if w.Equations > 0 {
		counts = append(counts, fmt.Sprintf("%d equations", w.Equations))
	}
	if len(w.Footnotes) > 0 {
		counts = append(counts, fmt.Sprintf("%d footnotes", len(w.Footnotes)))
	}
	if len(w.References) > 0 {
		refs := fmt.Sprintf("%d references", len(w.References))
		if w.ReferenceLinks > 0 {
			refs += fmt.Sprintf(", %d with resolver links", w.ReferenceLinks)
		}
		counts = append(counts, refs)
	}
	if len(w.Rails) > 0 {
		counts = append(counts, fmt.Sprintf("%d recommendations", len(w.Rails)))
	}
	if len(counts) > 0 {
		fmt.Fprintf(out, "\nbody         %s\n", strings.Join(counts, ", "))
	}

	printEnvelope(out, w.Envelope, withEnvelope)
}

// printEnvelope writes the provenance. The short form is the two things that
// change what a reader should believe: how many fields were answered and which
// ones were looked for and not found. The long form is the whole ladder.
func printEnvelope(out io.Writer, e spr.Envelope, full bool) {
	fmt.Fprintf(out, "\nenvelope     %s, %s, %d bytes, %d redirects, fetched %s\n",
		e.Tier, e.Status, e.Bytes, e.Redirects, e.Fetched.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(out, "             %d fields answered, %d missed, %d regions unread\n",
		len(e.Via), len(e.Missed), len(e.Unread))

	for _, m := range e.Missed {
		fmt.Fprintf(out, "  missed     %s: %s\n", m.Field, m.Why)
	}
	if !full {
		return
	}

	fmt.Fprintln(out, "\nvia")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, name := range sortedKeys(e.Via) {
		fmt.Fprintf(tw, "  %s\t%s\n", name, e.Via[name])
	}
	_ = tw.Flush()

	if len(e.Unread) > 0 {
		fmt.Fprintf(out, "\nunread (%d)\n", len(e.Unread))
		for _, r := range e.Unread {
			fmt.Fprintf(out, "  %s\n", r)
		}
	}
	if len(e.Extra) > 0 {
		fmt.Fprintln(out, "\ncarried and not read")
		for _, name := range sortedKeys(e.Extra) {
			fmt.Fprintf(out, "  %s  %d bytes\n", name, len(e.Extra[name]))
		}
	}
}

// accessLine says what the page said about this reader, in the page's own
// terms. Free and the world readable tag are two separate declarations and both
// are printed, because the day they stop agreeing is a day worth noticing.
func accessLine(a spr.Access) string {
	var parts []string
	switch {
	case a.Free == nil:
		// Nothing is printed for a page that declared nothing, so that it does
		// not read as a page that declared no.
	case *a.Free:
		parts = append(parts, "free to read")
	default:
		parts = append(parts, "not free to read")
	}
	if a.Raw != "" {
		parts = append(parts, "access="+a.Raw)
	}
	if a.WorldReadable != nil {
		v := *a.WorldReadable
		if v == "" {
			v = "declared and empty"
		}
		parts = append(parts, "world readable "+v)
	}
	if a.Statement != "" {
		parts = append(parts, a.Statement)
	}
	return strings.Join(parts, ", ")
}

// wrap folds a long string at a column, indenting every line after the first,
// so that an abstract in a terminal is readable rather than one long line.
func wrap(s string, width int, indent string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	col := 0
	for i, w := range words {
		if i > 0 {
			if col+1+len(w) > width {
				b.WriteString("\n" + indent)
				col = 0
			} else {
				b.WriteString(" ")
				col++
			}
		}
		b.WriteString(w)
		col += len(w)
	}
	return b.String()
}

// sortedKeys keeps two runs of the same command producing the same bytes, so a
// diff between them means the page changed.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
