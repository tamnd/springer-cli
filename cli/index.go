package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// spr crossref, spr openalex, spr cited-by and spr api.
//
// Four commands over three hosts that are not this site. They are here because
// link.springer.com publishes what a work cites and never publishes what cites
// it, and because the one number everybody wants, how often a work has been
// cited, has three different right answers depending on who counted.
//
// Each command prints under names that say which host answered. There is no
// command here that prints a merged citation count, and adding one would mean
// picking a number out of three that were measured nine to three hundred and
// fifty apart on the same DOI in the same minute.

func crossrefCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crossref [doi]",
		Short: "Read a work, its reference list or a query from Crossref",
		Long: "crossref reads the DOI registration agency, which holds what the publisher deposited\n" +
			"rather than what the web page renders.\n\n" +
			"That distinction is the reason to use it. The deposited reference list is the\n" +
			"publisher's own and carries DOIs, so it is the one place a reference becomes something\n" +
			"you can follow. The measured article deposits 122 references of which 66 carry a DOI,\n" +
			"and --references prints both numbers rather than a clean list of 66.\n\n" +
			"With a DOI it reads one record. With no DOI and any of the query flags it searches, and\n" +
			"--facet counts a result set by group for one request instead of paging it.\n\n" +
			"The citation count here is crossref_citations and it is deposited citations only. It\n" +
			"was 1,553 for a work the metrics page reports as 1,906 and OpenAlex as 1,563. All\n" +
			"three are right about a different corpus.",
		Args: cobra.MaximumNArgs(1),
		Example: "  spr crossref 10.1007/s10994-021-05946-3\n" +
			"  spr crossref 10.1007/s10994-021-05946-3 --references\n" +
			"  spr crossref --query \"aleatoric uncertainty\" --issn 0885-6125 --from 2020 --to 2024\n" +
			"  spr crossref --query \"uncertainty\" --facet type-name:5 --facet publisher-name:5\n" +
			"  spr crossref -o json 10.1007/s10994-021-05946-3 | jq .counts",
	}

	var (
		q          spr.CrossrefQuery
		issn, isbn string
		references bool
		envelope   bool
	)

	f := cmd.Flags()
	f.StringVar(&q.Bibliographic, "query", "", "free text across title, container, author and year")
	f.StringVar(&q.Title, "title", "", "title contains")
	f.StringVar(&q.Author, "author", "", "author name contains")
	f.StringVar(&issn, "issn", "", "only this journal, by either of its issns")
	f.StringVar(&isbn, "isbn", "", "only this book")
	f.StringVar(&q.Type, "type", "", "crossref work type: journal-article, book-chapter, proceedings-article")
	f.StringVar(&q.Funder, "funder", "", "funder registry id, with or without the resolver prefix")
	f.StringVar(&q.From, "from", "", "earliest publication date, as a year, a year and month, or a date")
	f.StringVar(&q.Until, "to", "", "latest publication date, at the same three precisions")
	f.IntVar(&q.Rows, "rows", 20, "results per page, capped by Crossref at 1000")
	f.StringVar(&q.Cursor, "cursor", "", "deep paging token, * for the first page of one")
	f.StringArrayVar(&q.Facet, "facet", nil, "count by group instead of listing, repeatable: type-name:5")
	f.StringVar(&q.Sort, "sort", "", "relevance, published or is-referenced-by-count")
	f.StringVar(&q.Order, "order", "", "asc or desc")
	f.BoolVar(&references, "references", false, "print the deposited reference list rather than the record")
	f.BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
		c := client(errw)

		if len(args) == 1 {
			doi, err := spr.ParseDOI(args[0])
			if err != nil {
				return exit(CodeUsage, err)
			}
			if references {
				return runCrossrefReferences(cmd, c, doi)
			}
			w, err := c.CrossrefWork(cmd.Context(), doi)
			if err != nil {
				return indexError(err)
			}
			if g.format == "json" {
				return encode(out, w)
			}
			printCrossrefWork(out, w, envelope)
			return nil
		}

		if references {
			return exit(CodeUsage, errors.New("--references reads one work's reference list, so it needs a doi"))
		}
		if issn != "" {
			parsed, err := spr.ParseISSN(issn)
			if err != nil {
				return exit(CodeUsage, err)
			}
			q.ISSN = parsed
		}
		if isbn != "" {
			parsed, err := spr.ParseISBN(isbn)
			if err != nil {
				return exit(CodeUsage, err)
			}
			q.ISBN = string(parsed)
		}
		if emptyCrossrefQuery(q) {
			return exit(CodeUsage, errors.New("crossref needs a doi, or --query, --title, --author or a filter to search with"))
		}

		res, err := c.CrossrefSearch(cmd.Context(), q)
		if err != nil {
			return indexError(err)
		}
		if g.format == "json" {
			if err := encode(out, res); err != nil {
				return err
			}
			return crossrefExit(res)
		}
		if len(q.Facet) > 0 {
			printCrossrefFacets(out, res)
			return crossrefExit(res)
		}
		printCrossrefResults(out, res, envelope)
		return crossrefExit(res)
	}
	return cmd
}

