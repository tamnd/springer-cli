package spr

import (
	"strings"
	"testing"
)

// The join is on the normalized DOI and on nothing else, which is the one
// decision in this file worth testing hard. A merge on the raw string would
// duplicate a work whose DOI came back in a different case, and a merge on the
// title or the position would fuse two different works.
func TestMergeJoinsOnNormalizedDOI(t *testing.T) {
	res := widenFixture()
	springer := len(res.Results)

	msg := mergeAlso(res, AlsoCrossref, []SearchResult{
		// The same work the Springer search already had, in the shape Crossref
		// returns it, upper cased in the suffix and carrying the abstract the
		// card did not have.
		{DOI: "10.1007/S10994-021-05946-3", Title: "Aleatoric and epistemic uncertainty in machine learning", Abstract: "The deposited abstract.", Via: "crossref"},
		// One Crossref has and this publisher did not publish.
		{DOI: "10.1145/3357384.3358090", Title: "A work from somewhere else entirely", Via: "crossref"},
		// And one with nothing to key on.
		{Title: "A record with no doi at all", Via: "crossref"},
	}, 213566)

	if got := len(res.Results); got != springer+1 {
		t.Fatalf("the merged set holds %d results, want %d", got, springer+1)
	}
	if got := res.Results[0].Via; got != "rss+html+crossref" {
		t.Errorf("the shared result says via %q, want rss+html+crossref", got)
	}
	if res.Results[0].Abstract != "The abstract in full, as the feed carries it." {
		t.Error("the backend overwrote an abstract the site already answered")
	}
	if got := res.Results[springer].Position; got != springer+1 {
		t.Errorf("the added result is at position %d, want %d", got, springer+1)
	}
	if got := res.Results[springer].Via; got != "crossref" {
		t.Errorf("the added result says via %q, and it came from one place", got)
	}

	// The counts are the caller's news and belong in the sentence, not in the
	// result set. 213,566 matches against 2 held is exactly the gap the flag
	// exists to show.
	for _, want := range []string{"213566", "1 already in the Springer results", "1 new", "1 with no doi to merge on"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the note does not say %q: %q", want, msg)
		}
	}
	if got := res.Paths[len(res.Paths)-1]; got != "crossref" {
		t.Errorf("the backend is not in the paths: %v", res.Paths)
	}
}

// A backend is allowed to fill an abstract the site did not carry, and that
// goes into the envelope, because a field answered by another host is not a
// field this site answered.
func TestMergeRecordsWhereTheAbstractCameFrom(t *testing.T) {
	res := widenFixture()
	res.Results[1].Abstract = ""

	mergeAlso(res, AlsoOpenAlex, []SearchResult{
		{DOI: res.Results[1].DOI, Abstract: "The inverted index put back in order."},
	}, 1)

	if res.Results[1].Abstract != "The inverted index put back in order." {
		t.Error("an empty abstract was not filled from the backend")
	}
	if got := res.Envelope.Via["results[].abstract"]; got != "openalex:abstract" {
		t.Errorf("the envelope says the abstract came from %q, want openalex:abstract", got)
	}
	if got := res.Envelope.Via["results[].openalex"]; got != "openalex:works" {
		t.Errorf("the envelope does not record the backend that answered: %q", got)
	}
}

// Merging twice does not merge twice. A result answered by the same backend on
// two runs says that backend once, and its position does not move.
func TestMergeIsIdempotent(t *testing.T) {
	res := widenFixture()
	items := []SearchResult{{DOI: "10.1007/s10994-021-05946-3", Via: "crossref"}}

	mergeAlso(res, AlsoCrossref, items, 1)
	after := len(res.Results)
	via := res.Results[0].Via

	mergeAlso(res, AlsoCrossref, items, 1)
	if len(res.Results) != after {
		t.Errorf("the second merge added %d results", len(res.Results)-after)
	}
	if res.Results[0].Via != via {
		t.Errorf("via grew to %q on the second merge", res.Results[0].Via)
	}
}

func TestAddVia(t *testing.T) {
	cases := []struct{ have, add, want string }{
		{"", "crossref", "crossref"},
		{"rss", "crossref", "rss+crossref"},
		{"rss+html", "crossref", "rss+html+crossref"},
		{"rss+html+crossref", "crossref", "rss+html+crossref"},
		{"rss+html+crossref", "openalex", "rss+html+crossref+openalex"},
	}
	for _, c := range cases {
		if got := addVia(c.have, c.add); got != c.want {
			t.Errorf("addVia(%q, %q) = %q, want %q", c.have, c.add, got, c.want)
		}
	}
}

