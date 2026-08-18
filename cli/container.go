package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// The three container commands.
//
// They share this file because they share their whole shape: one argument that
// is either an identifier or a url, one fetch, one extractor, one printer and
// the same envelope at the bottom. What they do not share is the record, and
// that is deliberate. A journal has an impact factor and no price, a book has
// three prices and no impact factor, and a command that printed one table for
// both would have to leave half of it blank on every page it was given.

func journalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "journal <id, issn or url>",
		Short: "Read one journal home page",
		Long: "journal reads a journal home page and prints the record it produced.\n\n" +
			"A journal home page carries 8 meta names and none of them are bibliographic, so almost\n" +
			"everything here is read out of a named region or out of the analytics payload the page\n" +
			"ships for its own tag manager. The envelope says which, field by field.\n\n" +
			"The volumes are a second page and a second request. --volumes makes it, and without it\n" +
			"the record says where the volumes are and that none were read, which is a different\n" +
			"statement from a journal with no volumes.",
		Args: cobra.ExactArgs(1),
		Example: "  spr journal 10994\n" +
			"  spr journal --volumes 10994\n" +
			"  spr journal -o json 10994 | jq .metrics",
	}

	var (
		volumes  bool
		envelope bool
	)
	cmd.Flags().BoolVar(&volumes, "volumes", false, "fetch the volumes and issues page as well, which is one more request and the whole back catalogue")
	cmd.Flags().BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		c := client(cmd.ErrOrStderr())
		resp, err := fetchContainer(cmd, c, journalTarget(args[0]))
		if err != nil {
			return err
		}
		j, err := spr.ExtractJournal(resp)
		if err != nil {
			return containerError(err, resp.URL, "journal")
		}

		var vols *spr.Volumes
		if volumes {
			vresp, err := fetchContainer(cmd, c, spr.VolumesURL(spr.SpringerID(j.SpringerID)))
			if err != nil {
				return err
			}
			vols, err = spr.ExtractVolumes(vresp)
			if err != nil {
				return containerError(err, vresp.URL, "volumes and issues")
			}
			j.Volumes = &spr.Conn{
				Loaded:     vols.Count(),
				TotalCount: vols.Count(),
				Complete:   true,
				URL:        vresp.URL,
			}
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			if vols != nil {
				return encode(out, map[string]any{"journal": j, "volumes": vols})
			}
			return encode(out, j)
		}
		printJournal(out, j, envelope)
		if vols != nil {
			printVolumes(out, vols)
		}
		return statusExit(resp.Status)
	}
	return cmd
}

func bookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "book <doi, isbn or url>",
		Short: "Read one book, proceedings volume or reference work",
		Long: "book reads a book page and prints the record it produced, including its table of\n" +
			"contents and what the page is charging for it.\n\n" +
			"The page prints four ISBNs under three labels and they are four different objects: the\n" +
			"electronic edition, the hardcover, the softcover and a print isbn in the analytics\n" +
			"payload. They are kept apart, because a record that is right about a book and wrong\n" +
			"about which edition you can buy is worse than one that says less.\n\n" +
			"Prices are localized by the requesting address, so the currency is read off the page\n" +
			"and never assumed. A record fetched from two places can carry two currencies for the\n" +
			"same book and neither is wrong.",
		Args: cobra.ExactArgs(1),
		Example: "  spr book 10.1007/978-3-031-28170-9\n" +
			"  spr book 978-3-031-28170-9\n" +
			"  spr book -o json 10.1007/978-3-031-28170-9 | jq '.chapters[].doi'",
	}

	var (
		chapters bool
		envelope bool
	)
	cmd.Flags().BoolVar(&chapters, "chapters", false, "print the whole table of contents rather than a count")
	cmd.Flags().BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		c := client(cmd.ErrOrStderr())
		target, err := bookTarget(args[0])
		if err != nil {
			return err
		}
		resp, err := fetchContainer(cmd, c, target)
		if err != nil {
			return err
		}
		b, err := spr.ExtractBook(resp)
		if err != nil {
			return containerError(err, resp.URL, "book")
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			return encode(out, b)
		}
		printBook(out, b, chapters, envelope)
		return statusExit(resp.Status)
	}
	return cmd
}

func seriesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "series <id or url>",
		Short: "Read one book series home page",
		Long: "series reads a book series home page and prints the record it produced.\n\n" +
			"The home page shows the five most recent titles and not the series, which for Lecture\n" +
			"Notes in Computer Science is five out of many thousands. The record says so: the titles\n" +
			"it holds are named latest_titles, and the pointer to the rest of them says how many\n" +
			"were read and where the others are.",
		Args: cobra.ExactArgs(1),
		Example: "  spr series 558\n" +
			"  spr series -o json 558 | jq .editors",
	}

	var envelope bool
	cmd.Flags().BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		c := client(cmd.ErrOrStderr())
		resp, err := fetchContainer(cmd, c, seriesTarget(args[0]))
		if err != nil {
			return err
		}
		s, err := spr.ExtractSeries(resp)
		if err != nil {
			return containerError(err, resp.URL, "series")
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			return encode(out, s)
		}
		printSeries(out, s, envelope)
		return statusExit(resp.Status)
	}
	return cmd
}

// journalTarget turns one argument into one path. A url or a path is taken as
// given, an issn is left alone because the site resolves it itself, and
// anything else is a springer journal number.
func journalTarget(arg string) string {
	if isURL(arg) {
		return arg
	}
	if issn, err := spr.ParseISSN(arg); err == nil {
		return "/journal/" + string(issn)
	}
	return "/journal/" + arg
}

// seriesTarget turns one argument into one path.
func seriesTarget(arg string) string {
	if isURL(arg) {
		return arg
	}
	return "/series/" + arg
}

// bookTarget turns one argument into one path.
//
// A book is addressable two ways and this tool uses both as given rather than
// converting between them. /book/10.1007/978-3-031-28170-9 is the doi form and
// /book/9783031281709 is the isbn form, and they are the same page.
func bookTarget(arg string) (string, error) {
	if isURL(arg) {
		return arg, nil
	}
	if doi, err := spr.ParseDOI(arg); err == nil {
		return "/book/" + string(doi), nil
	}
	if isbn, err := spr.ParseISBN(arg); err == nil {
		return "/book/" + isbn.Key(), nil
	}
	return "", exit(CodeUsage, fmt.Errorf("%q is not a doi, an isbn or a link.springer.com url", arg))
}

func isURL(arg string) bool {
	return strings.HasPrefix(arg, "/") || strings.Contains(arg, "://")
}

// fetchContainer makes the one request a container command makes, and turns the
// two answers that are not a page into the exit codes a script can act on.
func fetchContainer(cmd *cobra.Command, c *spr.Client, target string) (*spr.Response, error) {
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

// containerError separates a page that is the wrong kind, which is the caller's
// mistake, from a page that would not parse, which is not.
func containerError(err error, url, kind string) error {
	if errors.Is(err, spr.ErrNotAJournal) || errors.Is(err, spr.ErrNotABook) || errors.Is(err, spr.ErrNotASeries) || errors.Is(err, spr.ErrNotVolumes) {
		return exit(CodeUsage, fmt.Errorf("%s is not a %s page", url, kind))
	}
	return exit(CodeNoData, err)
}

func encode(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return exit(CodeTransport, err)
	}
	return nil
}

