package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// The three subpage commands.
//
// A work has three pages hanging off its own address and each of them is worth
// a command for a different reason. /metrics is the only place the publisher
// states how often the work was read, how often it was cited and by whose
// count. /figures/N carries the full resolution asset where the article page
// carries a thumbnail of it. /tables/N carries the table, full stop, because the
// article page announces its tables and publishes no table elements at all.
//
// So figures and tables are not a symmetrical pair. spr figures with no number
// answers from the article page and costs nothing extra, because the labels,
// the captions and the descriptions are all there. spr tables with no number
// also answers from the article page and can only tell you what the tables are
// called, since that is genuinely all the article page knows.

func metricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics <doi, url or path>",
		Short: "Read the accesses, citations and attention a work has drawn",
		Long: "metrics reads a work's /metrics subpage, which is the only page on this site that says\n" +
			"how often the work was read and how often it was cited.\n\n" +
			"A citation count is not a number, it is a number and a counter. Springer reports 1,906\n" +
			"for one of the measured articles where Crossref reports 1,553 and OpenAlex 1,563, and\n" +
			"the three are all correct about three different corpora. So the count is never printed\n" +
			"without the body that produced it, and if the page stops naming its source the record\n" +
			"says the source is missing rather than implying Springer counted it.\n\n" +
			"The attention score comes with two comparisons and not one: this work against every\n" +
			"tracked article of a similar age, and against the tracked articles of a similar age in\n" +
			"its own journal. On the measured capture those are 95th of 474,090 and 96th of 29, and\n" +
			"quoting the second without the cohort size behind it is how a percentile lies.\n\n" +
			"This subpage exists for articles. A chapter's /metrics answers 404.",
		Args: cobra.ExactArgs(1),
		Example: "  spr metrics 10.1007/s10994-021-05946-3\n" +
			"  spr metrics -o json 10.1007/s10994-021-05946-3 | jq .altmetric.cohorts\n" +
			"  spr metrics -o json 10.1007/s10994-021-05946-3 | jq -r '.citations | \"\\(.count) per \\(.source)\"'",
	}

	var envelope bool
	cmd.Flags().BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := metricsTarget(args[0])
		if err != nil {
			return err
		}
		resp, err := fetchSubpage(cmd, target)
		if err != nil {
			return err
		}
		m, err := spr.ExtractMetrics(resp)
		if err != nil {
			return exit(CodeNoData, err)
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			if err := encode(out, m); err != nil {
				return err
			}
			return statusExit(resp.Status)
		}
		printMetrics(out, m, envelope)
		return statusExit(resp.Status)
	}
	return cmd
}

func figuresCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "figures <doi, url or path> [number]",
		Short: "List a work's figures, or read one at full size",
		Long: "figures with no number lists what the article page already knows: every figure's label,\n" +
			"its caption, its description and the address of its own page. That costs one request\n" +
			"and it is the same request spr work makes.\n\n" +
			"figures with a number fetches that figure's own page, and the only thing it buys you is\n" +
			"the asset. The article page serves the image at 685 pixels wide and the figure page\n" +
			"serves the same asset at 1,177, under a path that differs by one segment. The path is\n" +
			"read rather than built, because guessing a CDN's naming scheme works until it does not.\n\n" +
			"A number past the end is not a 404. Springer answers it with 224 KB of page furniture\n" +
			"and an empty body, so the request looks healthy and the page is empty, and this command\n" +
			"says there is no such figure rather than printing a record with nothing in it.",
		Args: cobra.RangeArgs(1, 2),
		Example: "  spr figures 10.1007/s10994-021-05946-3\n" +
			"  spr figures 10.1007/s10994-021-05946-3 1\n" +
			"  spr figures -o json 10.1007/s10994-021-05946-3 1 | jq .image",
	}

	var envelope bool
	cmd.Flags().BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return listAssets(cmd, args[0], "figures")
		}
		n, err := assetNumber(args[1])
		if err != nil {
			return err
		}
		resp, err := fetchAsset(cmd, args[0], "figures", n)
		if err != nil {
			return err
		}
		f, err := spr.ExtractFigure(resp)
		if err != nil {
			return assetError(err, args[0], "figure", n)
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			if err := encode(out, f); err != nil {
				return err
			}
			return statusExit(resp.Status)
		}
		printFigure(out, f, envelope)
		return statusExit(resp.Status)
	}
	return cmd
}

func tablesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tables <doi, url or path> [number]",
		Short: "List a work's tables, or read one in full",
		Long: "tables with no number lists what the article page announces, which is a label, a caption\n" +
			"and a link per table, and that is genuinely everything the article page knows.\n\n" +
			"tables with a number fetches the table. This is the one subpage in this tool that is not\n" +
			"an optimization: the open access capture is 718 KB of html, it announces one table, and\n" +
			"it contains zero table elements. The rows are published here and nowhere else, so a\n" +
			"pipeline that reads the article page and expects to find tabular data will find none of\n" +
			"it and no error either.\n\n" +
			"Cells keep the LaTeX the publisher wrote. Rendering it would be this tool having an\n" +
			"opinion about notation, and anyone who wants it rendered has a renderer.",
		Args: cobra.RangeArgs(1, 2),
		Example: "  spr tables 10.1007/s10994-021-05946-3\n" +
			"  spr tables 10.1007/s10994-021-05946-3 1\n" +
			"  spr tables -o json 10.1007/s10994-021-05946-3 1 | jq -r '.rows[] | @tsv'",
	}

	var envelope bool
	cmd.Flags().BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return listAssets(cmd, args[0], "tables")
		}
		n, err := assetNumber(args[1])
		if err != nil {
			return err
		}
		resp, err := fetchAsset(cmd, args[0], "tables", n)
		if err != nil {
			return err
		}
		t, err := spr.ExtractTable(resp)
		if err != nil {
			return assetError(err, args[0], "table", n)
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			if err := encode(out, t); err != nil {
				return err
			}
			return statusExit(resp.Status)
		}
		printTable(out, t, envelope)
		return statusExit(resp.Status)
	}
	return cmd
}

// metricsTarget turns one argument into the metrics url to fetch.
//
// A bare doi goes straight to /article/<doi>/metrics rather than through the
// path search spr work does, because this subpage only exists for articles. A
// chapter doi therefore costs one request and gets told so, which is a better
// answer than four requests and the same news.
func metricsTarget(arg string) (string, error) {
	if isURL(arg) {
		if spr.MetricsPath(arg) {
			return arg, nil
		}
		return spr.MetricsURL(arg), nil
	}
	doi, err := spr.ParseDOI(arg)
	if err != nil {
		return "", exit(CodeUsage, err)
	}
	return spr.MetricsURL("/article/" + string(doi)), nil
}

// assetNumber reads the figure or table number off the command line. Zero and
// negative numbers are refused here rather than sent, because the site answers
// them with a page.
func assetNumber(arg string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n < 1 {
		return 0, exit(CodeUsage, fmt.Errorf("%q is not a figure or table number; they start at 1", arg))
	}
	return n, nil
}

// fetchAsset resolves the work and fetches one of its asset subpages.
func fetchAsset(cmd *cobra.Command, arg, kind string, n int) (*spr.Response, error) {
	work, err := assetWorkURL(arg)
	if err != nil {
		return nil, err
	}
	target := spr.FigureURL(work, n)
	if kind == "tables" {
		target = spr.TableURL(work, n)
	}
	return fetchSubpage(cmd, target)
}

// assetWorkURL turns one argument into the work path the subpages hang off.
//
// A url that is already a subpage is trimmed back to the work, so that pasting
// /figures/1 out of a browser and asking for figure 4 does the obvious thing
// rather than building /figures/1/figures/4.
func assetWorkURL(arg string) (string, error) {
	if isURL(arg) {
		return spr.WorkURL(arg), nil
	}
	doi, err := spr.ParseDOI(arg)
	if err != nil {
		return "", exit(CodeUsage, err)
	}
	return spr.WorkURL("/article/" + string(doi)), nil
}

