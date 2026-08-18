package spr

import (
	"strings"
	"testing"
)

// The two search paths, against the five captures.
//
// Every number here was counted on the stored page or the stored feed. Where
// the spec and the capture disagreed the capture won, and the disagreement is
// written down beside the assertion rather than quietly resolved.

func searchPage(t *testing.T) *SearchPage {
	t.Helper()
	s, err := ExtractSearch(capturedResponse(t, "search.html"))
	if err != nil {
		t.Fatalf("search.html: %v", err)
	}
	return s
}

func feed(t *testing.T, file string) *Feed {
	t.Helper()
	f, err := ParseFeed(capturedFeed(t, file))
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return f
}

func TestSearchPage(t *testing.T) {
	s := searchPage(t)

	// The one number that exists on this path and nowhere else, printed as
	// "Showing 1-20 of 557 results".
	if s.Total != 557 {
		t.Errorf("total = %d, want 557", s.Total)
	}
	if s.First != 1 || s.Last != 20 {
		t.Errorf("showing %d-%d, want 1-20", s.First, s.Last)
	}
	if len(s.Results) != 20 {
		t.Fatalf("%d results, want 20 cards", len(s.Results))
	}
	if !s.HasNext {
		t.Error("page 1 of 28 printed no next link")
	}

	// The sort orders are read off the page's own select rather than compiled
	// in, and the spec's --sort date is not one of them.
	want := []string{"relevance", "newestFirst", "oldestFirst"}
	if strings.Join(s.Sorts, " ") != strings.Join(want, " ") {
		t.Errorf("sorts = %v, want %v", s.Sorts, want)
	}

	// The chips the site prints back, which are the query restated in the
	// site's own words. The link text also carries a screen reader "Remove
	// filter " that is the button's instruction and not the filter's name.
	if strings.Join(s.Applied, ", ") != "Article, 2020-2024" {
		t.Errorf("applied = %v, want [Article 2020-2024]", s.Applied)
	}
}

func TestSearchCard(t *testing.T) {
	r := searchPage(t).Results[0]

	if r.DOI != "10.1007/s41976-024-00155-7" {
		t.Errorf("doi = %q", r.DOI)
	}
	if !strings.HasPrefix(r.Title, "Bayesian Neural Networks for Satellite Fog Detection") {
		t.Errorf("title = %q", r.Title)
	}
	if r.Via != ViaHTML {
		t.Errorf("via = %q, want html", r.Via)
	}
	if r.ContentType != "Article" {
		t.Errorf("content type = %q", r.ContentType)
	}
	if r.Position != 1 {
		t.Errorf("position = %d, want 1", r.Position)
	}

	// The card's description is the truncated one and it says so with an
	// ellipsis. It is not the abstract and it is not stored in that field,
	// because a hundred and eighty characters that stop mid sentence would read
	// as a paper's summary to everything downstream.
	if r.Abstract != "" {
		t.Errorf("the card set an abstract, and the card does not carry one: %q", r.Abstract)
	}
	if !strings.HasSuffix(r.Snippet, "...") {
		t.Errorf("snippet = %q, and the card's description ends in an ellipsis", r.Snippet)
	}

	want := []string{"Prasad Deshpande", "Shivam Tripathi", "Arnab Bhattacharya"}
	if strings.Join(r.Authors, "|") != strings.Join(want, "|") {
		t.Errorf("authors = %v, want %v", r.Authors, want)
	}

	if r.Container == nil {
		t.Fatal("the card names its journal and the record does not")
	}
	if r.Container.Kind != "journal" || r.Container.ID != "41976" {
		t.Errorf("container = %+v, want journal 41976", r.Container)
	}

	// The card prints 25 November 2024 where the feed sends 2024-11-25 for the
	// same kind of field. One parser reads both.
	if r.Published == nil || r.Published.Value.Format("2006-01-02") != "2024-11-25" {
		t.Errorf("published = %+v, want 2024-11-25", r.Published)
	}
}

// Ten of the twenty cards carry an open access label and eleven carry an
// entitlement. Neither is on every card, which is why both are read as present
// or absent rather than as a value that is always there.
func TestSearchCardsAccess(t *testing.T) {
	var oa, access int
	for _, r := range searchPage(t).Results {
		if r.OpenAccess {
			oa++
		}
		if r.Access != "" {
			access++
		}
	}
	if oa != 10 {
		t.Errorf("%d open access cards, want 10", oa)
	}
	if access != 11 {
		t.Errorf("%d cards with an entitlement, want 11", access)
	}
}