// emptyCrossrefQuery is whether a query would ask Crossref for everything.
//
// Sort, order, rows and the facet list are all present here and none of them
// narrows anything, so a run with those alone is 170 million records and a
// request nobody meant to make. The paging flags are deliberately not counted
// as content for the same reason.
func emptyCrossrefQuery(q spr.CrossrefQuery) bool {
	return q.Bibliographic == "" && q.Title == "" && q.Author == "" &&
		q.ISSN == "" && q.ISBN == "" && q.Type == "" && q.Funder == "" &&
		q.From == "" && q.Until == ""
}

func runCrossrefReferences(cmd *cobra.Command, c *spr.Client, doi spr.DOI) error {
	dois, unresolved, err := c.CrossrefReferences(cmd.Context(), doi)
	if err != nil {
		return indexError(err)
	}
	out := cmd.OutOrStdout()
	if g.format == "json" {
		if err := encode(out, map[string]any{
			"doi":                            string(doi),
			"crossref_references_with_doi":   len(dois),
			"crossref_references_unresolved": unresolved,
			"references":                     dois,
		}); err != nil {
			return err
		}
	} else {
		for _, d := range dois {
			fmt.Fprintln(out, d)
		}
		// The count of entries that carry no DOI goes to stderr so that piping
		// this into another command still gets a clean list of identifiers,
		// while a person watching still learns that the list is partial.
		fmt.Fprintf(cmd.ErrOrStderr(), "crossref: %d of %d deposited references carry a doi, and %d do not\n",
			len(dois), len(dois)+unresolved, unresolved)
	}
	if len(dois) == 0 {
		return exit(CodeNoData, nil)
	}
	return nil
}

func openalexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "openalex [doi or work id]",
		Short: "Read a work or a query from OpenAlex",
		Long: "openalex reads the open index that holds both citation directions, an abstract, and\n" +
			"institution ids that resolve to ROR.\n\n" +
			"It is the backend that fills in what the other two leave empty. Crossref carries the\n" +
			"measured article's authors with an empty affiliation array on every one of them, and\n" +
			"OpenAlex has Paderborn University with its ROR id for the same people. That is the\n" +
			"reason both backends exist rather than one.\n\n" +
			"The abstract is reconstructed from an inverted index, which is a word to position map\n" +
			"rather than a string. The words and their order come back exactly; the original\n" +
			"whitespace and any markup do not.\n\n" +
			"openalex_citations is a stored aggregate rather than a live count. The record carries\n" +
			"an updated date, and this command prints it next to the number, because listing the\n" +
			"same work's citations in the same minute returned nine fewer.",
		Args: cobra.MaximumNArgs(1),
		Example: "  spr openalex 10.1007/s10994-021-05946-3\n" +
			"  spr openalex W3014596384\n" +
			"  spr openalex --query \"aleatoric uncertainty\" --from 2020 --to 2024\n" +
			"  spr openalex -o json W3014596384 | jq '.authors[].institutions[].ror'",
	}

	var (
		q        spr.OpenAlexQuery
		issn     string
		envelope bool
	)

	f := cmd.Flags()
	f.StringVar(&q.Search, "query", "", "full text search, which OpenAlex scores")
	f.StringVar(&q.Title, "title", "", "title contains")
	f.StringVar(&q.Author, "author", "", "raw author name contains")
	f.StringVar(&issn, "issn", "", "only this journal, by any of its issns")
	f.StringVar(&q.From, "from", "", "earliest publication date, as a full date")
	f.StringVar(&q.Until, "to", "", "latest publication date, as a full date")
	f.StringVar(&q.Cites, "cites", "", "only works citing this one, by doi or work id")
	f.StringVar(&q.CitedBy, "cited-by", "", "only works this one cites, by doi or work id")
	f.IntVar(&q.PerPage, "rows", 25, "results per page, capped by OpenAlex at 200")
	f.IntVar(&q.Page, "page", 1, "which page of results")
	f.BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
		c := client(errw)

		if len(args) == 1 {
			w, err := openAlexRecord(cmd, c, args[0])
			if err != nil {
				return err
			}
			if g.format == "json" {
				return encode(out, w)
			}
			printOpenAlexWork(out, w, envelope)
			return nil
		}

		if issn != "" {
			parsed, err := spr.ParseISSN(issn)
			if err != nil {
				return exit(CodeUsage, err)
			}
			q.ISSN = parsed
		}
		if q.Search == "" && q.Title == "" && q.Author == "" && q.ISSN == "" && q.Cites == "" && q.CitedBy == "" {
			return exit(CodeUsage, errors.New("openalex needs a doi or a work id, or --query, --title, --author, --issn, --cites or --cited-by to search with"))
		}
		if q.PerPage > spr.OpenAlexPageSize {
			return exit(CodeUsage, fmt.Errorf("--rows %d is above the %d OpenAlex serves", q.PerPage, spr.OpenAlexPageSize))
		}

		res, err := c.OpenAlexSearch(cmd.Context(), q)
		if err != nil {
			return indexError(err)
		}
		if g.format == "json" {
			if err := encode(out, res); err != nil {
				return err
			}
			return openAlexExit(res)
		}
		printOpenAlexResults(out, res, envelope)
		return openAlexExit(res)
	}
	return cmd
}