// printJournal writes the human form, leading with what the journal is and
// putting the numbers it publishes about itself next, because those are what
// somebody typing this at a prompt came for.
func printJournal(out io.Writer, j *spr.Journal, withEnvelope bool) {
	line := field(out)
	line("title", j.Title)
	line("id", j.SpringerID)
	line("url", j.URL)
	line("issn", joinNonEmpty(", ", j.ElectronicISSN, j.PrintISSN))
	line("publisher", joinNonEmpty(", ", j.PublisherBrand, j.Imprint))
	line("model", j.PublishingModel)
	if j.ContinuousPublication != nil && *j.ContinuousPublication {
		line("publishing", "continuous, articles appear before the issue closes")
	}
	if j.OpenAccessArticles > 0 {
		line("open access", fmt.Sprintf("%d articles", j.OpenAccessArticles))
	}
	line("copyright", j.Copyright)

	for _, m := range j.Metrics {
		fmt.Fprintf(out, "%-13s %s (%d)\n", "metric", strings.TrimSpace(m.Name+" "+m.Raw), m.Year)
	}
	if len(j.Subjects) > 0 {
		line("subjects", strings.Join(j.Subjects, ", "))
	}
	if len(j.Editors) > 0 {
		fmt.Fprintf(out, "\neditors (%d)\n", len(j.Editors))
		for _, e := range j.Editors {
			fmt.Fprintf(out, "  %2d  %s\n", e.Position+1, strings.TrimSpace(e.Name+"  "+e.Role))
		}
	}
	if len(j.IndexedIn) > 0 {
		fmt.Fprintf(out, "\nindexed in (%d)\n  %s\n", len(j.IndexedIn), wrap(strings.Join(j.IndexedIn, ", "), 76, "  "))
	}
	if j.About != "" {
		fmt.Fprintf(out, "\nabout\n  %s\n", wrap(j.About, 76, "  "))
	}
	if j.Volumes != nil {
		fmt.Fprintf(out, "\nvolumes      %s\n", conn(*j.Volumes))
	}
	printEnvelope(out, j.Envelope, withEnvelope)
}

// printVolumes writes the volume index, one line per volume and one line per
// issue, which for a journal of any age is a long list and is why it takes a
// flag to ask for it.
func printVolumes(out io.Writer, v *spr.Volumes) {
	fmt.Fprintf(out, "\nvolumes (%d), %d issues\n", len(v.Volumes), v.Count())
	for _, vol := range v.Volumes {
		fmt.Fprintf(out, "  volume %-5s %s\n", vol.Number, vol.Label)
		for _, iss := range vol.Issue {
			date := ""
			if iss.Date != nil {
				date = iss.Date.String()
			}
			// The themed collection line is padded to a column only when
			// there is one, so that the 262 issues without one do not each
			// carry ten trailing spaces into whatever reads this.
			if iss.SpecialTitle == "" {
				fmt.Fprintf(out, "    %-10s %s\n", iss.Label, date)
				continue
			}
			fmt.Fprintf(out, "    %-10s %-10s %s\n", iss.Label, date, iss.SpecialTitle)
		}
	}
	printEnvelope(out, v.Envelope, false)
}

// printBook writes the human form. The four isbns are printed under four names
// rather than one, because that is the one thing about this page a reader is
// most likely to get wrong.
func printBook(out io.Writer, b *spr.Book, withChapters, withEnvelope bool) {
	line := field(out)
	line("title", strings.TrimSpace(b.Title+". "+b.Subtitle))
	line("doi", b.DOI)
	line("kind", joinNonEmpty(", ", b.Kind, b.ProductType))
	line("url", b.URL)
	line("isbn ebook", b.ISBNElectronic)
	line("isbn print", b.ISBNPrint)
	line("isbn hard", b.ISBNHardcover)
	line("isbn soft", b.ISBNSoftcover)
	line("publisher", b.Publisher)
	line("edition", b.Edition)
	line("pages", b.Pages)
	line("illustrations", b.Illustrations)
	if b.Series != nil {
		line("series", strings.TrimSpace(b.Series.Name+"  "+b.SeriesISSN))
	}
	if b.Conference != nil {
		line("conference", strings.TrimSpace(fmt.Sprintf("%s %s %d", b.Conference.Name, b.Conference.Acronym, b.Conference.Year)))
	}
	if b.Published != nil {
		line("published", b.Published.String())
	}
	if b.PublishedHardcover != nil {
		line("hardcover", b.PublishedHardcover.String())
	}
	if b.PublishedSoftcover != nil {
		line("softcover", b.PublishedSoftcover.String())
	}
	line("copyright", b.Copyright)
	line("access", accessLine(b.Access))
	if b.Accesses > 0 || b.Citations > 0 {
		line("metrics", fmt.Sprintf("%d accesses, %d citations", b.Accesses, b.Citations))
	}

	if len(b.Authors) > 0 {
		fmt.Fprintf(out, "\nauthors (%d)\n", len(b.Authors))
		for _, a := range b.Authors {
			fmt.Fprintf(out, "  %2d  %s\n", a.Position+1, a.Name)
		}
	}
	if len(b.Editors) > 0 {
		fmt.Fprintf(out, "\neditors (%d)\n", len(b.Editors))
		for _, e := range b.Editors {
			fmt.Fprintf(out, "  %2d  %s\n", e.Position+1, e.Name)
		}
	}
	if len(b.Subjects) > 0 {
		fmt.Fprintf(out, "\nsubjects     %s\n", strings.Join(b.Subjects, ", "))
	}
	if len(b.Keywords) > 0 {
		fmt.Fprintf(out, "keywords     %s\n", wrap(strings.Join(b.Keywords, ", "), 76, "             "))
	}

	if len(b.Offers) > 0 {
		fmt.Fprintln(out, "\nprices")
		for _, o := range b.Offers {
			price := ""
			if o.Price != nil {
				price = o.Price.Raw
			}
			fmt.Fprintf(out, "  %-16s %-10s %s\n", o.Label, price, o.Kind)
		}
	}

	if len(b.Chapters) > 0 {
		fmt.Fprintf(out, "\nchapters (%d of %d rows, front and back matter included)\n", b.ChapterCount, len(b.Chapters))
		if withChapters {
			for _, ch := range b.Chapters {
				fmt.Fprintf(out, "  %-10s %s\n", ch.Pages, ch.Title)
				if ch.DOI != "" {
					fmt.Fprintf(out, "             %s\n", ch.DOI)
				}
			}
		}
	}
	printEnvelope(out, b.Envelope, withEnvelope)
}