// The eight facet groups, with the counts the page printed.
func TestSearchFacets(t *testing.T) {
	groups := map[string]FacetGroup{}
	for _, g := range searchPage(t).Facets {
		groups[g.Group] = g
	}
	if len(groups) != 8 {
		t.Fatalf("%d facet groups, want 8", len(groups))
	}

	for _, tc := range []struct {
		group, param string
		items        int
	}{
		{"content-type", "content-type", 4},
		{"publishing-model", "openAccess", 1},
		{"language", "language", 2},
		{"taxonomy", "taxonomy", 16},
		{"discipline", "facet-discipline", 16},
		{"sub-discipline", "facet-sub-discipline", 16},
		{"sustainable-development-goal", "sustainableDevelopmentGoal", 11},
		// Five windows, not seven. The region also holds the two text inputs
		// that echo the years this query was sent with, and those are the
		// query's state rather than options a caller could choose.
		{"date", "date", 5},
	} {
		g, ok := groups[tc.group]
		if !ok {
			t.Errorf("no %s facet group", tc.group)
			continue
		}
		if g.Param != tc.param {
			t.Errorf("%s posts under %q, want %q", tc.group, g.Param, tc.param)
		}
		if len(g.Items) != tc.items {
			t.Errorf("%s has %d items, want %d", tc.group, len(g.Items), tc.items)
		}
	}

	// Three of the four content types post a value that is not their label,
	// and this is the page saying so in its own markup.
	byLabel := map[string]Facet{}
	for _, f := range groups["content-type"].Items {
		byLabel[f.Label] = f
	}
	for label, wantValue := range map[string]string{
		"Article":          "Article",
		"Research article": "Research",
		"Review article":   "Review",
		"News article":     "News",
	} {
		if got := byLabel[label].Value; got != wantValue {
			t.Errorf("the facet labelled %q posts %q, want %q", label, got, wantValue)
		}
	}
	if !byLabel["Article"].Selected {
		t.Error("the query filtered to Article and the page's own checkbox is checked, and the record does not say so")
	}

	// The counts, which are the reason --facets exists.
	for _, tc := range []struct {
		group, label string
		count        int
	}{
		{"content-type", "Article", 557},
		{"content-type", "Research article", 482},
		{"content-type", "Review article", 57},
		{"content-type", "News article", 1},
		{"publishing-model", "Open access", 291},
		{"language", "English", 555},
		{"language", "German", 2},
		{"taxonomy", "Machine learning", 168},
		{"discipline", "Computer science", 142},
		{"sub-discipline", "Artificial intelligence", 118},
		{"sustainable-development-goal", "Affordable and clean energy", 32},
	} {
		var found bool
		for _, f := range groups[tc.group].Items {
			if f.Label == tc.label {
				found = true
				if f.Count != tc.count {
					t.Errorf("%s %s = %d, want %d", tc.group, tc.label, f.Count, tc.count)
				}
			}
		}
		if !found {
			t.Errorf("%s has no item labelled %q", tc.group, tc.label)
		}
	}

	// The quoted four keep their quotes, because the value is what gets sent
	// and sending it bare matches nothing.
	for _, g := range []string{"taxonomy", "discipline", "sub-discipline", "sustainable-development-goal"} {
		for _, f := range groups[g].Items {
			if !strings.HasPrefix(f.Value, `"`) || !strings.HasSuffix(f.Value, `"`) {
				t.Errorf("%s value %q lost the quotes the form sends it with", g, f.Value)
			}
		}
	}
	// The date group carries no counts at all, and the years it was sent are
	// stated as applied rather than as choices.
	if got := strings.Join(groups["date"].Applied, " "); got != "dateFrom=2020 dateTo=2024" {
		t.Errorf("date applied = %q, want dateFrom=2020 dateTo=2024", got)
	}
}

