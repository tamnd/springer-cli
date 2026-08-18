package spr

import (
	"encoding/json"
	"strings"
	"testing"
)

// extract runs the extractor over one named capture.
func extract(t *testing.T, file string) *Work {
	t.Helper()
	for _, c := range Captures {
		if c.File != file {
			continue
		}
		w, err := ExtractWork(load(t, c))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		return w
	}
	t.Fatalf("no capture named %s", file)
	return nil
}

// The numbers below are the ones counted on the page by hand before any of this
// code existed. They are here so that a change which quietly halves the
// reference list has to argue with a specific number rather than with a vague
// sense that the record looks fine.
func TestOpenAccessArticle(t *testing.T) {
	w := extract(t, "article_oa.html")

	if w.DOI != "10.1007/s10994-021-05946-3" {
		t.Errorf("doi = %q", w.DOI)
	}
	// The tag says Article, not the OriginalPaper that the springer api returns
	// for the same doi. Two vocabularies, two names for one thing, and this is
	// the one the page states.
	if w.Type != "article" || w.ArticleType != "Article" {
		t.Errorf("type = %q, article_type = %q", w.Type, w.ArticleType)
	}
	if !strings.HasPrefix(w.Title, "Aleatoric and epistemic uncertainty") {
		t.Errorf("title = %q", w.Title)
	}
	if w.ContainerTitle != "Machine Learning" || w.Volume != "110" {
		t.Errorf("container = %q volume = %q", w.ContainerTitle, w.Volume)
	}

	// Highwire gave one issn and JSON-LD gave two. Both are kept.
	if len(w.ISSN) != 2 {
		t.Errorf("issn = %v, want the print and the electronic one", w.ISSN)
	}

	if got := len(w.Keywords); got != 21 {
		t.Errorf("keywords = %d, want 21", got)
	}
	if got := len(w.Subjects); got != 5 {
		t.Errorf("subjects = %d, want 5", got)
	}
	if got := len(w.References); got != 122 {
		t.Errorf("references = %d, want 122", got)
	}
	if w.ReferenceLinks != 68 {
		t.Errorf("reference links = %d, want 68", w.ReferenceLinks)
	}
	if got := len(w.Figures); got != 17 {
		t.Errorf("figures = %d, want 17", got)
	}
	if w.Equations != 66 {
		t.Errorf("equations = %d, want 66", w.Equations)
	}
	if got := len(w.Footnotes); got != 24 {
		t.Errorf("footnotes = %d, want 24", got)
	}
	if w.Pages != 50 {
		t.Errorf("pages = %d, want 50 for 457 to 506", w.Pages)
	}

	if w.Access.Free == nil || !*w.Access.Free || w.Access.Raw != "Yes" {
		t.Errorf("access = %+v, want a free page", w.Access)
	}
	if w.License != "http://creativecommons.org/licenses/by/4.0/" {
		t.Errorf("license = %q", w.License)
	}

	// The api pointer ships with an empty key today. It is stripped anyway, so
	// there is no path from the page to the output that could carry one.
	if strings.Contains(w.APIURL, "api_key=") && !strings.Contains(w.APIURL, "api_key=&") &&
		!strings.HasSuffix(w.APIURL, "api_key=") {
		t.Errorf("api url still carries a key: %q", w.APIURL)
	}
}

// Authors come from rung 2 because it is the only source that binds orcid,
// affiliation and email to the right person. This is the test that would fail
// if somebody moved them back to the parallel Highwire arrays.
func TestAuthorsComeFromLinkedData(t *testing.T) {
	w := extract(t, "article_oa.html")

	if len(w.Authors) != 2 {
		t.Fatalf("authors = %d, want 2", len(w.Authors))
	}
	if got := w.Envelope.Via["authors"]; got != "linkdata:author[]" {
		t.Errorf("authors came via %q", got)
	}

	first := w.Authors[0]
	if first.Name != "Eyke Hüllermeier" {
		t.Errorf("first author = %q", first.Name)
	}
	if first.ORCID != "0000-0002-9944-4108" {
		t.Errorf("orcid = %q, want it normalized out of the url", first.ORCID)
	}
	if first.Email != "eyke@upb.de" {
		t.Errorf("email = %q", first.Email)
	}
	if len(first.Affiliations) != 1 || first.Affiliations[0].Name != "Paderborn University" {
		t.Errorf("affiliations = %+v", first.Affiliations)
	}
	if !strings.HasPrefix(first.Affiliations[0].Address, "Heinz Nixdorf Institute") {
		t.Errorf("address = %q", first.Affiliations[0].Address)
	}
	if first.Family != "" || first.Given != "" {
		t.Errorf("a name was split into %q and %q; this tool does not split names", first.Family, first.Given)
	}
}

