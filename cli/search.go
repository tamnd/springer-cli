package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// spr search.
//
// The command is thin on purpose. Every decision worth arguing about lives in
// the spr package: which path runs first, how the two are joined, what quoting
// a facet value needs. What is left here is turning flags into a Query, turning
// a run into text or json, and turning a failure into an exit code a script can
// branch on.
//
// The one thing this file does decide is when to say something on stderr. A
// search that lost its facets to a challenge, or that ran with a sort the feed
// ignores, has printed results that are correct and incomplete in a way the
// results themselves cannot show. Those go to stderr as they happen, once each,
// and into the record so a json consumer gets the same news.

// billAt is the request count above which a run prints its bill before making
// it.
//
// It is not a prompt. Prompting breaks every pipeline this tool is meant to sit
// in, and a run of 26 requests at a five second pace is two minutes of somebody
// waiting without being told why. So the bill is printed and the run proceeds,
// and --dry-run is there for anyone who wants the bill and nothing else.
const billAt = 5

func searchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [terms]",
		Short: "Search link.springer.com, on both of the paths it answers on",
		Long: "search runs one query against the two surfaces Springer serves it on and returns one\n" +
			"merged answer.\n\n" +
			"/search.rss is the primary path. It pages to the end of the result set, carries the\n" +
			"full abstract on every item rather than a truncated card, states the bare doi in guid,\n" +
			"and it kept answering while the html surface was serving challenges.\n\n" +
			"/search html is the enrichment pass. It is the only source of the total, the facet\n" +
			"counts, and the per result content type, container and author list. One page of it is\n" +
			"fetched for those, and --enrich fetches enough pages to cover the whole result set.\n\n" +
			"The two do not agree on what the first twenty results are. Fetched for the same query\n" +
			"in the same minute, html page 1 and rss page 1 share 3 of 20, because the html honours\n" +
			"sortBy and the feed ignores it and always answers newest first. So the join is on doi\n" +
			"and never on position, every result says which path it came from, and a result built\n" +
			"from both says rss+html.\n\n" +
			"The taxonomy, discipline, sub-discipline and sdg values have to reach the site wrapped\n" +
			"in double quotes. Sending them bare is a valid request that answers 200 and matches\n" +
			"nothing, so the quotes are added for you and --taxonomy \"Machine Learning\" is what you\n" +
			"type.\n\n" +
			"A query that matched nothing exits 3, so that no results and a failed run are two\n" +
			"different things to a script without either of them being parsed out of the output.",
		Args: cobra.MaximumNArgs(1),
		Example: "  spr search \"aleatoric uncertainty\"\n" +
			"  spr search \"aleatoric uncertainty\" --type article --from 2020 --to 2024\n" +
			"  spr search \"uncertainty\" --journal \"Machine Learning\" --open-access\n" +
			"  spr search \"graph neural network\" --taxonomy \"Machine Learning\" --sort date --limit 500\n" +
			"  spr search --title \"uncertainty\" --contributor \"Hüllermeier\"\n" +
			"  spr search \"climate\" --sdg \"Climate action\" --facets\n" +
			"  spr search \"uncertainty\" --limit 500 --dry-run\n" +
			"  spr search -o json \"uncertainty\" | jq -r '.results[] | \"\\(.doi)\\t\\(.title)\"'",
	}

	var (
		q        spr.Query
		limit    int
		path     string
		enrich   bool
		facets   bool
		abstract bool
		dryRun   bool
		envelope bool
	)

	f := cmd.Flags()
	f.StringVar(&q.Title, "title", "", "title contains, from advanced search")
	f.StringVar(&q.Contributor, "contributor", "", "author or editor name, from advanced search")
	f.StringVar(&q.Journal, "journal", "", "journal or book title, from advanced search")
	f.StringArrayVar(&q.Types, "type", nil, "content type, repeatable: article, research, review, news, book, chapter, conference paper")
	f.BoolVar(&q.OpenAccess, "open-access", false, "open access only")
	f.StringArrayVar(&q.Languages, "language", nil, "language code, repeatable: En, De")
	f.StringArrayVar(&q.Taxonomy, "taxonomy", nil, "taxonomy term, repeatable, quoted for you")
	f.StringArrayVar(&q.Disciplines, "discipline", nil, "discipline, repeatable, quoted for you")
	f.StringArrayVar(&q.SubDisciplines, "sub-discipline", nil, "sub-discipline, repeatable, quoted for you")
	f.StringArrayVar(&q.SDGs, "sdg", nil, "sustainable development goal, repeatable, quoted for you")
	f.StringVar(&q.From, "from", "", "earliest publication year")
	f.StringVar(&q.To, "to", "", "latest publication year")
	f.StringVar(&q.Last, "last", "", "relative window instead of a year range: m3, m6, m12 or m24")
	f.StringVar(&q.Sort, "sort", "", "relevance, date or oldest, honoured by the html path only")
	f.IntVar(&q.Page, "page", 1, "first page of results, 20 per page")
	f.IntVar(&limit, "limit", spr.FeedPageSize, "how many results to return")
	f.StringVar(&path, "path", "", "force one surface: rss or html, default is both")
	f.BoolVar(&enrich, "enrich", false, "fetch the html card fields for every result rather than the first page")
	f.BoolVar(&facets, "facets", false, "print the facet groups and counts instead of the results, one request")
	f.BoolVar(&abstract, "abstract", false, "print each result's abstract, which the feed carries in full")
	f.BoolVar(&dryRun, "dry-run", false, "print what this query would cost and make no requests")
	f.BoolVar(&envelope, "envelope", false, "print the whole envelope: every field, its source, what was missed and what was left unread")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			q.Terms = args[0]
		}
		if err := checkQuery(q, facets); err != nil {
			return err
		}
		if q.From != "" && q.To != "" && q.From > q.To {
			return exit(CodeUsage, fmt.Errorf("--from %s is after --to %s", q.From, q.To))
		}
		if q.Last != "" && (q.From != "" || q.To != "") {
			return exit(CodeUsage, errors.New("--last and --from/--to are the same radio group on the form, so pick one"))
		}
		if err := checkWindow(q.Last); err != nil {
			return err
		}
		if err := checkPath(path); err != nil {
			return err
		}
		if limit < 1 {
			return exit(CodeUsage, fmt.Errorf("--limit %d asks for no results", limit))
		}
		if q.Page < 1 {
			return exit(CodeUsage, fmt.Errorf("--page %d is before the first page", q.Page))
		}

		out := cmd.OutOrStdout()
		errw := cmd.ErrOrStderr()

		rss, html := q.Requests(limit, enrich, facets)
		if dryRun {
			printBill(out, q, rss, html, facets)
			return nil
		}
		if rss+html > billAt {
			printBill(errw, q, rss, html, facets)
		}

		c := client(errw)
		res, err := c.Search(cmd.Context(), q, spr.SearchOptions{
			Limit:      limit,
			Path:       path,
			Enrich:     enrich,
			FacetsOnly: facets,
			Note:       func(s string) { fmt.Fprintln(errw, "search: "+s) },
		})
		if err != nil {
			return searchError(errw, err)
		}

		if g.format == "json" {
			if err := encode(out, res); err != nil {
				return err
			}
			return searchExit(res, facets)
		}
		if facets {
			printFacets(out, res)
			return searchExit(res, facets)
		}
		printSearch(out, res, abstract, envelope)
		return searchExit(res, facets)
	}
	return cmd
}