// openAlexRecord reads one work by whichever of the two identifiers was given.
// A DOI goes to the doi: form of the path and a W id goes to the id itself,
// because OpenAlex answers both and picking the wrong one is a 404 rather than
// a redirect.
func openAlexRecord(cmd *cobra.Command, c *spr.Client, arg string) (*spr.OpenAlexWork, error) {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(arg)), "W") {
		w, err := c.OpenAlexWorkByID(cmd.Context(), arg)
		if err != nil {
			return nil, indexError(err)
		}
		return w, nil
	}
	doi, err := spr.ParseDOI(arg)
	if err != nil {
		return nil, exit(CodeUsage, fmt.Errorf("%q is neither a doi nor an openalex work id: %w", arg, err))
	}
	w, err := c.OpenAlexWork(cmd.Context(), doi)
	if err != nil {
		return nil, indexError(err)
	}
	return w, nil
}

func citedByCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cited-by <doi or work id>",
		Short: "List the works that cite this one, which this site does not publish",
		Long: "cited-by is the direction link.springer.com has no page for.\n\n" +
			"A work page lists what a work cites. Nothing on the site lists what cites it, and the\n" +
			"metrics page states a total attributed to Dimensions without naming a single citing\n" +
			"work. This command asks OpenAlex, which publishes the edges themselves.\n\n" +
			"The total it prints is meta.count from this listing and not the record's stored\n" +
			"cited_by_count. The two were 1,554 and 1,563 for the same work in the same minute,\n" +
			"because one is the live index and the other is an aggregate rebuilt on its own\n" +
			"schedule. Both are OpenAlex's and this command says which one it is holding.\n\n" +
			"--by-year gets the whole history in one request instead of the eight a full listing\n" +
			"costs, which is the cheap way to ask when a work was read rather than by whom.",
		Args: cobra.ExactArgs(1),
		Example: "  spr cited-by 10.1007/s10994-021-05946-3\n" +
			"  spr cited-by 10.1007/s10994-021-05946-3 --by-year\n" +
			"  spr cited-by W3014596384 --limit 0\n" +
			"  spr cited-by 10.1007/s10994-021-05946-3 -o json | jq -r '.works[].doi' | spr work",
	}

	var (
		byYear bool
		limit  int
	)
	cmd.Flags().BoolVar(&byYear, "by-year", false, "counts grouped by publication year, one request instead of a full listing")
	cmd.Flags().IntVar(&limit, "limit", 50, "how many citing works to list, 0 for every one of them at 200 per request")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
		c := client(errw)

		id, err := openAlexID(cmd, c, args[0])
		if err != nil {
			return err
		}

		if byYear {
			years, total, err := c.OpenAlexCitedByYear(cmd.Context(), id)
			if err != nil {
				return indexError(err)
			}
			if g.format == "json" {
				if err := encode(out, map[string]any{"id": id, "openalex_cited_by": total, "by_year": years}); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(out, "%s cited by %s works, as OpenAlex counts them today\n", id, group(total))
				for _, y := range years {
					fmt.Fprintf(out, "  %d  %s\n", y.Year, group(y.Count))
				}
			}
			if total == 0 {
				return exit(CodeNoData, nil)
			}
			return nil
		}

		works, total, err := c.OpenAlexCitedBy(cmd.Context(), id, limit)
		if err != nil {
			return indexError(err)
		}
		if g.format == "json" {
			if err := encode(out, map[string]any{"id": id, "openalex_cited_by": total, "works": works}); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(out, "%s cited by %s works, showing %d, counted live by OpenAlex\n", id, group(total), len(works))
			for i, w := range works {
				fmt.Fprintf(out, "\n%3d  %s\n", i+1, wrap(w.Title, 70, "     "))
				if line := citingFacts(w); line != "" {
					fmt.Fprintf(out, "     %s\n", line)
				}
				if w.DOI != "" {
					fmt.Fprintf(out, "     %s\n", w.DOI)
				}
			}
		}
		if len(works) == 0 {
			return exit(CodeNoData, nil)
		}
		return nil
	}
	return cmd
}