// The section tree is built from data-title and document order. Figures also
// carry data-title and are not sections, and the recommender strip carries one
// and is an advertisement.
func TestSectionTree(t *testing.T) {
	w := extract(t, "article_oa.html")

	if len(w.Sections) != 14 {
		t.Fatalf("sections = %d, want the 14 real ones", len(w.Sections))
	}
	for _, s := range w.Sections {
		if strings.HasPrefix(s.Title, "Fig.") {
			t.Errorf("a figure got into the section tree: %q", s.Title)
		}
		if s.Title == railTitle {
			t.Errorf("the recommender strip got into the section tree")
		}
	}
	if w.Sections[0].Title != "Abstract" || w.Sections[1].Title != "Introduction" {
		t.Errorf("the tree starts %q, %q", w.Sections[0].Title, w.Sections[1].Title)
	}
	if w.Sections[1].Number != "1" {
		t.Errorf("Introduction is numbered %q, want 1", w.Sections[1].Number)
	}

	// Position is the handle, because there is no id attribute to use and a
	// synthesized one would look stable and would not be.
	for i, s := range w.Sections {
		if s.Position != i {
			t.Errorf("section %d has position %d", i, s.Position)
		}
	}

	if len(w.Rails) != 3 {
		t.Errorf("rails = %d, want the 3 recommendations", len(w.Rails))
	}
	for _, r := range w.Rails {
		if strings.Contains(r.URL, "?") {
			t.Errorf("a rail kept its tracking query: %q", r.URL)
		}
	}
}

// 68 of 122 references parse as Highwire pairs and the other 54 are free text.
// Both keep the verbatim string, because the pairs are lossy and the string is
// what Springer published.
func TestReferencesInBothShapes(t *testing.T) {
	w := extract(t, "article_oa.html")

	structured, free, withLinks := 0, 0, 0
	for _, r := range w.References {
		if r.Text == "" {
			t.Fatalf("reference %d kept no verbatim text", r.Position)
		}
		if r.Structured {
			structured++
			if r.Title == "" && r.Journal == "" {
				t.Errorf("reference %d claims to be structured and parsed nothing", r.Position)
			}
		} else {
			free++
		}
		if len(r.Links) > 0 {
			withLinks++
		}
	}
	if structured != 68 || free != 54 {
		t.Errorf("%d structured and %d free text, want 68 and 54", structured, free)
	}
	if withLinks != 68 {
		t.Errorf("%d references carry resolver links, want 68", withLinks)
	}

	kinds := map[string]int{}
	for _, r := range w.References {
		for _, l := range r.Links {
			kinds[l.Kind]++
		}
	}
	// Counted on the page: 68 google scholar, 47 math, 44 article, 29
	// mathscinet, 5 book. The kind is the page's own data-track-action.
	for kind, want := range map[string]int{
		"google-scholar": 68, "math": 47, "article": 44, "mathscinet": 29, "book": 5,
	} {
		if kinds[kind] != want {
			t.Errorf("%s links = %d, want %d", kind, kinds[kind], want)
		}
	}
}

// A restricted page is extracted, not refused. Everything the page carried is
// in the record and only the body is named as missing, in the page's own words.
func TestRestrictedChapterKeepsItsMetadata(t *testing.T) {
	w := extract(t, "chapter.html")

	if w.Type != "chapter" {
		t.Errorf("type = %q", w.Type)
	}
	if w.Title == "" || len(w.Authors) == 0 || w.ISBN == "" {
		t.Errorf("a restricted page lost metadata: title %q, %d authors, isbn %q", w.Title, len(w.Authors), w.ISBN)
	}
	if w.ContainerTitle != "The Economics of Family Taxation" {
		t.Errorf("container = %q", w.ContainerTitle)
	}
	if w.Access.Free == nil || *w.Access.Free {
		t.Errorf("access = %+v, want a restricted page", w.Access)
	}
	if !strings.Contains(w.Access.Statement, "preview of subscription content") {
		t.Errorf("access statement = %q", w.Access.Statement)
	}

	var missed string
	for _, m := range w.Envelope.Missed {
		if m.Field == "body" {
			missed = m.Why
		}
	}
	if missed == "" {
		t.Fatal("the body is absent and nothing said why")
	}
	if !strings.Contains(missed, "preview of subscription content") {
		t.Errorf("the reason given for the missing body was %q", missed)
	}
}

// Present and empty is a fact, not an absence. The tag is present and empty on
// both article captures, including the open access one, so the day it goes away
// has to look different from every other day.
//
// Only the articles carry it. A chapter, a protocol and a reference work entry
// do not ship the tag at all, which is why they are not in this list and why
// absent and present-and-empty have to stay two different answers.
func TestWorldReadableIsPresentAndEmpty(t *testing.T) {
	for _, file := range []string{"article_oa.html", "article_subscription.html"} {
		w := extract(t, file)
		if w.Access.WorldReadable == nil {
			t.Errorf("%s: citation_fulltext_world_readable was recorded as absent", file)
			continue
		}
		if *w.Access.WorldReadable != "" {
			t.Errorf("%s: it has a value now: %q", file, *w.Access.WorldReadable)
		}
		if got := w.Envelope.Via["access.world_readable"]; !strings.Contains(got, "present, empty") {
			t.Errorf("%s: via says %q", file, got)
		}
	}

	for _, file := range []string{"chapter.html", "protocol.html", "referenceworkentry.html"} {
		w := extract(t, file)
		if w.Access.WorldReadable != nil {
			t.Errorf("%s: the tag is absent on this type and something recorded it as present", file)
		}
	}
}