// checkQuery refuses a search with nothing in it.
//
// The site answers an empty query with the whole index, which is 21 million
// records and a request nobody meant to make. Every other empty flag is left
// out of the url rather than sent empty, and this is the one case where leaving
// everything out is a mistake worth naming.
func checkQuery(q spr.Query, facets bool) error {
	if q.Terms != "" || q.Title != "" || q.Contributor != "" || q.Journal != "" {
		return nil
	}
	if len(q.Types) > 0 || q.OpenAccess || len(q.Languages) > 0 || len(q.Taxonomy) > 0 ||
		len(q.Disciplines) > 0 || len(q.SubDisciplines) > 0 || len(q.SDGs) > 0 {
		return nil
	}
	if facets {
		return exit(CodeUsage, errors.New("--facets needs a query to count, otherwise it is counting the whole index"))
	}
	return exit(CodeUsage, errors.New("search needs terms, or one of --title, --contributor, --journal, or a filter"))
}

// The four relative windows the form offers, as it spells them.
var windows = []string{"m3", "m6", "m12", "m24"}

func checkWindow(s string) error {
	if s == "" {
		return nil
	}
	for _, w := range windows {
		if strings.EqualFold(s, w) {
			return nil
		}
	}
	return exit(CodeUsage, fmt.Errorf("--last %q is not one of %s", s, strings.Join(windows, ", ")))
}