// listAssets answers from the article page, which already holds the list.
func listAssets(cmd *cobra.Command, arg, kind string) error {
	c := client(cmd.ErrOrStderr())
	resp, err := findWork(cmd.Context(), c, arg)
	if err != nil {
		return err
	}
	if resp.Status == spr.StatusChallenged {
		fmt.Fprintln(cmd.ErrOrStderr(), explain(resp.Status))
		return exit(CodeChallenged, nil)
	}
	w, err := spr.ExtractWork(resp)
	if err != nil {
		return exit(CodeUsage, fmt.Errorf("%s is not a work page, so it has no %s to list", resp.URL, kind))
	}

	out := cmd.OutOrStdout()
	if kind == "tables" {
		if g.format == "json" {
			if err := encode(out, w.Tables); err != nil {
				return err
			}
			return statusExit(resp.Status)
		}
		printTableList(out, w)
		return statusExit(resp.Status)
	}
	if g.format == "json" {
		if err := encode(out, w.Figures); err != nil {
			return err
		}
		return statusExit(resp.Status)
	}
	printFigureList(out, w)
	return statusExit(resp.Status)
}

// fetchSubpage makes the one request a subpage command makes.
func fetchSubpage(cmd *cobra.Command, target string) (*spr.Response, error) {
	c := client(cmd.ErrOrStderr())
	resp, err := c.Get(cmd.Context(), target, spr.KindHTML)
	if err != nil {
		return nil, exit(CodeTransport, err)
	}
	if resp.Status == spr.StatusNotFound {
		return nil, exit(CodeNoData, fmt.Errorf("%s: there is no such page", target))
	}
	if resp.Status == spr.StatusChallenged {
		fmt.Fprintln(cmd.ErrOrStderr(), explain(resp.Status))
		return nil, exit(CodeChallenged, nil)
	}
	return resp, nil
}

// assetError turns the empty page a number past the end produces into the
// sentence a person would write about it.
func assetError(err error, arg, kind string, n int) error {
	if err == spr.ErrNoSuchFigure || err == spr.ErrNoSuchTable {
		return exit(CodeNoData, fmt.Errorf("%s has no %s %d, and the site answered 200 for it anyway", arg, kind, n))
	}
	return exit(CodeNoData, err)
}

// printMetrics writes the human form. Counts first, because that is what
// somebody typing this at a prompt came for, and each one carries whoever
// counted it.
func printMetrics(out io.Writer, m *spr.Metrics, withEnvelope bool) {
	line := field(out)
	line("title", m.Title)
	line("doi", m.DOI)
	line("article", m.ArticleURL)
	if m.Updated != nil {
		line("updated", m.Updated.Format("2006-01-02 15:04 MST"))
	}

	// Two separate facts, and the printer keeps them separate. 134k is the page
	// abbreviating, which is worth expanding. Approximate is the page saying in
	// its own prose that the number is not exact, and it says that for 1069 as
	// well, so an expansion is not what discloses it.
	if a := m.Accesses; a != nil {
		v := a.Raw
		if s := strconv.Itoa(a.Value); a.Value > 0 && s != a.Raw {
			v += ", about " + group(a.Value)
		}
		if a.Approximate {
			v += ", which the page calls an approximate count"
		}
		line("accesses", v)
	}
	if c := m.Citations; c != nil {
		src := c.Source
		if src == "" {
			src = "an unnamed counter, which is why this record does not treat it as Springer's"
		}
		line("citations", fmt.Sprintf("%s per %s", group(c.Count), src))
	}

	if a := m.Altmetric; a != nil {
		line("attention", fmt.Sprintf("%d", a.Score))
		line("details", a.DetailsURL)
		for _, k := range a.Breakdown {
			fmt.Fprintf(out, "%-13s %s %s\n", "  "+k.Kind, group(k.Count), k.Label)
		}
		for _, c := range a.Cohorts {
			fmt.Fprintf(out, "%-13s %s of %s tracked articles in %s\n", "  ranked", ordinal(c.Rank), group(c.Size), c.Scope)
			fmt.Fprintf(out, "%-13s %s percentile\n", "", ordinal(c.Percentile))
		}
	}

	if len(m.Mentions) > 0 {
		fmt.Fprintf(out, "\nmentions (%d, the named coverage only)\n", len(m.Mentions))
		for _, mn := range m.Mentions {
			fmt.Fprintf(out, "  %s\n    %s\n    %s\n", mn.Outlet, wrap(mn.Title, 74, "    "), mn.URL)
		}
	}
	printEnvelope(out, m.Envelope, withEnvelope)
}

