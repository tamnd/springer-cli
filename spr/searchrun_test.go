package spr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// The two path run, against a server that behaves the way the site was measured
// behaving.
//
// The captures are the pages. The server decides which one to serve for which
// page number, and the interesting cases are the ones where it stops serving.

// searchServer answers /search and /search.rss out of the captures.
//
// pages is how many full feed pages exist before the short one, so a test can
// have a result set of any length without storing feeds for all of it. The two
// challenge switches are what the site actually did.
type searchServer struct {
	t             *testing.T
	pages         int
	challengeHTML bool
	challengeRSS  bool
	feedHits      int
	htmlHits      int
}

func (s *searchServer) start() (*Client, func()) {
	s.t.Helper()
	body := func(file string) []byte {
		for _, c := range append(append([]Capture{}, Captures...), feeds...) {
			if c.File == file {
				return load(s.t, c).Body
			}
		}
		s.t.Fatalf("no capture named %s", file)
		return nil
	}
	challenge := challengeBody(s.t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		switch r.URL.Path {
		case feedPath:
			s.feedHits++
			if s.challengeRSS {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write(challenge)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			switch {
			case page < s.pages:
				_, _ = w.Write(body("search.rss"))
			case page == s.pages:
				_, _ = w.Write(body("search_last.rss"))
			default:
				_, _ = w.Write(body("search_empty.rss"))
			}
		case searchPath:
			s.htmlHits++
			if s.challengeHTML {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write(challenge)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(body("search.html"))
		default:
			http.NotFound(w, r)
		}
	}))
	c := fast(s.t)
	c.base = srv.URL
	return c, srv.Close
}