// openAlexID turns whatever the caller typed into an OpenAlex work id, looking
// the DOI up when it has to. A W id costs nothing and a DOI costs one request,
// which is worth saying because cited-by on a DOI is two requests and not one.
func openAlexID(cmd *cobra.Command, c *spr.Client, arg string) (string, error) {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(arg)), "W") {
		return spr.ShortOpenAlexID(arg), nil
	}
	doi, err := spr.ParseDOI(arg)
	if err != nil {
		return "", exit(CodeUsage, fmt.Errorf("%q is neither a doi nor an openalex work id: %w", arg, err))
	}
	w, err := c.OpenAlexWork(cmd.Context(), doi)
	if err != nil {
		return "", indexError(err)
	}
	if w.ID == "" {
		return "", exit(CodeNoData, fmt.Errorf("openalex has a record for %s and no work id on it, so there is nothing to ask the citation index with", doi))
	}
	return w.ID, nil
}

func apiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api [terms]",
		Short: "Query the Springer Nature API, which needs a key",
		Long: "api queries the Springer Nature API, which is the publisher's own service rather\n" +
			"than the web site, and it is the only surface here that needs a credential.\n\n" +
			"The key is read from " + spr.KeyEnv + " or from the config file, and it is never printed,\n" +
			"never cached and never written to a record. It travels in the query string because\n" +
			"that is the only way these endpoints accept one, and everything downstream of the\n" +
			"request sees the url with the key blanked out.\n\n" +
			"There are three endpoints. /meta/v2 is the current metadata one, /metadata is the\n" +
			"older one, and /openaccess adds full text for the works that have it.\n\n" +
			"Every field this command decodes comes from the published schema and not from a\n" +
			"measured response, because no key was available to measure one with. The envelope\n" +
			"lists every top level key of the answer that this decoder did not read, so the first\n" +
			"run with a real key reports the gap rather than hiding it.",
		Args: cobra.MaximumNArgs(1),
		Example: "  spr api \"aleatoric uncertainty\"\n" +
			"  spr api --doi 10.1007/s10994-021-05946-3\n" +
			"  spr api --issn 0885-6125 --year 2021 --rows 50\n" +
			"  spr api --endpoint openaccess --keyword \"machine learning\"\n" +
			"  spr api -o json --doi 10.1007/s10994-021-05946-3 | jq .envelope.unread",
	}

	var (
		q         spr.SpringerQuery
		endpoint  string
		doi, issn string
		isbn      string
		envelope  bool
	)

	f := cmd.Flags()
	f.StringVar(&endpoint, "endpoint", string(spr.EndpointMetaV2), "which endpoint: "+strings.Join(spr.SpringerEndpoints, ", "))
	f.StringVar(&doi, "doi", "", "one work by doi")
	f.StringVar(&issn, "issn", "", "one journal by issn")
	f.StringVar(&isbn, "isbn", "", "one book by isbn")
	f.StringVar(&q.Title, "title", "", "title contains, quoted for you when it has a space")
	f.StringVar(&q.Keyword, "keyword", "", "keyword, quoted for you when it has a space")
	f.StringVar(&q.Subject, "subject", "", "the publisher's own subject vocabulary")
	f.StringVar(&q.Type, "type", "", "the publisher's own content type vocabulary")
	f.StringVar(&q.Year, "year", "", "publication year")
	f.StringVar(&q.DateFrom, "from", "", "earliest publication date, as yyyy-mm-dd")
	f.StringVar(&q.DateTo, "to", "", "latest publication date, as yyyy-mm-dd")
	f.IntVar(&q.Start, "start", 0, "first record, one based rather than zero based")
	f.IntVar(&q.Rows, "rows", 20, "records per page")
	f.BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			q.Free = args[0]
		}
		ep, err := spr.ParseEndpoint(endpoint)
		if err != nil {
			return exit(CodeUsage, err)
		}
		q.Endpoint = ep

		if doi != "" {
			parsed, err := spr.ParseDOI(doi)
			if err != nil {
				return exit(CodeUsage, err)
			}
			q.DOI = parsed
		}
		if issn != "" {
			parsed, err := spr.ParseISSN(issn)
			if err != nil {
				return exit(CodeUsage, err)
			}
			q.ISSN = parsed
		}
		if isbn != "" {
			parsed, err := spr.ParseISBN(isbn)
			if err != nil {
				return exit(CodeUsage, err)
			}
			q.ISBN = string(parsed)
		}
		if q.Q() == "" {
			return exit(CodeUsage, errors.New("api needs terms, or one of --doi, --issn, --isbn, --title, --keyword, --subject, --type or a date"))
		}

		out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
		res, err := client(errw).SpringerSearch(cmd.Context(), q)
		if err != nil {
			if errors.Is(err, spr.ErrNoKey) {
				return exit(CodeUsage, err)
			}
			return indexError(err)
		}

		if g.format == "json" {
			if err := encode(out, res); err != nil {
				return err
			}
		} else {
			printSpringerAPI(out, res, envelope)
		}
		if len(res.Records) == 0 {
			return exit(CodeNoData, nil)
		}
		return nil
	}
	return cmd
}