// The analytics blob on an article is javascript and does not parse as JSON. It
// is carried verbatim and nothing reads it, because a lenient parser that half
// works on a payload the site can change without notice is a permanent source
// of quiet wrongness.
func TestTheAnalyticsBlobIsCarriedAndNotRead(t *testing.T) {
	w := extract(t, "article_oa.html")

	raw, ok := w.Envelope.Extra["datalayer"]
	if !ok {
		t.Fatal("the analytics blob was not carried")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("the blob was stored as something other than a string: %v", err)
	}
	if !strings.Contains(s, "window.dataLayer") {
		t.Errorf("the blob does not look like the analytics payload: %.80q", s)
	}
	if json.Valid([]byte(s)) {
		t.Error("the blob parses as json after all, so it should be read rather than carried")
	}
}

// The envelope is the difference between a record you can audit and a record
// you have to trust.
func TestEnvelope(t *testing.T) {
	w := extract(t, "article_oa.html")
	e := w.Envelope

	if e.Tier != "html" {
		t.Errorf("tier = %q", e.Tier)
	}
	if len(e.URLs) != 1 || e.URLs[0] != w.URL {
		t.Errorf("urls = %v", e.URLs)
	}
	if e.Status != StatusOK {
		t.Errorf("status = %q", e.Status)
	}
	if e.Redirects != 3 {
		t.Errorf("redirects = %d", e.Redirects)
	}
	if e.Bytes != 718906 {
		t.Errorf("bytes = %d, want the capture's own size", e.Bytes)
	}
	if e.Fetched.IsZero() {
		t.Error("the record does not say when it was fetched")
	}

	// Every field in the record names the rung and the exact source that
	// answered it.
	for field, want := range map[string]string{
		"title":      "highwire:citation_title",
		"abstract":   "highwire:dc.description",
		"keywords":   "linkdata:keywords",
		"authors":    "linkdata:author[]",
		"sections":   "region:section[data-title]",
		"references": "highwire:citation_reference",
		"ref_links":  "selector:.c-article-references__links a",
		"access":     "highwire:access + jsonld:isAccessibleForFree",
	} {
		if got := e.Via[field]; got != want {
			t.Errorf("via[%s] = %q, want %q", field, got, want)
		}
	}

	// Unread names what was left on the table. An empty list on a 700 KB page
	// would mean the extractor had stopped noticing rather than that it had
	// read everything.
	if len(e.Unread) == 0 {
		t.Error("the extractor claims to have read every region on the page")
	}
	for _, name := range e.Unread {
		if name == "figure" {
			t.Error("figure is listed as unread and the record has 17 figures")
		}
	}
}

// A record omits what the page did not carry rather than emitting nulls and
// empty strings, so a consumer can tell absent from empty.
func TestAbsentMeansAbsent(t *testing.T) {
	w := extract(t, "chapter.html")
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	// A chapter has no volume, no issue and no journal abbreviation, and it
	// says so by not having the keys.
	for _, key := range []string{"volume", "issue", "container_abbrev", "issn"} {
		if _, ok := got[key]; ok {
			t.Errorf("a chapter emitted %q, which it does not have", key)
		}
	}
	if _, ok := got["envelope"]; !ok {
		t.Error("the record has no envelope")
	}
}

// A page that is not one of the four work types is not an error to be caught,
// it is a plain answer.
func TestContainerPagesAreNotWorks(t *testing.T) {
	for _, c := range Captures {
		if c.Kind != "" {
			continue
		}
		if _, err := ExtractWork(load(t, c)); err != ErrNotAWork {
			t.Errorf("%s: err = %v, want ErrNotAWork", c.File, err)
		}
	}
}

// The four work types share one record and no field is invented for one type
// and faked for the others.
func TestAllFourWorkTypes(t *testing.T) {
	for _, c := range Captures {
		if c.Kind == "" {
			continue
		}
		w := extract(t, c.File)
		if w.Type != c.Kind {
			t.Errorf("%s: type = %q, want %q", c.File, w.Type, c.Kind)
		}
		if w.DOI == "" || w.Title == "" {
			t.Errorf("%s: doi = %q, title = %q", c.File, w.DOI, w.Title)
		}
		if len(w.Authors) == 0 {
			t.Errorf("%s: no authors", c.File)
		}
		if w.Access.Free == nil {
			t.Errorf("%s: no access declaration", c.File)
		}
		if w.URL != c.URL {
			t.Errorf("%s: url = %q, want the requested one", c.File, w.URL)
		}
	}
}

// The two vocabularies that state a fact twice agreed on every capture. A
// disagreement is a design decision that needs a person, so it is reported and
// never resolved.
func TestVocabulariesAgree(t *testing.T) {
	for _, c := range Captures {
		resp := load(t, c)
		doc, err := parseDoc(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if d := ParseMeta(doc).CrossCheck(); len(d) > 0 {
			t.Errorf("%s: the vocabularies disagree: %+v", c.File, d)
		}
	}
}