func printFigure(out io.Writer, f *spr.FigurePage, withEnvelope bool) {
	line := field(out)
	line("label", f.Label)
	line("in", f.ArticleTitle)
	line("article", f.ArticleURL+f.Anchor)
	if i := f.Image; i != nil {
		line("image", i.URL)
		if i.Width > 0 && i.Height > 0 {
			line("size", fmt.Sprintf("%d by %d", i.Width, i.Height))
		}
		line("webp", i.WebP)
		line("alt", i.Alt)
	}
	if f.Caption != "" {
		fmt.Fprintf(out, "\ncaption\n  %s\n", wrap(f.Caption, 76, "  "))
	}
	if len(f.Refs) > 0 {
		fmt.Fprintf(out, "\ncited in the caption (%d)\n", len(f.Refs))
		for _, r := range f.Refs {
			fmt.Fprintf(out, "  %s\n", wrap(joinNonEmpty("  ", r.Citation, r.URL), 74, "  "))
		}
	}
	printEnvelope(out, f.Envelope, withEnvelope)
}

func printTable(out io.Writer, t *spr.TablePage, withEnvelope bool) {
	line := field(out)
	line("label", t.Label)
	line("in", t.ArticleTitle)
	line("article", t.ArticleURL+t.Anchor)
	if t.Caption != "" {
		fmt.Fprintf(out, "\ncaption\n  %s\n", wrap(t.Caption, 76, "  "))
	}

	// The rows are printed as tab separated fields rather than aligned into
	// columns. A cell here holds LaTeX and can be far wider than a terminal, so
	// aligning would either wrap the table into nonsense or truncate the data,
	// and tabs are what the next program in the pipe wants anyway.
	if len(t.Head) > 0 || len(t.Rows) > 0 {
		fmt.Fprintf(out, "\n%d rows, %d columns\n", len(t.Rows), t.Cols())
		if len(t.Head) > 0 {
			fmt.Fprintln(out, strings.Join(t.Head, "\t"))
		}
		for _, r := range t.Rows {
			fmt.Fprintln(out, strings.Join(r, "\t"))
		}
	}
	printEnvelope(out, t.Envelope, withEnvelope)
}

func printFigureList(out io.Writer, w *spr.Work) {
	if len(w.Figures) == 0 {
		fmt.Fprintf(out, "%s publishes no figures\n", w.Title)
		return
	}
	fmt.Fprintf(out, "%s\n\nfigures (%d)\n", w.Title, len(w.Figures))
	for _, f := range w.Figures {
		fmt.Fprintf(out, "\n  %s\n", f.Label)
		if f.Caption != "" {
			fmt.Fprintf(out, "  %s\n", wrap(f.Caption, 74, "  "))
		}
		fmt.Fprintf(out, "  %s\n", f.PageURL)
	}
	fmt.Fprintln(out, "\nThe images above are the inline rendition. Ask for a number to get the full one.")
}

func printTableList(out io.Writer, w *spr.Work) {
	if len(w.Tables) == 0 {
		fmt.Fprintf(out, "%s publishes no tables\n", w.Title)
		return
	}
	fmt.Fprintf(out, "%s\n\ntables (%d)\n", w.Title, len(w.Tables))
	for _, t := range w.Tables {
		fmt.Fprintf(out, "\n  %s\n", t.Label)
		if t.Caption != "" {
			fmt.Fprintf(out, "  %s\n", wrap(t.Caption, 74, "  "))
		}
		fmt.Fprintf(out, "  %s\n", t.PageURL)
	}
	fmt.Fprintln(out, "\nThe article page carries no rows for any of these. Ask for a number to read one.")
}

// ordinal writes 1st, 22,032nd and 96th, which is how the page writes them and
// how a reader expects a rank to look.
func ordinal(n int) string {
	s := group(n)
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return s + "th"
	case n%10 == 1:
		return s + "st"
	case n%10 == 2:
		return s + "nd"
	case n%10 == 3:
		return s + "rd"
	}
	return s + "th"
}

// group writes a number with thousands separators, because 474090 and 474,090
// are the same number and only one of them can be read at a glance.
func group(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