// indexError maps a failed backend call onto an exit code.
//
// A 404 from an index is a fact about the identifier rather than a broken run,
// so it exits with the no data code and prints what the host said. Everything
// else is a transport failure, because these hosts serve no challenge and have
// no paywall.
func indexError(err error) error {
	var no *spr.NoRecord
	if errors.As(err, &no) {
		return exit(CodeNoData, err)
	}
	return exit(CodeTransport, err)
}

func crossrefExit(res *spr.CrossrefResult) error {
	if len(res.Items) == 0 && len(res.Facets) == 0 {
		return exit(CodeNoData, nil)
	}
	return nil
}

func openAlexExit(res *spr.OpenAlexResult) error {
	if len(res.Items) == 0 {
		return exit(CodeNoData, nil)
	}
	return nil
}

// printCrossrefWork writes the human form of a deposit.
func printCrossrefWork(out io.Writer, w *spr.CrossrefWork, withEnvelope bool) {
	line := field(out)
	line("doi", string(w.DOI))
	line("type", w.Type)
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
	if w.Pages != "" {
		container += " pp " + w.Pages
	}
	line("published in", strings.TrimSpace(container))
	line("publisher", w.Publisher)

	// The ISSNs are printed with their medium, because the untyped array these
	// came from does not say which of the two numbers is which and a journal
	// has one of each.
	for _, s := range w.ISSNs {
		line("issn", strings.TrimSpace(string(s.Value)+" "+s.Type))
	}
	if len(w.ISBNs) > 0 {
		line("isbn", strings.Join(w.ISBNs, ", "))
	}

	// Five dates, kept apart, because they are five different claims and four
	// of them were a month while one was a day on the article measured.
	for _, d := range []struct {
		label string
		value *spr.Date
	}{
		{"issued", w.Issued},
		{"published", w.Published},
		{"online", w.PublishedOnline},
		{"print", w.PublishedPrint},
		{"deposited", w.Deposited},
	} {
		if d.value != nil {
			line(d.label, d.value.String())
		}
	}

	printCrossrefPeople(out, "authors", w.Authors)
	printCrossrefPeople(out, "editors", w.Editors)

	if len(w.Funders) > 0 {
		fmt.Fprintf(out, "\nfunders (%d)\n", len(w.Funders))
		for _, fu := range w.Funders {
			held := fu.Name
			if fu.DOI == "" {
				// A funder with no id joins to nothing, and saying so is more
				// use to somebody building a graph than a blank column.
				held += "  (no funder id deposited)"
			} else {
				held += "  " + fu.DOI
			}
			if len(fu.Awards) > 0 {
				held += "  awards " + strings.Join(fu.Awards, ", ")
			}
			fmt.Fprintf(out, "  %s\n", held)
		}
	}

	if len(w.Licenses) > 0 {
		fmt.Fprintf(out, "\nlicenses (%d)\n", len(w.Licenses))
		for _, l := range w.Licenses {
			held := l.URL
			if l.Version != "" {
				held += "  " + l.Version
			}
			held += fmt.Sprintf("  from day %d", l.DelayInDays)
			fmt.Fprintf(out, "  %s\n", held)
		}
	}
	if len(w.Links) > 0 {
		fmt.Fprintf(out, "\nlinks (%d)\n", len(w.Links))
		for _, l := range w.Links {
			fmt.Fprintf(out, "  %s  %s  %s\n", l.Application, l.ContentType, l.URL)
		}
	}
	if len(w.Subjects) > 0 {
		fmt.Fprintf(out, "\nsubjects     %s\n", strings.Join(w.Subjects, ", "))
	}
	if w.Abstract != "" {
		fmt.Fprintf(out, "\nabstract\n  %s\n", wrap(w.Abstract, 76, "  "))
	}

	fmt.Fprintf(out, "\ncounts\n")
	fmt.Fprintf(out, "  crossref_citations             %s, deposited citations only\n", group(w.Counts.Citations))
	fmt.Fprintf(out, "  crossref_references            %s deposited\n", group(w.Counts.References))
	fmt.Fprintf(out, "  crossref_references_with_doi   %s of those resolve to something\n", group(w.Counts.ReferencesWithDOI))

	printEnvelope(out, w.Envelope, withEnvelope)
}