func checkPath(s string) error {
	switch s {
	case "", spr.PathRSS, spr.PathHTML:
		return nil
	}
	return exit(CodeUsage, fmt.Errorf("--path %q is not rss or html", s))
}

// searchError maps a failed run onto an exit code.
//
// A challenge gets its own code because it is a rate and not a refusal, so the
// thing to do about it is wait and retry, and a script cannot tell that from a
// generic failure.
func searchError(errw io.Writer, err error) error {
	if errors.Is(err, spr.ErrChallenged) {
		fmt.Fprintln(errw, explain(spr.StatusChallenged))
		return exit(CodeChallenged, err)
	}
	if errors.Is(err, spr.ErrNotAFeed) || errors.Is(err, spr.ErrNotSearch) {
		return exit(CodeNoData, err)
	}
	return exit(CodeTransport, err)
}

// searchExit reports a query that matched nothing as its own exit code, so that
// a pipeline can branch on it without reading the output.
func searchExit(res *spr.SearchResponse, facets bool) error {
	if facets {
		if len(res.Facets) == 0 {
			return exit(CodeNoData, nil)
		}
		return nil
	}
	if len(res.Results) == 0 {
		return exit(CodeNoData, nil)
	}
	return nil
}

// printBill says what a query will cost before it is spent.
//
// The pace is the number that makes this worth printing. The search surface has
// a five second bucket of its own, and both paths share it, so 26 requests is
// over two minutes of wall clock and a person deserves to know that before it
// starts rather than after.
func printBill(out io.Writer, q spr.Query, rss, html int, facets bool) {
	line := field(out)
	line("query", queryLine(q))
	switch {
	case facets:
		line("path", "/search, facet counts only")
	case rss == 0:
		line("path", "/search, 20 per page")
	default:
		line("path", "/search.rss, 20 per page")
	}

	var parts []string
	if rss > 0 {
		parts = append(parts, fmt.Sprintf("%d rss %s", rss, plural(rss, "page")))
	}
	if html > 0 {
		what := "for facets and the total"
		if html > 1 {
			what = "for the card fields"
		}
		parts = append(parts, fmt.Sprintf("%d html %s %s", html, plural(html, "page"), what))
	}
	line("requests", strings.Join(parts, " + "))
	line("pace", fmt.Sprintf("1 request / %s, which both search paths share", spr.SearchPace))
	line("estimate", estimate(time.Duration(rss+html-1)*spr.SearchPace))
}

// queryLine writes the query back in one line, the way a person would say it,
// so the bill is about something recognizable rather than about a url.
func queryLine(q spr.Query) string {
	var parts []string
	if q.Terms != "" {
		parts = append(parts, q.Terms)
	}
	for _, f := range []struct{ label, value string }{
		{"title", q.Title}, {"contributor", q.Contributor}, {"journal", q.Journal},
	} {
		if f.value != "" {
			parts = append(parts, f.label+" "+f.value)
		}
	}
	for _, t := range q.Types {
		parts = append(parts, "type "+t)
	}
	if q.OpenAccess {
		parts = append(parts, "open access")
	}
	switch {
	case q.From != "" || q.To != "":
		parts = append(parts, joinNonEmpty(" to ", q.From, q.To))
	case q.Last != "":
		parts = append(parts, "last "+q.Last)
	}
	return strings.Join(parts, ", ")
}