func TestSearchRunsBothPaths(t *testing.T) {
	srv := &searchServer{t: t, pages: 3}
	c, stop := srv.start()
	defer stop()

	q := Query{Terms: "aleatoric uncertainty", Types: []string{"article"}, From: "2020", To: "2024"}
	res, err := c.Search(context.Background(), q, SearchOptions{Limit: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// One feed page and one html page is what twenty results costs.
	if srv.feedHits != 1 || srv.htmlHits != 1 {
		t.Errorf("%d feed and %d html requests, want 1 and 1", srv.feedHits, srv.htmlHits)
	}
	if res.Total != 557 {
		t.Errorf("total = %d, want the 557 only the html page states", res.Total)
	}
	if len(res.Facets) != 8 {
		t.Errorf("%d facet groups, want 8", len(res.Facets))
	}
	if len(res.Results) != 20 {
		t.Fatalf("%d results, want the 20 that were asked for", len(res.Results))
	}
	if strings.Join(res.Paths, "+") != "rss+html" {
		t.Errorf("paths = %v, want both", res.Paths)
	}

	// The join is on doi, and the two paths agree on 3 of 20 for this query, so
	// three results carry fields from both and the rest carry the feed's.
	var merged, rssOnly int
	for _, r := range res.Results {
		switch r.Via {
		case ViaRSS + "+" + ViaHTML:
			merged++
			if len(r.Authors) == 0 {
				t.Errorf("%s: merged and still has no authors", r.DOI)
			}
			if r.Abstract == "" {
				t.Errorf("%s: merged and lost the feed's abstract", r.DOI)
			}
		case ViaRSS:
			rssOnly++
		}
	}
	if merged != 3 {
		t.Errorf("%d results carry both paths, and the two agree on 3 of 20", merged)
	}
	if rssOnly != 17 {
		t.Errorf("%d results are feed only, want 17", rssOnly)
	}
}

// The failure the design exists for. The html surface is the one that
// challenges, and losing it costs the total, the facets and the card fields.
// It does not cost the results, and the run says so rather than returning less
// and looking healthy.
func TestSearchSurvivesAChallengedEnrichment(t *testing.T) {
	srv := &searchServer{t: t, pages: 3, challengeHTML: true}
	c, stop := srv.start()
	defer stop()

	var notes []string
	res, err := c.Search(context.Background(), Query{Terms: "uncertainty"}, SearchOptions{
		Limit: 20,
		Note:  func(s string) { notes = append(notes, s) },
	})
	if err != nil {
		t.Fatalf("a challenged enrichment ended the run: %v", err)
	}
	if len(res.Results) != 20 {
		t.Errorf("%d results, want the 20 the feed answered with", len(res.Results))
	}
	if res.Total != 0 {
		t.Errorf("total = %d, and no html page answered, so there is no total", res.Total)
	}
	if len(res.Facets) != 0 {
		t.Errorf("%d facet groups from a challenged html pass", len(res.Facets))
	}
	if strings.Join(res.Paths, "+") != "rss" {
		t.Errorf("paths = %v, want rss alone", res.Paths)
	}

	var said bool
	for _, n := range notes {
		if strings.Contains(n, "challenge") {
			said = true
		}
	}
	if !said {
		t.Errorf("the run lost its facets and did not say why: %v", notes)
	}
	if len(res.Notes) != len(notes) {
		t.Errorf("%d notes on the record and %d on stderr, and a json consumer gets neither more nor less", len(res.Notes), len(notes))
	}
}

// A challenged feed with no html to fall back on is a search that failed, and
// it has to say so rather than return an empty result set that reads as no
// matches.
func TestSearchFailsWhenBothPathsAreGone(t *testing.T) {
	srv := &searchServer{t: t, pages: 3, challengeRSS: true, challengeHTML: true}
	c, stop := srv.start()
	defer stop()

	if _, err := c.Search(context.Background(), Query{Terms: "uncertainty"}, SearchOptions{Limit: 20}); err == nil {
		t.Fatal("both paths were challenged and the search returned success")
	}
}

// Paging stops on the item count. The 28th page of this query has 17 items and
// the 29th has none, and neither the status nor the byte count says so.
func TestSearchPagesToTheEnd(t *testing.T) {
	srv := &searchServer{t: t, pages: 3}
	c, stop := srv.start()
	defer stop()

	res, err := c.Search(context.Background(), Query{Terms: "uncertainty"}, SearchOptions{Limit: 500})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// Two full pages of 20 and one short page of 17. The short page ends it
	// without a request for the page after.
	if len(res.Results) != 57 {
		t.Errorf("%d results, want 57", len(res.Results))
	}
	if srv.feedHits != 3 {
		t.Errorf("%d feed requests, want 3, and a fourth means the short page was not recognized as the last one", srv.feedHits)
	}
	if srv.htmlHits != 1 {
		t.Errorf("%d html requests for an unenriched run, want the 1 the total and the facets cost", srv.htmlHits)
	}
	for i, r := range res.Results {
		if r.Position != i+1 {
			t.Errorf("result %d is at position %d", i, r.Position)
		}
	}

	// The one html page read here holds 17 results the feed's ordering does not,
	// and they stay out of a 57 result set rather than joining it, because they
	// are the leftovers of the first twenty by relevance and nothing was read of
	// the next twenty seven pages. Left out silently that would be a result set
	// nobody could account for, so the run says the number.
	var said bool
	for _, n := range res.Notes {
		if strings.Contains(n, "--enrich") {
			said = true
		}
	}
	if !said {
		t.Errorf("17 html results were dropped without a word: %v", res.Notes)
	}
}

// And with --enrich the html pass covers the same ground the feed does, so the
// results it holds and the feed does not are the answer to the same question
// and they are kept.
func TestEnrichKeepsWhatTheFeedMissed(t *testing.T) {
	srv := &searchServer{t: t, pages: 3}
	c, stop := srv.start()
	defer stop()

	res, err := c.Search(context.Background(), Query{Terms: "uncertainty"}, SearchOptions{Limit: 500, Enrich: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// 57 from the feed, and the 17 of the html page's 20 that the feed did not
	// return. The stub serves the same html page for every page number, so the
	// three pages of enrichment add those 17 once and then match them.
	if len(res.Results) != 74 {
		t.Errorf("%d results, want the feed's 57 plus the 17 only the html had", len(res.Results))
	}
	var htmlOnly int
	for _, r := range res.Results {
		if r.Via == ViaHTML {
			htmlOnly++
		}
	}
	if htmlOnly != 17 {
		t.Errorf("%d html only results, want 17", htmlOnly)
	}
}

// --facets is one request and no results, which is the whole reason it exists.
func TestSearchFacetsOnly(t *testing.T) {
	srv := &searchServer{t: t, pages: 3}
	c, stop := srv.start()
	defer stop()

	res, err := c.Search(context.Background(), Query{Terms: "uncertainty"}, SearchOptions{FacetsOnly: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if srv.feedHits != 0 {
		t.Errorf("%d feed requests for a facet listing, want 0", srv.feedHits)
	}
	if srv.htmlHits != 1 {
		t.Errorf("%d html requests, want 1", srv.htmlHits)
	}
	if len(res.Results) != 0 {
		t.Errorf("%d results from a facet listing", len(res.Results))
	}
	if res.Total != 557 || len(res.Facets) != 8 {
		t.Errorf("total %d and %d facet groups", res.Total, len(res.Facets))
	}
}

// A sort the feed cannot honour is stated rather than silently ignored. A
// caller who asked for relevance and got newest first has no way to see that
// from the results themselves.
func TestSearchSaysWhenTheFeedIgnoresTheSort(t *testing.T) {
	srv := &searchServer{t: t, pages: 3}
	c, stop := srv.start()
	defer stop()

	res, err := c.Search(context.Background(), Query{Terms: "uncertainty", Sort: "relevance"}, SearchOptions{Limit: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var said bool
	for _, n := range res.Notes {
		if strings.Contains(n, "sortBy") {
			said = true
		}
	}
	if !said {
		t.Errorf("--sort relevance ran on a feed that ignores it and nothing was said: %v", res.Notes)
	}

	// And the same query without a sort says nothing, because there is nothing
	// to say.
	res, err = c.Search(context.Background(), Query{Terms: "uncertainty"}, SearchOptions{Limit: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, n := range res.Notes {
		if strings.Contains(n, "sortBy") {
			t.Errorf("a run with no --sort warned about sorting: %q", n)
		}
	}
}

func TestRequestsBilledBeforeTheRun(t *testing.T) {
	q := Query{Terms: "uncertainty"}
	for _, tc := range []struct {
		limit                 int
		enrich, facets        bool
		wantRSS, wantHTMLPage int
	}{
		{20, false, false, 1, 1},
		{500, false, false, 25, 1},
		{500, true, false, 25, 25},
		{0, false, true, 0, 1},
	} {
		rss, html := q.Requests(tc.limit, tc.enrich, tc.facets)
		if rss != tc.wantRSS || html != tc.wantHTMLPage {
			t.Errorf("limit %d enrich %v facets %v: %d rss and %d html, want %d and %d",
				tc.limit, tc.enrich, tc.facets, rss, html, tc.wantRSS, tc.wantHTMLPage)
		}
	}
}