func printCrossrefPeople(out io.Writer, label string, people []spr.CrossrefPerson) {
	if len(people) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s (%d)\n", label, len(people))
	for i, p := range people {
		fmt.Fprintf(out, "  %2d  %s", i+1, p.Display())
		if p.Sequence != "" {
			fmt.Fprintf(out, "  %s", p.Sequence)
		}
		if p.ORCID != "" {
			// Whether the id was authenticated is printed every time it is
			// present, because an id the publisher typed in and an id the
			// person signed in to attach are not the same evidence.
			state := "unauthenticated"
			if p.ORCIDAuthenticated {
				state = "authenticated"
			}
			fmt.Fprintf(out, "  %s (%s)", p.ORCID, state)
		}
		fmt.Fprintln(out)
		for _, af := range p.Affiliations {
			fmt.Fprintf(out, "      %s\n", af)
		}
	}
}

// printCrossrefResults writes a page of a query.
func printCrossrefResults(out io.Writer, res *spr.CrossrefResult, withEnvelope bool) {
	fmt.Fprintf(out, "%s results, showing %d, via crossref\n", group(res.Total), len(res.Items))
	for i, w := range res.Items {
		fmt.Fprintf(out, "\n%3d  %s\n", i+1, wrap(w.Title, 70, "     "))
		var parts []string
		if w.Type != "" {
			parts = append(parts, w.Type)
		}
		if w.ContainerTitle != "" {
			parts = append(parts, w.ContainerTitle)
		}
		if w.Issued != nil {
			parts = append(parts, w.Issued.String())
		}
		if len(w.Authors) > 0 {
			names := make([]string, 0, len(w.Authors))
			for _, p := range w.Authors {
				names = append(names, p.Display())
			}
			parts = append(parts, authorLine(names))
		}
		parts = append(parts, fmt.Sprintf("cited %s times by crossref's count", group(w.Counts.Citations)))
		fmt.Fprintf(out, "     %s\n", wrap(strings.Join(parts, ", "), 70, "     "))
		if w.DOI != "" {
			fmt.Fprintf(out, "     %s\n", w.DOI)
		}
	}
	if res.NextCursor != "" {
		fmt.Fprintf(out, "\nnext cursor  %s\n", res.NextCursor)
	}
	printEnvelope(out, res.Envelope, withEnvelope)
}

