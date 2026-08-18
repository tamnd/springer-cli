package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tamnd/springer-cli/spr"
)

// Nothing in this file makes a request. The flag checks run before the client
// exists, --dry-run exists precisely so that a query can be inspected without
// being sent, and the printers take a record built here.

// The bad flag combinations, each of which is a request the site would answer
// with something other than what was asked for.
func TestSearchRefusesBadFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"nothing at all", []string{"search"}, "search needs terms"},
		{"facets with nothing to count", []string{"search", "--facets"}, "counting the whole index"},
		{"a range that runs backwards", []string{"search", "x", "--from", "2024", "--to", "2020"}, "is after"},
		{"both halves of one radio group", []string{"search", "x", "--last", "m6", "--from", "2020"}, "same radio group"},
		{"a window that is not offered", []string{"search", "x", "--last", "m9"}, "not one of m3, m6, m12, m24"},
		{"a path that is not a path", []string{"search", "x", "--path", "atom"}, "not rss or html"},
		{"no results wanted", []string{"search", "x", "--limit", "0"}, "asks for no results"},
		{"a page before the first", []string{"search", "x", "--page", "0"}, "before the first page"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := run(t, c.args...)
			if err == nil {
				t.Fatalf("accepted, and printed:\n%s", out)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error is %q, want it to mention %q", err, c.want)
			}
			var ee *ExitError
			if !errors.As(err, &ee) || ee.Code != CodeUsage {
				t.Errorf("exit code is not %d, and nothing was fetched", CodeUsage)
			}
		})
	}
}

// A filter with no terms is a real search. Everything published in German this
// quarter is a question, and refusing it because the terms box is empty would
// be this tool having an opinion the site does not have.
func TestSearchAcceptsAFilterWithNoTerms(t *testing.T) {
	out, err := run(t, "search", "--language", "De", "--last", "m3", "--dry-run")
	if err != nil {
		t.Fatalf("a filter only search was refused: %v", err)
	}
	if !strings.Contains(out, "last m3") {
		t.Errorf("the bill does not describe the query:\n%s", out)
	}
}

// The bill is the reason --dry-run exists. 500 results is 25 feed pages and one
// html page on a surface with a five second bucket, which is over two minutes,
// and that is worth knowing before it starts rather than after.
func TestSearchDryRunBills(t *testing.T) {
	out, err := run(t, "search", "uncertainty", "--limit", "500", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	for _, want := range []string{
		"query         uncertainty",
		"path          /search.rss, 20 per page",
		"25 rss pages + 1 html page for facets and the total",
		"1 request / 5s",
		"2 minutes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the bill does not say %q:\n%s", want, out)
		}
	}

	// And --facets is one request, which is the whole argument for it.
	out, err = run(t, "search", "uncertainty", "--facets", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "1 html page for facets and the total") || strings.Contains(out, "rss") {
		t.Errorf("a facet listing is one html request and no feed:\n%s", out)
	}
	if !strings.Contains(out, "immediate") {
		t.Errorf("one request waits for nothing:\n%s", out)
	}
}

