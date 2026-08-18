package spr

import (
	"regexp"
	"strings"
	"testing"
)

// The query builder, against the site's own form.
//
// Every expectation below is either a value read off the stored search page or
// a rule that page states about itself. The quoting is the reason this file is
// longer than the code it tests: a taxonomy value sent without its quotes is
// accepted, answers 200, and matches nothing, so the failure mode is an empty
// result set that looks like an honest answer.

func TestQueryValues(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		want  string
	}{
		{
			"the plain case",
			Query{Terms: "aleatoric uncertainty"},
			"query=aleatoric+uncertainty",
		},
		{
			"the capture's own query",
			Query{Terms: "aleatoric uncertainty", Types: []string{"article"}, From: "2020", To: "2024", Sort: "relevance"},
			"content-type=Article&date=custom&dateFrom=2020&dateTo=2024&query=aleatoric+uncertainty&sortBy=relevance",
		},
		{
			"taxonomy carries its quotes",
			Query{Taxonomy: []string{"Machine Learning"}},
			`taxonomy=%22Machine+Learning%22`,
		},
		{
			"a value that arrives already quoted is not quoted twice",
			Query{Taxonomy: []string{`"Machine Learning"`}},
			`taxonomy=%22Machine+Learning%22`,
		},
		{
			"discipline and sub-discipline post under their own names",
			Query{Disciplines: []string{"Computer Science"}, SubDisciplines: []string{"Artificial Intelligence"}},
			`facet-discipline=%22Computer+Science%22&facet-sub-discipline=%22Artificial+Intelligence%22`,
		},
		{
			"the sdg parameter is not named after its group",
			Query{SDGs: []string{"Affordable and clean energy"}},
			`sustainableDevelopmentGoal=%22Affordable+and+clean+energy%22`,
		},
		{
			"open access is a publishing model facet under a different parameter",
			Query{OpenAccess: true},
			"openAccess=true",
		},
		{
			"language is unquoted and is a two letter code",
			Query{Languages: []string{"En", "De"}},
			"language=En&language=De",
		},
		{
			"repeated facets repeat the parameter",
			Query{Taxonomy: []string{"Machine Learning", "Bayesian Inference"}},
			`taxonomy=%22Machine+Learning%22&taxonomy=%22Bayesian+Inference%22`,
		},
		{
			"a relative window is a date value of its own",
			Query{Terms: "x", Last: "m12"},
			"date=m12&query=x",
		},
		{
			// The form offers the windows and the custom range as one radio
			// group, so a request cannot hold both. Somebody who typed two
			// years meant them.
			"a custom range beats a relative window",
			Query{Last: "m12", From: "2020", To: "2024"},
			"date=custom&dateFrom=2020&dateTo=2024",
		},
		{
			"one open ended year is still a custom range",
			Query{From: "2020"},
			"date=custom&dateFrom=2020",
		},
		{
			"the field scoped inputs travel as ordinary parameters",
			Query{Title: "uncertainty", Contributor: "Hüllermeier", Journal: "Machine Learning"},
			"contributor=H%C3%BCllermeier&journal=Machine+Learning&title=uncertainty",
		},
		{
			"sort takes the word a person would type and sends the one the form posts",
			Query{Terms: "x", Sort: "date"},
			"query=x&sortBy=newestFirst",
		},
		{
			"page one is the absence of a page parameter",
			Query{Terms: "x", Page: 1},
			"query=x",
		},
		{
			"page two is not",
			Query{Terms: "x", Page: 2},
			"page=2&query=x",
		},
		{
			"an empty query is an empty query string, not a query of empty strings",
			Query{Terms: "  ", Types: []string{"", " "}, Taxonomy: []string{" "}},
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.query.Values().Encode(); got != tc.want {
				t.Errorf("\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// The three content types whose posted value is not their printed label. A
// filter built from the label matches nothing on these and works on the other
// thirteen, which is the least detectable kind of wrong.
func TestContentTypeValueIsNotTheLabel(t *testing.T) {
	for _, tc := range []struct{ typed, want string }{
		{"article", "Article"},
		{"Article", "Article"},
		{"research", "Research"},
		{"research article", "Research"},
		{"review article", "Review"},
		{"news article", "News"},
		{"reference work entry", "Reference Work Entry"},
		{"entry", "Reference Work Entry"},
		{"conference paper", "Conference Paper"},
		{"chapter", "Chapter"},
		{"book", "Book"},
		{"protocol", "Protocol"},
		{"video segment", "Video Segment"},
		// Anything unrecognized goes through untouched, so that a type
		// Springer adds tomorrow is usable today without a release.
		{"Whatever Springer Ships Next", "Whatever Springer Ships Next"},
	} {
		if got := contentType(tc.typed); got != tc.want {
			t.Errorf("--type %q sent content-type=%q, want %q", tc.typed, got, tc.want)
		}
	}
}

// facetInputRe reads the parameter name and the value off the search page's own
// facet checkboxes.
var facetInputRe = regexp.MustCompile(`<input[^>]*\bname="([^"]*)"[^>]*\bvalue="([^"]*)"[^>]*data-test="([a-z-]+)-facet-item"`)

// The quoting rule is not asserted from the spec here. It is read off the
// stored page, which states for every facet the exact parameter and the exact
// value the site would send, quotes and all. If Springer ever drops the quotes
// this test fails and the code follows the site rather than the comment.
func TestQuotingMatchesTheCapture(t *testing.T) {
	body := string(capturedResponse(t, "search.html").Body)

	// param -> a value the page says it sends under it.
	sent := map[string]string{}
	for _, m := range facetInputRe.FindAllStringSubmatch(body, -1) {
		name, value := m[1], strings.ReplaceAll(m[2], "&quot;", `"`)
		if _, seen := sent[name]; !seen {
			sent[name] = value
		}
	}
	if len(sent) == 0 {
		t.Fatal("the capture carries no facet inputs, so this test is asserting nothing")
	}

	// Each of these is a facet value as a person would type it, and the
	// parameter the page says it goes out under.
	for _, tc := range []struct {
		param string
		query Query
	}{
		{"taxonomy", Query{Taxonomy: []string{"Machine Learning"}}},
		{"facet-discipline", Query{Disciplines: []string{"Computer Science"}}},
		{"facet-sub-discipline", Query{SubDisciplines: []string{"Artificial Intelligence"}}},
		{"sustainableDevelopmentGoal", Query{SDGs: []string{"Affordable and clean energy"}}},
		{"content-type", Query{Types: []string{"Article"}}},
		{"language", Query{Languages: []string{"En"}}},
		{"openAccess", Query{OpenAccess: true}},
	} {
		pageValue, ok := sent[tc.param]
		if !ok {
			t.Errorf("the capture has no facet posting under %s", tc.param)
			continue
		}
		got := tc.query.Values()[tc.param]
		if len(got) != 1 {
			t.Errorf("%s: the builder sent %d values, want 1", tc.param, len(got))
			continue
		}
		// The page's first value for a parameter is not always the one this
		// query asks for, so the check is on the shape: quoted parameters get
		// quotes and unquoted ones do not.
		wantQuoted := strings.HasPrefix(pageValue, `"`)
		gotQuoted := strings.HasPrefix(got[0], `"`)
		if wantQuoted != gotQuoted {
			t.Errorf("%s: page sends %q, builder sends %q, and one of them has quotes the other does not", tc.param, pageValue, got[0])
		}
	}
}

// The sort orders in this test are the option values on the page's own select,
// and the aliases are what somebody would actually type.
func TestSortValues(t *testing.T) {
	for typed, want := range map[string]string{
		"relevance": "relevance",
		"date":      "newestFirst",
		"newest":    "newestFirst",
		"oldest":    "oldestFirst",
		"":          "",
	} {
		if got := sortValue(typed); got != want {
			t.Errorf("--sort %q sent sortBy=%q, want %q", typed, got, want)
		}
	}

	// The feed ignores sortBy entirely. Measured with the parameter absent,
	// with relevance and with newestFirst against the same query in the same
	// minute: the same twenty items in the same order every time, newest
	// first, first guid 10.1007/s00193-024-01208-y. So a run that falls back to
	// the feed with --sort relevance is handing back an order nobody asked for,
	// and this is the check that lets the command say so.
	if (Query{Sort: "relevance"}).SortsFeed() {
		t.Error("relevance is claimed to survive the feed, and it does not")
	}
	for _, s := range []string{"", "date", "newest", "newestFirst"} {
		if !(Query{Sort: s}).SortsFeed() {
			t.Errorf("--sort %q is claimed not to survive the feed, and newest first is the only order it has", s)
		}
	}
}

func TestSearchAndFeedURLsDifferOnlyInPath(t *testing.T) {
	q := Query{Terms: "aleatoric uncertainty", Types: []string{"article"}, From: "2020", To: "2024"}
	html, feed := q.SearchURL(), q.FeedURL()
	if strings.Replace(html, "/search?", "/search.rss?", 1) != feed {
		t.Errorf("the two paths built different queries\n html %s\n feed %s", html, feed)
	}
}