// printCrossrefFacets writes the group counts, which is the cheap way to see
// the shape of a result set before deciding whether to page through it.
func printCrossrefFacets(out io.Writer, res *spr.CrossrefResult) {
	fmt.Fprintf(out, "%s results\n", group(res.Total))
	for _, f := range res.Facets {
		var parts []string
		for _, v := range f.Values {
			parts = append(parts, fmt.Sprintf("%s %s", v.Label, group(v.Count)))
		}
		fmt.Fprintf(out, "\n%-30s%s\n", fmt.Sprintf("%s (%d values)", f.Name, f.ValueCount),
			wrap(strings.Join(parts, ", "), 48, strings.Repeat(" ", 30)))
	}
}

// printOpenAlexWork writes the human form of an index record.
func printOpenAlexWork(out io.Writer, w *spr.OpenAlexWork, withEnvelope bool) {
	line := field(out)
	line("id", w.ID)
	line("doi", string(w.DOI))
	line("type", w.Type)
	line("title", w.Title)
	line("language", w.Language)
	line("published", w.PublicationDate)

	if w.Source != nil {
		container := w.Source.DisplayName
		switch {
		case w.Volume != "" && w.Issue != "":
			container += fmt.Sprintf(" %s(%s)", w.Volume, w.Issue)
		case w.Volume != "":
			container += " " + w.Volume
		}
		if w.Pages != "" {
			container += " pp " + w.Pages
		}
		line("published in", strings.TrimSpace(container))
		line("publisher", w.Source.Publisher)
		// The linking ISSN is named separately because it is the one number
		// that identifies the title across print and electronic rather than
		// identifying a medium.
		line("issn-l", string(w.Source.ISSNL))
		if len(w.Source.ISSNs) > 0 {
			held := make([]string, 0, len(w.Source.ISSNs))
			for _, s := range w.Source.ISSNs {
				held = append(held, string(s))
			}
			line("issn", strings.Join(held, ", "))
		}
	}
	if w.OpenAccess != nil {
		held := "closed"
		if w.OpenAccess.IsOA {
			held = "open"
		}
		line("access", strings.TrimSpace(held+", "+w.OpenAccess.Status))
		line("free at", w.OpenAccess.URL)
	}

	if len(w.Authors) > 0 {
		fmt.Fprintf(out, "\nauthors (%d)\n", len(w.Authors))
		for i, a := range w.Authors {
			fmt.Fprintf(out, "  %2d  %s", i+1, a.Name)
			if a.Position != "" {
				fmt.Fprintf(out, "  %s", a.Position)
			}
			if a.ORCID != "" {
				fmt.Fprintf(out, "  %s", a.ORCID)
			}
			if a.IsCorresponding {
				fmt.Fprint(out, "  corresponding")
			}
			fmt.Fprintln(out)
			for _, in := range a.Institutions {
				held := in.DisplayName
				if in.ROR != "" {
					held += "  " + string(in.ROR)
				}
				if in.CountryCode != "" {
					held += "  " + in.CountryCode
				}
				fmt.Fprintf(out, "      %s\n", held)
			}
		}
	}

	printTags(out, "concepts", w.Concepts)
	printTags(out, "topics", w.Topics)

	if w.Abstract != "" {
		fmt.Fprintf(out, "\nabstract\n  %s\n", wrap(w.Abstract, 76, "  "))
	}

	fmt.Fprintf(out, "\ncounts\n")
	stored := fmt.Sprintf("  openalex_citations             %s", group(w.Counts.Citations))
	if w.Counts.UpdatedDate != "" {
		// The date is printed next to the number rather than under it, because
		// the number is a stored aggregate and the date is the only thing that
		// says how old it is.
		stored += ", as stored on " + w.Counts.UpdatedDate
	}
	fmt.Fprintln(out, stored)
	fmt.Fprintf(out, "  openalex_references            %s resolved to works in the index\n", group(w.Counts.References))
	if w.Counts.FWCI > 0 {
		fmt.Fprintf(out, "  fwci                           %.2f, against the average work of its field, year and type\n", w.Counts.FWCI)
	}
	if w.Counts.Percentile > 0 {
		held := fmt.Sprintf("  percentile                     %.6f", w.Counts.Percentile)
		switch {
		case w.Counts.InTopOnePercent:
			held += ", in the top 1 percent"
		case w.Counts.InTopTenPercent:
			held += ", in the top 10 percent"
		}
		fmt.Fprintln(out, held)
	}
	if len(w.Counts.ByYear) > 0 {
		var parts []string
		for _, y := range w.Counts.ByYear {
			parts = append(parts, fmt.Sprintf("%d %s", y.Year, group(y.Count)))
		}
		fmt.Fprintf(out, "  by year                        %s\n", strings.Join(parts, ", "))
	}

	printEnvelope(out, w.Envelope, withEnvelope)
}