// The whole reason ParseFeed does not bind description to a string.
//
// encoding/xml takes only the direct chardata of an element, and every abstract
// in this feed is wrapped in <p> with <i>, <InternalRef> and <CitationRef>
// inside it. Bound as a string, 19 of these 20 come back empty and the
// twentieth comes back as the two floating words "Graphical abstract". It
// parses, it does not error, and it looks exactly like Springer having stopped
// sending abstracts.
func TestFeedAbstractsSurviveTheirMarkup(t *testing.T) {
	f := feed(t, "search.rss")
	if len(f.Items) != 20 {
		t.Fatalf("%d items, want 20", len(f.Items))
	}
	for _, r := range f.Items {
		if len(r.Abstract) < 700 {
			t.Errorf("item %d abstract is %d characters, and every abstract in this capture is over 700", r.Position, len(r.Abstract))
		}
	}
	first := f.Items[0].Abstract
	if !strings.HasPrefix(first, "As the detonation product cloud") {
		t.Errorf("abstract starts %q", first[:min(40, len(first))])
	}
	if !strings.HasSuffix(first, "from high explosives.") {
		t.Errorf("abstract ends %q", first[max(0, len(first)-40):])
	}
	// The full abstract, not the card's 180 character version.
	if len(first) != 1408 {
		t.Errorf("abstract is %d characters, want the whole 1408", len(first))
	}
}

func TestFeedItem(t *testing.T) {
	r := feed(t, "search.rss").Items[0]
	if r.DOI != "10.1007/s00193-024-01208-y" {
		t.Errorf("doi = %q", r.DOI)
	}
	if r.Via != ViaRSS {
		t.Errorf("via = %q, want rss", r.Via)
	}
	if r.URL != "https://link.springer.com/article/10.1007/s00193-024-01208-y" {
		t.Errorf("url = %q", r.URL)
	}
	// RSS 2.0 specifies RFC 822 for pubDate. This feed sends 2024-12-28.
	if r.Published == nil || r.Published.Raw != "2024-12-28" {
		t.Fatalf("published = %+v", r.Published)
	}
	if r.Published.Precision != PrecisionDay {
		t.Errorf("precision = %q, want day", r.Published.Precision)
	}
	// The feed carries no content type, no authors and no container. A record
	// that invented them from the doi would be guessing.
	if r.ContentType != "" || len(r.Authors) > 0 || r.Container != nil {
		t.Errorf("the feed produced card fields it does not carry: %+v", r)
	}
}

// One link in twenty is the feed's own base concatenated onto an already
// absolute url. The guid is a clean doi either way, which is why the doi and
// not the link is this record's identity.
func TestFeedRepairsTheDoubledURL(t *testing.T) {
	var found bool
	for _, r := range feed(t, "search.rss").Items {
		if strings.Count(r.URL, "https://") != 1 {
			t.Errorf("%s: url still carries two schemes: %s", r.DOI, r.URL)
		}
		if r.DOI == "10.1038/s44334-024-00011-y" {
			found = true
			if r.URL != "https://www.nature.com/articles/s44334-024-00011-y" {
				t.Errorf("url = %q, want the nature url the doubled one contained", r.URL)
			}
		}
	}
	if !found {
		t.Error("the capture no longer carries the malformed link this test is about")
	}
}

// Two empty feeds, four bytes apart, both 200 ok. Page 29 of this query carries
// the four characters null in the channel body and page 200 carries nothing at
// all. Neither the status nor the size can be a termination signal, and the
// item count is the only thing that can.
func TestEmptyFeedsAreEmptyInTwoWays(t *testing.T) {
	for _, tc := range []struct {
		file  string
		bytes int
	}{
		{"search_null.rss", 190},
		{"search_empty.rss", 186},
	} {
		resp := capturedFeed(t, tc.file)
		if resp.Status != StatusOK {
			t.Errorf("%s classified as %v, and both empty feeds answer 200 ok", tc.file, resp.Status)
		}
		if len(resp.Body) != tc.bytes {
			t.Errorf("%s is %d bytes, want %d", tc.file, len(resp.Body), tc.bytes)
		}
		f := feed(t, tc.file)
		if len(f.Items) != 0 {
			t.Errorf("%s produced %d items", tc.file, len(f.Items))
		}
		// The channel itself is intact on both, so a reader looking for a
		// broken feed would find a healthy one.
		if f.Title != "Latest Results" {
			t.Errorf("%s: channel title = %q", tc.file, f.Title)
		}
	}
	if strings.Contains(string(capturedFeed(t, "search_null.rss").Body), "null") != true {
		t.Error("the null the empty page 29 carries is gone from the capture")
	}
}