// printSeries writes the human form.
func printSeries(out io.Writer, s *spr.Series, withEnvelope bool) {
	line := field(out)
	line("title", s.Title)
	line("id", s.SeriesID)
	line("url", s.URL)
	line("issn", joinNonEmpty(", ", s.ElectronicISSN, s.PrintISSN))

	if len(s.Editors) > 0 {
		fmt.Fprintf(out, "\neditors (%d)\n", len(s.Editors))
		for _, e := range s.Editors {
			fmt.Fprintf(out, "  %2d  %s\n", e.Position+1, strings.TrimSpace(e.Name+"  "+e.Role))
		}
	}
	if len(s.IndexedIn) > 0 {
		fmt.Fprintf(out, "\nindexed in (%d)\n  %s\n", len(s.IndexedIn), wrap(strings.Join(s.IndexedIn, ", "), 76, "  "))
	}
	if len(s.LatestTitles) > 0 {
		fmt.Fprintf(out, "\nlatest titles (%d)\n", len(s.LatestTitles))
		for _, t := range s.LatestTitles {
			year := ""
			if t.CopyrightYear > 0 {
				year = fmt.Sprintf("%d", t.CopyrightYear)
			}
			flag := ""
			if t.OpenAccess {
				flag = "  open access"
			}
			fmt.Fprintf(out, "  %-6s %s%s\n", year, t.Title, flag)
		}
	}
	if s.Titles != nil {
		fmt.Fprintf(out, "\ntitles       %s\n", conn(*s.Titles))
	}
	if s.About != "" {
		fmt.Fprintf(out, "\nabout\n  %s\n", wrap(s.About, 76, "  "))
	}
	printEnvelope(out, s.Envelope, withEnvelope)
}

// conn says what a pointer to an unread collection actually says, which is how
// many are held and where the rest of them are.
func conn(c spr.Conn) string {
	held := fmt.Sprintf("%d held", c.Loaded)
	if c.TotalCount > 0 {
		held = fmt.Sprintf("%d of %d held", c.Loaded, c.TotalCount)
	}
	if !c.Complete {
		held += ", more at " + c.URL
	}
	return held
}

// field returns the label printer the container commands share.
func field(out io.Writer) func(label, value string) {
	return func(label, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(out, "%-13s %s\n", label, value)
		}
	}
}

// joinNonEmpty joins the values that are actually there, so that a journal with
// one issn does not print a trailing comma for the one it does not have.
func joinNonEmpty(sep string, values ...string) string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, sep)
}