// printTags writes one of the two classifications. Both are printed when both
// are present, because OpenAlex publishes both, they disagree, and picking one
// would be picking a vocabulary on the reader's behalf.
func printTags(out io.Writer, label string, tags []spr.OpenAlexTag) {
	if len(tags) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s (%d)\n", label, len(tags))
	for _, t := range tags {
		held := fmt.Sprintf("  %-40s %.4f", t.Name, t.Score)
		if t.Level > 0 {
			held += fmt.Sprintf("  level %d", t.Level)
		}
		fmt.Fprintln(out, held)
	}
}

// printOpenAlexResults writes a page of a query, with the money on it.
func printOpenAlexResults(out io.Writer, res *spr.OpenAlexResult, withEnvelope bool) {
	head := fmt.Sprintf("%s results, showing %d, via openalex", group(res.Total), len(res.Items))
	if res.CostUSD > 0 {
		// The budget on this host is metered in dollars and the request count
		// is not what runs out first, so the price of the page is printed with
		// it rather than buried in a header nobody reads.
		head += fmt.Sprintf(", which cost $%.4f of the metered budget", res.CostUSD)
	}
	fmt.Fprintln(out, head)

	for i, w := range res.Items {
		fmt.Fprintf(out, "\n%3d  %s\n", i+1, wrap(w.Title, 70, "     "))
		var parts []string
		if w.Type != "" {
			parts = append(parts, w.Type)
		}
		if w.Source != nil && w.Source.DisplayName != "" {
			parts = append(parts, w.Source.DisplayName)
		}
		if w.PublicationDate != "" {
			parts = append(parts, w.PublicationDate)
		}
		if len(w.Authors) > 0 {
			names := make([]string, 0, len(w.Authors))
			for _, a := range w.Authors {
				names = append(names, a.Name)
			}
			parts = append(parts, authorLine(names))
		}
		parts = append(parts, fmt.Sprintf("cited %s times by openalex's count", group(w.Counts.Citations)))
		fmt.Fprintf(out, "     %s\n", wrap(strings.Join(parts, ", "), 70, "     "))
		if w.DOI != "" {
			fmt.Fprintf(out, "     %s\n", w.DOI)
		}
		fmt.Fprintf(out, "     %s\n", w.ID)
	}
	printEnvelope(out, res.Envelope, withEnvelope)
}

// citingFacts is the one line under a citing work's title. The listing asks for
// six fields and this prints the four of them that are worth a line.
func citingFacts(w spr.OpenAlexWork) string {
	var parts []string
	if w.Type != "" {
		parts = append(parts, w.Type)
	}
	if w.PublicationYear > 0 {
		parts = append(parts, fmt.Sprintf("%d", w.PublicationYear))
	}
	parts = append(parts, fmt.Sprintf("cited %s times itself", group(w.Counts.Citations)))
	parts = append(parts, w.ID)
	return strings.Join(parts, ", ")
}

// printSpringerAPI writes what the publisher's own API returned.
func printSpringerAPI(out io.Writer, res *spr.SpringerResult, withEnvelope bool) {
	fmt.Fprintf(out, "%s results from %s, showing %d from record %d\n",
		group(res.Total), res.Endpoint, len(res.Records), res.Start)

	for i, r := range res.Records {
		fmt.Fprintf(out, "\n%3d  %s\n", i+1, wrap(r.Title, 70, "     "))
		var parts []string
		if r.ContainerTitle != "" {
			parts = append(parts, r.ContainerTitle)
		}
		if r.PublicationDate != "" {
			parts = append(parts, r.PublicationDate)
		}
		if len(r.Authors) > 0 {
			parts = append(parts, authorLine(r.Authors))
		}
		if r.OpenAccess {
			parts = append(parts, "open access")
		}
		if line := strings.Join(parts, ", "); line != "" {
			fmt.Fprintf(out, "     %s\n", wrap(line, 70, "     "))
		}
		if r.DOI != "" {
			fmt.Fprintf(out, "     %s\n", r.DOI)
		}
	}
	printEnvelope(out, res.Envelope, withEnvelope)
}