// The last page is short, and 27 full pages plus this one is exactly the total
// the html page printed.
func TestLastFeedPageIsShort(t *testing.T) {
	f := feed(t, "search_last.rss")
	if len(f.Items) != 17 {
		t.Fatalf("%d items on the last page, want 17", len(f.Items))
	}
	if got := 27*FeedPageSize + len(f.Items); got != 557 {
		t.Errorf("27 full pages and this one is %d results, and the html page says 557", got)
	}

	// Two of these seventeen works have no abstract at all, and the feed says
	// so by sending an empty description rather than by omitting the element.
	// The count is named so that the whole field breaking is distinguishable
	// from two works being without one.
	var empty int
	for _, r := range f.Items {
		if r.Abstract == "" {
			empty++
		}
	}
	if empty != 2 {
		t.Errorf("%d items without an abstract, want 2", empty)
	}
	var said bool
	for _, m := range f.Envelope.Missed {
		if m.Field == "abstract" && strings.Contains(m.Why, "2 of 17") {
			said = true
		}
	}
	if !said {
		t.Errorf("the envelope did not name the two missing abstracts: %+v", f.Envelope.Missed)
	}
}

// The feed ships no xml declaration. Anything that sniffs for <?xml to decide
// whether it has xml rejects the primary search path of this site.
func TestFeedHasNoXMLDeclaration(t *testing.T) {
	for _, file := range []string{"search.rss", "search_last.rss", "search_null.rss", "search_empty.rss"} {
		body := capturedFeed(t, file).Body
		if strings.HasPrefix(string(body), "<?xml") {
			t.Errorf("%s now ships an xml declaration, which is worth knowing about", file)
		}
		if !strings.HasPrefix(string(body), `<rss version="2.0">`) {
			t.Errorf("%s does not open with <rss version=\"2.0\">", file)
		}
	}
}

// The two paths do not return the same twenty results for the same query in the
// same minute, because the html honours sortBy=relevance and the feed ignores
// it. This is the measurement that makes joining on position impossible.
func TestThePathsDisagree(t *testing.T) {
	html := searchPage(t)
	rss := feed(t, "search.rss")

	inFeed := map[string]bool{}
	for _, r := range rss.Items {
		inFeed[r.DOI] = true
	}
	var shared int
	for i, r := range html.Results {
		if inFeed[r.DOI] {
			shared++
		}
		if i < len(rss.Items) && r.DOI == rss.Items[i].DOI && i > 0 {
			t.Errorf("position %d holds the same doi on both paths, which the orderings do not support", i+1)
		}
	}
	if shared != 3 {
		t.Errorf("%d of 20 results are on both paths, and 3 were measured", shared)
	}
}

// An html page under a feed url, and a feed under an html one. Both are things
// this site does when something goes wrong upstream, and both have to fail as
// the wrong shape rather than as a page with nothing in it.
func TestWrongShapes(t *testing.T) {
	if _, err := ParseFeed(capturedResponse(t, "search.html")); err != ErrNotAFeed {
		t.Errorf("an html page parsed as a feed with %v", err)
	}
	if _, err := ExtractSearch(capturedFeed(t, "search.rss")); err != ErrNotSearch {
		t.Errorf("a feed extracted as a search page with %v", err)
	}
	if _, err := ExtractSearch(capturedResponse(t, "journal.html")); err != ErrNotSearch {
		t.Errorf("a journal extracted as a search page with %v", err)
	}
}

func TestParseShowing(t *testing.T) {
	for _, tc := range []struct {
		in                string
		first, last, want int
	}{
		{"Showing 1-20 of 557 results", 1, 20, 557},
		// An unfiltered search prints its total with separators, and a reader
		// that stopped at the first comma would report one result in a million.
		{"Showing 1-20 of 1,334,344 results", 1, 20, 1_334_344},
		{"Showing 21-40 of 557 results", 21, 40, 557},
		{"", 0, 0, 0},
		{"no numbers here", 0, 0, 0},
	} {
		first, last, total := parseShowing(tc.in)
		if first != tc.first || last != tc.last || total != tc.want {
			t.Errorf("%q -> %d-%d of %d, want %d-%d of %d", tc.in, first, last, total, tc.first, tc.last, tc.want)
		}
	}
}