// estimate rounds a duration to something worth reading. Nobody needs 2m5s.
func estimate(d time.Duration) string {
	switch {
	case d <= 0:
		return "immediate"
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Round(time.Second).Seconds()))
	}
	m := int(d.Round(time.Minute).Minutes())
	if m < 1 {
		m = 1
	}
	return fmt.Sprintf("%d %s", m, plural(m, "minute"))
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// printSearch writes the human form of a run.
func printSearch(out io.Writer, res *spr.SearchResponse, withAbstract, withEnvelope bool) {
	fmt.Fprintln(out, headline(res))
	for _, r := range res.Results {
		fmt.Fprintf(out, "\n%3d  %s\n", r.Position, wrap(r.Title, 70, "     "))

		if line := resultFacts(r); line != "" {
			fmt.Fprintf(out, "     %s\n", wrap(line, 70, "     "))
		}
		if r.DOI != "" {
			fmt.Fprintf(out, "     %s\n", r.DOI)
		}
		if r.URL != "" {
			fmt.Fprintf(out, "     %s\n", r.URL)
		}

		// The abstract is behind a flag because the feed carries the whole thing
		// and twenty of them is a screen nobody asked for. The snippet is the
		// card's truncated one and is printed in its place, since it is short by
		// construction.
		if withAbstract && r.Abstract != "" {
			fmt.Fprintf(out, "\n     %s\n", wrap(r.Abstract, 70, "     "))
		} else if !withAbstract && r.Snippet != "" {
			fmt.Fprintf(out, "     %s\n", wrap(r.Snippet, 70, "     "))
		}
	}
	printEnvelope(out, res.Envelope, withEnvelope)
}

// headline is the sentence above the results, and it is careful about the
// total.
//
// The total exists only on the html page. A run that lost that page to a
// challenge knows how many results it has and not how many there are, and those
// two numbers printed the same way would be a lie about the second one.
func headline(res *spr.SearchResponse) string {
	via := strings.Join(res.Paths, "+")
	if via == "" {
		via = "nothing"
	}
	if res.Total == 0 {
		return fmt.Sprintf("%d results, and no total, because that is stated on the html page only, via %s",
			len(res.Results), via)
	}
	return fmt.Sprintf("%s results, showing %d, via %s", group(res.Total), len(res.Results), via)
}

// resultFacts is the line under a title: what the thing is, where it came out,
// when, and whether it can be read.
func resultFacts(r spr.SearchResult) string {
	var parts []string
	if r.ContentType != "" {
		parts = append(parts, r.ContentType)
	}
	if r.Container != nil && r.Container.Name != "" {
		parts = append(parts, r.Container.Name)
	}
	if r.Published != nil && !r.Published.Zero() {
		parts = append(parts, r.Published.String())
	}
	if n := len(r.Authors); n > 0 {
		parts = append(parts, authorLine(r.Authors))
	}
	if r.OpenAccess {
		parts = append(parts, "open access")
	}
	if r.Access != "" {
		parts = append(parts, strings.ToLower(r.Access))
	}
	return strings.Join(parts, ", ")
}

// authorLine names the first three and counts the rest, because a paper with
// forty authors would otherwise be the only thing on the screen.
func authorLine(names []string) string {
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:3], ", "), len(names)-3)
}

// printFacets writes the counts, which is what --facets is for: seeing the
// shape of a result set for one request before deciding whether to fetch it.
func printFacets(out io.Writer, res *spr.SearchResponse) {
	if res.Total > 0 {
		fmt.Fprintf(out, "%s results\n", group(res.Total))
	}
	for _, fg := range res.Facets {
		var parts []string
		for _, item := range fg.Items {
			if item.Count > 0 {
				parts = append(parts, fmt.Sprintf("%s %s", item.Label, group(item.Count)))
				continue
			}
			// The date group prints no counts at all, so its options are listed
			// as bare names rather than as a name and a zero.
			parts = append(parts, item.Label)
		}
		if len(fg.Applied) > 0 {
			parts = append(parts, "applied: "+strings.Join(fg.Applied, " "))
		}
		if len(parts) == 0 {
			continue
		}
		fmt.Fprintf(out, "\n%-30s%s\n", fg.Group, wrap(strings.Join(parts, ", "), 48, strings.Repeat(" ", 30)))
	}
}