// Springer's form takes a year and both indexes document a full date. Widening
// a year to the first of January at both ends would quietly lose eleven months
// off the top of the range, so the two ends widen differently.
func TestFullDate(t *testing.T) {
	cases := []struct {
		in    string
		end   bool
		want  string
		about string
	}{
		{"2020", false, "2020-01-01", "a year as a lower bound"},
		{"2024", true, "2024-12-31", "a year as an upper bound"},
		{"2020-03", false, "2020-03", "a year and month is left alone"},
		{"2020-03-08", true, "2020-03-08", "a full date is left alone"},
		{"", false, "", "nothing stays nothing"},
	}
	for _, c := range cases {
		if got := fullDate(c.in, c.end); got != c.want {
			t.Errorf("%s: fullDate(%q, %v) = %q, want %q", c.about, c.in, c.end, got, c.want)
		}
	}
}

func TestParseAlso(t *testing.T) {
	for _, s := range []string{"crossref", "Crossref", " OPENALEX "} {
		if _, err := ParseAlso(s); err != nil {
			t.Errorf("ParseAlso(%q): %v", s, err)
		}
	}
	if _, err := ParseAlso("scopus"); err == nil {
		t.Error("a backend this tool does not read was accepted")
	} else if !strings.Contains(err.Error(), "crossref, openalex") {
		t.Errorf("the error does not list what is accepted: %v", err)
	}
}

// The two record shapes turn into search results without losing what makes
// each backend worth asking. Crossref brings the deposited abstract and
// OpenAlex brings the open access state and the source id.
func TestBackendRecordsBecomeResults(t *testing.T) {
	cr := crossrefResult(*decodeCrossref(t))
	if cr.DOI != "10.1007/s10994-021-05946-3" || cr.Via != "crossref" {
		t.Errorf("crossref result = %+v", cr)
	}
	if cr.Abstract == "" {
		t.Error("the deposited abstract did not survive the conversion")
	}
	if cr.Container == nil || cr.Container.Name != "Machine Learning" {
		t.Errorf("the container did not survive: %+v", cr.Container)
	}
	if len(cr.Authors) != 2 || cr.Authors[0] != "Eyke Hüllermeier" {
		t.Errorf("the authors did not survive: %v", cr.Authors)
	}
	if cr.Published == nil || cr.Published.String() != "2021-03" {
		t.Errorf("published = %v, want the issued date at month precision", cr.Published)
	}

	oa := openAlexResult(*decodeOpenAlex(t))
	if oa.DOI != "10.1007/s10994-021-05946-3" || oa.Via != "openalex" {
		t.Errorf("openalex result = %+v", oa)
	}
	if !oa.OpenAccess {
		t.Error("the open access state did not survive the conversion")
	}
	if oa.Container == nil || oa.Container.ID != "S62148650" {
		t.Errorf("the source id did not survive: %+v", oa.Container)
	}
	// OpenAlex says the first of March where Crossref deposits published-online
	// as the eighth, and neither of them is wrong about its own field. It is one
	// more reason the merge joins on the DOI and leaves the site's own dates
	// alone rather than reconciling them.
	if oa.Published == nil || oa.Published.String() != "2021-03-01" {
		t.Errorf("published = %v, want the publication date at day precision", oa.Published)
	}
	// Both point at the resolver rather than at whichever host answered, so
	// that a merged result set has one kind of url in it.
	if !strings.HasPrefix(oa.URL, "https://doi.org/") {
		t.Errorf("openalex result url = %q", oa.URL)
	}
}

// widenFixture is a Springer search that has already run, in the state Widen
// receives it.
func widenFixture() *SearchResponse {
	return &SearchResponse{
		Total: 557,
		Paths: []string{"rss", "html"},
		Results: []SearchResult{
			{
				Position: 1,
				DOI:      "10.1007/s10994-021-05946-3",
				Title:    "Aleatoric and epistemic uncertainty in machine learning",
				Abstract: "The abstract in full, as the feed carries it.",
				Via:      "rss+html",
			},
			{
				Position: 2,
				DOI:      "10.1007/s10994-024-06594-z",
				Title:    "A second work the site returned",
				Abstract: "Another abstract from the feed.",
				Via:      "rss",
			},
		},
		Envelope: Envelope{Tier: "search", Status: StatusOK},
	}
}