// --enrich pays for the card fields across the whole result set, and the bill
// has to show that it doubled the cost rather than leaving it to be discovered.
func TestSearchDryRunShowsWhatEnrichCosts(t *testing.T) {
	plain, err := run(t, "search", "uncertainty", "--limit", "100", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	rich, err := run(t, "search", "uncertainty", "--limit", "100", "--enrich", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, "5 rss pages + 1 html page") {
		t.Errorf("unenriched bill:\n%s", plain)
	}
	if !strings.Contains(rich, "5 rss pages + 5 html pages for the card fields") {
		t.Errorf("enriched bill:\n%s", rich)
	}
}

// The total is stated on the html page and nowhere else. A run that lost that
// page knows how many results it has and not how many there are, and the two
// printed the same way would be a lie about the second one.
func TestHeadlineDoesNotInventATotal(t *testing.T) {
	both := &spr.SearchResponse{Total: 557, Results: make([]spr.SearchResult, 20), Paths: []string{"rss", "html"}}
	if got, want := headline(both), "557 results, showing 20, via rss+html"; got != want {
		t.Errorf("headline = %q, want %q", got, want)
	}

	feedOnly := &spr.SearchResponse{Results: make([]spr.SearchResult, 20), Paths: []string{"rss"}}
	got := headline(feedOnly)
	if strings.Contains(got, "557") || !strings.Contains(got, "no total") {
		t.Errorf("headline = %q, and a run with no html page has no total to print", got)
	}
	if !strings.Contains(got, "20 results") {
		t.Errorf("headline = %q, and it still knows how many it holds", got)
	}
}

func TestAuthorLineCountsTheRest(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"Eyke Hüllermeier"}, "Eyke Hüllermeier"},
		{[]string{"A", "B", "C"}, "A, B, C"},
		{[]string{"A", "B", "C", "D", "E"}, "A, B, C and 2 more"},
	}
	for _, c := range cases {
		if got := authorLine(c.in); got != c.want {
			t.Errorf("authorLine(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A record printed twice prints the same bytes twice, so that a diff between
// two runs means the results changed.
func TestPrintSearchIsStable(t *testing.T) {
	res := searchFixture()
	var first bytes.Buffer
	printSearch(&first, res, false, true)
	for i := 0; i < 5; i++ {
		var again bytes.Buffer
		printSearch(&again, res, false, true)
		if again.String() != first.String() {
			t.Fatal("two runs of the same result set printed different bytes")
		}
	}

	out := first.String()
	for _, want := range []string{
		"557 results, showing 2, via rss+html",
		"Article, Machine Learning, 2021-03-08",
		"10.1007/s10994-021-05946-3",
		"open access",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the printed results do not say %q:\n%s", want, out)
		}
	}

	// The snippet is the card's truncated line and the abstract is the feed's
	// full one. Printing the truncation under the name abstract would make a
	// hundred and eighty characters look like a paper's summary, so the two are
	// printed in different places and only one of them at a time.
	if strings.Contains(out, "The abstract in full") {
		t.Errorf("the abstract printed without being asked for:\n%s", out)
	}
	var withAbstract bytes.Buffer
	printSearch(&withAbstract, res, true, false)
	if !strings.Contains(withAbstract.String(), "The abstract in full") {
		t.Errorf("--abstract did not print the abstract:\n%s", withAbstract.String())
	}
	if strings.Contains(withAbstract.String(), "truncated card line") {
		t.Errorf("--abstract printed the snippet as well:\n%s", withAbstract.String())
	}
}

// The facet listing carries counts, and the date group carries none, so the
// printer has to handle a group whose options are bare names.
func TestPrintFacets(t *testing.T) {
	var out bytes.Buffer
	printFacets(&out, &spr.SearchResponse{
		Total: 557,
		Facets: []spr.FacetGroup{
			{Group: "content-type", Items: []spr.Facet{
				{Label: "Article", Count: 557},
				{Label: "Research article", Count: 482},
			}},
			{Group: "date", Items: []spr.Facet{{Label: "Last 3 months"}}, Applied: []string{"dateFrom=2020", "dateTo=2024"}},
			{Group: "language"},
		},
	})
	got := out.String()
	for _, want := range []string{
		"557 results",
		"Article 557, Research article 482",
		"Last 3 months, applied: dateFrom=2020",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the facet listing does not say %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Last 3 months 0") {
		t.Errorf("a group with no counts printed zeros:\n%s", got)
	}
	if strings.Contains(got, "language") {
		t.Errorf("an empty group printed a heading and nothing under it:\n%s", got)
	}
}

// A challenge is a rate and not a refusal, so it gets its own exit code and a
// script can tell that waiting is the fix.
func TestSearchExitCodes(t *testing.T) {
	var errw bytes.Buffer
	err := searchError(&errw, fmt.Errorf("%w on the first feed page", spr.ErrChallenged))
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeChallenged {
		t.Errorf("a challenge exits %v, want %d", err, CodeChallenged)
	}
	if !strings.Contains(errw.String(), "/search.rss") {
		t.Errorf("the challenge was reported without saying what to do about it:\n%s", errw.String())
	}

	if err := searchError(&errw, errors.New("dial tcp: i/o timeout")); !errors.As(err, &ee) || ee.Code != CodeTransport {
		t.Errorf("a network failure exits %v, want %d", err, CodeTransport)
	}

	// A query that matched nothing is not a failure and is not a success
	// either. It is a third thing, and a pipeline should be able to branch on it
	// without parsing the output.
	empty := &spr.SearchResponse{Paths: []string{"rss", "html"}}
	if err := searchExit(empty, false); !errors.As(err, &ee) || ee.Code != CodeNoData {
		t.Errorf("no results exits %v, want %d", err, CodeNoData)
	}
	if err := searchExit(searchFixture(), false); err != nil {
		t.Errorf("a search that found things exits %v", err)
	}
}

func searchFixture() *spr.SearchResponse {
	day := func(s string) *spr.Date {
		d, err := spr.ParseDate(s)
		if err != nil {
			panic(err)
		}
		return &d
	}
	return &spr.SearchResponse{
		Total:   557,
		Page:    1,
		PerPage: 20,
		Paths:   []string{"rss", "html"},
		Results: []spr.SearchResult{
			{
				Position:    1,
				DOI:         "10.1007/s10994-021-05946-3",
				Title:       "Aleatoric and epistemic uncertainty in machine learning",
				Abstract:    "The abstract in full, as the feed carries it.",
				Snippet:     "The truncated card line, as the html carries it ...",
				URL:         "https://link.springer.com/article/10.1007/s10994-021-05946-3",
				Published:   day("2021-03-08"),
				ContentType: "Article",
				Authors:     []string{"Eyke Hüllermeier", "Willem Waegeman"},
				Container:   &spr.Ref{Kind: "journal", Name: "Machine Learning"},
				OpenAccess:  true,
				Via:         "rss+html",
			},
			{
				Position: 2,
				DOI:      "10.1038/s44334-024-00011-y",
				Title:    "A result the feed had and the html page did not",
				URL:      "https://www.nature.com/articles/s44334-024-00011-y",
				Via:      "rss",
			},
		},
		Envelope: spr.Envelope{Tier: "search", Status: spr.StatusOK},
	}
}
