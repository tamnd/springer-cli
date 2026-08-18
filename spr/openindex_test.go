package spr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Everything here runs against the eight captures in testdata, which were taken
// from the three live hosts inside the same few minutes. Nothing in this file
// makes a request.

// decodeCrossref loads the work capture and builds the record the way the
// client would, without the fetch.
func decodeCrossref(t *testing.T) *CrossrefWork {
	t.Helper()
	resp := capturedIndex(t, "crossref_work.json")
	var env crossrefEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		t.Fatalf("crossref envelope: %v", err)
	}
	if env.Status != "ok" {
		t.Fatalf("crossref status = %q, want ok", env.Status)
	}
	var item crossrefItem
	if err := json.Unmarshal(env.Message, &item); err != nil {
		t.Fatalf("crossref message: %v", err)
	}
	w := crossrefRecord(&item)
	w.Envelope.Tier = "crossref"
	w.Envelope.record(resp)
	crossrefProvenance(&w.Envelope, &item)
	w.Envelope.sortMissed()
	return w
}

func TestCrossrefWork(t *testing.T) {
	w := decodeCrossref(t)

	if got, want := string(w.DOI), "10.1007/s10994-021-05946-3"; got != want {
		t.Errorf("doi = %q, want %q", got, want)
	}
	if got, want := w.Type, "journal-article"; got != want {
		t.Errorf("type = %q, want %q", got, want)
	}
	if !strings.HasPrefix(w.Title, "Aleatoric and epistemic uncertainty") {
		t.Errorf("title = %q", w.Title)
	}
	if got, want := w.ContainerTitle, "Machine Learning"; got != want {
		t.Errorf("container = %q, want %q", got, want)
	}
	if got, want := w.ShortContainer, "Mach Learn"; got != want {
		t.Errorf("short container = %q, want %q", got, want)
	}
	if got, want := w.Publisher, "Springer Science and Business Media LLC"; got != want {
		t.Errorf("publisher = %q, want %q", got, want)
	}
	if w.Volume != "110" || w.Issue != "3" || w.Pages != "457-506" {
		t.Errorf("biblio = %s(%s) pp %s, want 110(3) pp 457-506", w.Volume, w.Issue, w.Pages)
	}
}

// The two ISSNs are typed, and which is which is the point. An untyped list
// would let a caller cite the print issn for an electronic article.
func TestCrossrefTypedISSN(t *testing.T) {
	w := decodeCrossref(t)
	want := []TypedISSN{
		{Value: ISSN("0885-6125"), Type: "print"},
		{Value: ISSN("1573-0565"), Type: "electronic"},
	}
	if len(w.ISSNs) != len(want) {
		t.Fatalf("got %d issns, want %d", len(w.ISSNs), len(want))
	}
	for i := range want {
		if w.ISSNs[i] != want[i] {
			t.Errorf("issn %d = %+v, want %+v", i, w.ISSNs[i], want[i])
		}
	}
}

// Four dates, three precisions, and the reason they are four fields. Only
// published-online states a day, so any collapse of these invents one.
func TestCrossrefDatePrecision(t *testing.T) {
	w := decodeCrossref(t)
	for _, tc := range []struct {
		name      string
		got       *Date
		want      string
		precision Precision
	}{
		{"issued", w.Issued, "2021-03", PrecisionMonth},
		{"published", w.Published, "2021-03", PrecisionMonth},
		{"published-print", w.PublishedPrint, "2021-03", PrecisionMonth},
		{"published-online", w.PublishedOnline, "2021-03-08", PrecisionDay},
		{"deposited", w.Deposited, "2023-01-29", PrecisionDay},
	} {
		if tc.got == nil {
			t.Errorf("%s is nil", tc.name)
			continue
		}
		if tc.got.String() != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got.String(), tc.want)
		}
		if tc.got.Precision != tc.precision {
			t.Errorf("%s precision = %q, want %q", tc.name, tc.got.Precision, tc.precision)
		}
	}
}

// The counts, under names that say who counted. The three numbers here are the
// whole argument for this milestone.
func TestCrossrefCounts(t *testing.T) {
	w := decodeCrossref(t)
	if got, want := w.Counts.Citations, 1553; got != want {
		t.Errorf("crossref_citations = %d, want %d", got, want)
	}
	if got, want := w.Counts.References, 122; got != want {
		t.Errorf("crossref_references = %d, want %d", got, want)
	}
	// 66 of the 122 deposited references carry a DOI. The other 56 are a
	// citation string and resolve to nothing, which is why this is its own
	// number and not a footnote.
	if got, want := w.Counts.ReferencesWithDOI, 66; got != want {
		t.Errorf("crossref_references_with_doi = %d, want %d", got, want)
	}

	// And the field that does not exist. A record marshalled here must never
	// carry a bare "citations", because three bodies count this work and the
	// name would not say which one.
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]any
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	counts, _ := round["counts"].(map[string]any)
	if _, exists := counts["citations"]; exists {
		t.Error("the marshalled counts carry a bare citations field, which names no counter")
	}
	if _, exists := counts["crossref_citations"]; !exists {
		t.Error("the marshalled counts do not carry crossref_citations")
	}
}

// The abstract the spec said would not be there.
func TestCrossrefAbstractIsJATS(t *testing.T) {
	w := decodeCrossref(t)
	if w.Abstract == "" {
		t.Fatal("no abstract, and this record has one")
	}
	if strings.Contains(w.Abstract, "<") || strings.Contains(w.Abstract, "jats:") {
		t.Errorf("markup survived the strip: %q", w.Abstract[:80])
	}
	// The jats:title heading is dropped, so the text starts at the first word
	// the author wrote.
	if !strings.HasPrefix(w.Abstract, "The notion of uncertainty") {
		t.Errorf("abstract starts %q", w.Abstract[:60])
	}
}

// The two facts about this deposit that a reader should not have to discover
// for themselves: nobody deposited an affiliation, and the funder has no id.
func TestCrossrefEnvelopeSaysWhatIsMissing(t *testing.T) {
	w := decodeCrossref(t)

	if len(w.Funders) != 1 || w.Funders[0].Name != "Projekt DEAL" {
		t.Errorf("funders = %+v, want one named Projekt DEAL", w.Funders)
	}
	if w.Funders[0].DOI != "" {
		t.Errorf("funder doi = %q, want empty on this deposit", w.Funders[0].DOI)
	}

	want := map[string]string{
		"authors.affiliations": "all 2 authors deposited an empty affiliation array, and OpenAlex has institutions for the same work",
		"funders.doi":          "the funders were deposited by name with no Funder Registry id, so they join to nothing",
		"references.doi":       "56 of 122 deposited references carry no DOI and are a citation string only",
		"subjects":             "the subject array is present and empty, which is every Springer record measured",
	}
	got := map[string]string{}
	for _, m := range w.Envelope.Missed {
		got[m.Field] = m.Why
	}
	for field, why := range want {
		if got[field] != why {
			t.Errorf("missed[%s]\n got %q\nwant %q", field, got[field], why)
		}
	}
	if via := w.Envelope.Via["counts.crossref_citations"]; via != "crossref:is-referenced-by-count" {
		t.Errorf("via = %q", via)
	}
}

func TestCrossrefLicensesAndLinks(t *testing.T) {
	w := decodeCrossref(t)
	// Two licences under one url, told apart by the version they cover and the
	// delay before it applies.
	if len(w.Licenses) != 2 {
		t.Fatalf("got %d licences, want 2", len(w.Licenses))
	}
	if w.Licenses[0].Version != "tdm" || w.Licenses[0].DelayInDays != 0 {
		t.Errorf("licence 0 = %+v, want tdm at day 0", w.Licenses[0])
	}
	if w.Licenses[1].Version != "vor" || w.Licenses[1].DelayInDays != 7 {
		t.Errorf("licence 1 = %+v, want vor after 7 days", w.Licenses[1])
	}
	if len(w.Links) != 3 {
		t.Errorf("got %d links, want 3", len(w.Links))
	}
}

// The ORCID is there and it is not authenticated, which is a difference worth
// keeping. One says the person attached it and the other says the publisher
// typed it.
func TestCrossrefORCIDIsNotAuthenticated(t *testing.T) {
	w := decodeCrossref(t)
	if len(w.Authors) != 2 {
		t.Fatalf("got %d authors, want 2", len(w.Authors))
	}
	first := w.Authors[0]
	if got, want := first.Display(), "Eyke Hüllermeier"; got != want {
		t.Errorf("author = %q, want %q", got, want)
	}
	if got, want := string(first.ORCID), "0000-0002-9944-4108"; got != want {
		t.Errorf("orcid = %q, want %q", got, want)
	}
	if first.ORCIDAuthenticated {
		t.Error("orcid is reported as authenticated and the deposit says false")
	}
	if got, want := first.Sequence, "first"; got != want {
		t.Errorf("sequence = %q, want %q", got, want)
	}
}

// The query url, which is where the filter language either goes right or fails
// silently and returns the whole corpus.
func TestCrossrefQueryURL(t *testing.T) {
	q := CrossrefQuery{
		Bibliographic: "aleatoric uncertainty",
		ISSN:          ISSN("1573-0565"),
		Type:          "journal-article",
		From:          "2020-01-01",
		Until:         "2024-12-31",
		Funder:        "https://doi.org/10.13039/501100001659",
		Rows:          3,
	}
	got := q.URL()
	for _, want := range []string{
		"query.bibliographic=aleatoric+uncertainty",
		"filter=issn%3A1573-0565%2Ctype%3Ajournal-article%2Cfunder%3A10.13039%2F501100001659%2Cfrom-pub-date%3A2020-01-01%2Cuntil-pub-date%3A2024-12-31",
		"rows=3",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("url is missing %q\n got %s", want, got)
		}
	}
	// The funder filter takes a bare Funder Registry DOI. Leaving the resolver
	// prefix on it matches nothing and reports no error.
	if strings.Contains(got, "doi.org") {
		t.Errorf("the resolver prefix survived into the funder filter: %s", got)
	}
}

// A query with no facet parameter still decodes, which is the whole reason the
// facet block is a raw message.
func TestCrossrefQueryWithoutFacets(t *testing.T) {
	resp := capturedIndex(t, "crossref_query.json")
	var env crossrefEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		t.Fatal(err)
	}
	var page crossrefPage
	if err := json.Unmarshal(env.Message, &page); err != nil {
		t.Fatalf("a query with no facets did not decode: %v", err)
	}
	if got, want := page.TotalResults, 6; got != want {
		t.Errorf("total = %d, want %d", got, want)
	}
	if got, want := len(page.Items), 3; got != want {
		t.Errorf("items = %d, want %d", got, want)
	}
	if f := crossrefFacets(page.Facets); len(f) != 0 {
		t.Errorf("got %d facet groups from a query that asked for none", len(f))
	}
	// The narrow filter is the finding. Six results for a phrase that matches
	// 213,566 works across all of Crossref is what an issn and a date range buy.
	if string(crossrefRecord(&page.Items[0]).DOI) != "10.1007/s10994-021-05946-3" {
		t.Errorf("first item = %q", page.Items[0].DOI)
	}
}

func TestCrossrefFacets(t *testing.T) {
	resp := capturedIndex(t, "crossref_facets.json")
	var env crossrefEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		t.Fatal(err)
	}
	var page crossrefPage
	if err := json.Unmarshal(env.Message, &page); err != nil {
		t.Fatal(err)
	}
	got := crossrefFacets(page.Facets)
	if len(got) != 2 {
		t.Fatalf("got %d facet groups, want 2", len(got))
	}
	// Sorted by name, so publisher-name comes first.
	if got[0].Name != "publisher-name" || got[1].Name != "type-name" {
		t.Errorf("groups = %q and %q", got[0].Name, got[1].Name)
	}
	if got[0].ValueCount != 3 {
		t.Errorf("publisher value-count = %d, want 3", got[0].ValueCount)
	}
	// Within a group the values are ordered by count, because the source is a
	// map and a map has no order to preserve.
	if got[0].Values[0].Label != "Elsevier BV" || got[0].Values[0].Count != 38842 {
		t.Errorf("top publisher = %+v", got[0].Values[0])
	}
	if got[1].Values[0].Label != "Journal Article" || got[1].Values[0].Count != 125945 {
		t.Errorf("top type = %+v", got[1].Values[0])
	}
}

// The 404 body is five words of text/plain, and it has to survive contact with
// a json decoder that never runs on it.
func TestCrossrefNotFound(t *testing.T) {
	resp := capturedIndex(t, "crossref_404.txt")
	resp.Code = 404
	resp.Status = Classify(resp.Code, nil, resp.Body, KindJSON)
	if resp.Status != StatusNotFound {
		t.Fatalf("status = %q, want %q", resp.Status, StatusNotFound)
	}
	err := &NoRecord{Host: CrossrefHost, URL: "https://api.crossref.org/works/10.1007/nope-nope-nope", Code: 404, Says: shortReason(resp.Body)}
	want := "api.crossref.org has no record at https://api.crossref.org/works/10.1007/nope-nope-nope: Resource not found."
	if err.Error() != want {
		t.Errorf("\n got %q\nwant %q", err.Error(), want)
	}
}

// decodeOpenAlex loads the work capture and builds the record.
func decodeOpenAlex(t *testing.T) *OpenAlexWork {
	t.Helper()
	resp := capturedIndex(t, "openalex_work.json")
	var wire openAlexWire
	if err := json.Unmarshal(resp.Body, &wire); err != nil {
		t.Fatalf("openalex: %v", err)
	}
	w := openAlexRecord(&wire)
	w.Envelope.Tier = "openalex"
	w.Envelope.record(resp)
	openAlexProvenance(&w.Envelope, &wire)
	w.Envelope.sortMissed()
	return w
}

func TestOpenAlexWork(t *testing.T) {
	w := decodeOpenAlex(t)

	// The id arrives as a url and is carried as the id, because that is the
	// form both citation filters take.
	if got, want := w.ID, "W3014596384"; got != want {
		t.Errorf("id = %q, want %q", got, want)
	}
	if got, want := string(w.DOI), "10.1007/s10994-021-05946-3"; got != want {
		t.Errorf("doi = %q, want %q", got, want)
	}
	if w.Volume != "110" || w.Issue != "3" || w.Pages != "457-506" {
		t.Errorf("biblio = %s(%s) pp %s", w.Volume, w.Issue, w.Pages)
	}
	if w.Source == nil {
		t.Fatal("no source")
	}
	if got, want := string(w.Source.ISSNL), "0885-6125"; got != want {
		t.Errorf("issn_l = %q, want %q", got, want)
	}
	if got, want := len(w.Source.ISSNs), 2; got != want {
		t.Errorf("issns = %d, want %d", got, want)
	}
	if w.OpenAccess == nil || w.OpenAccess.Status != "hybrid" {
		t.Errorf("open access = %+v, want hybrid", w.OpenAccess)
	}
}

// The ROR id, which is the thing OpenAlex has and Crossref does not for the
// same two people on the same paper.
func TestOpenAlexHasInstitutionsWhereCrossrefHasNone(t *testing.T) {
	oa := decodeOpenAlex(t)
	cr := decodeCrossref(t)

	if len(oa.Authors) != len(cr.Authors) {
		t.Fatalf("the two backends disagree about the author count, %d against %d", len(oa.Authors), len(cr.Authors))
	}
	for i, a := range cr.Authors {
		if len(a.Affiliations) != 0 {
			t.Errorf("crossref author %d has an affiliation and the deposit is empty", i)
		}
	}
	first := oa.Authors[0]
	if len(first.Institutions) != 1 {
		t.Fatalf("got %d institutions, want 1", len(first.Institutions))
	}
	inst := first.Institutions[0]
	if got, want := string(inst.ROR), "058kzsd48"; got != want {
		t.Errorf("ror = %q, want %q", got, want)
	}
	if got, want := inst.ROR.URL(), "https://ror.org/058kzsd48"; got != want {
		t.Errorf("ror url = %q, want %q", got, want)
	}
	if got, want := inst.DisplayName, "Paderborn University"; got != want {
		t.Errorf("institution = %q, want %q", got, want)
	}
	if !first.IsCorresponding {
		t.Error("the first author is marked corresponding in the source and not here")
	}
	// Named, orcid'd, and with a null author id. The person is identified and
	// not addressable, and the envelope says so rather than leaving an empty
	// field to be read as an absent person.
	if first.ID != "" {
		t.Errorf("author id = %q, and the source publishes null", first.ID)
	}
	var said bool
	for _, m := range oa.Envelope.Missed {
		if m.Field == "authors.id" {
			said = true
			if want := "2 of 2 authorships carry a display name and orcid with a null author id, so the person is named and not addressable"; m.Why != want {
				t.Errorf("\n got %q\nwant %q", m.Why, want)
			}
		}
	}
	if !said {
		t.Error("the envelope does not mention the null author ids")
	}
}

// The inverted abstract, put back together. 94 distinct words at 141 positions.
func TestOpenAlexInvertedAbstract(t *testing.T) {
	w := decodeOpenAlex(t)
	if !strings.HasPrefix(w.Abstract, "The notion of uncertainty is of major importance in machine learning") {
		t.Errorf("abstract starts %q", w.Abstract[:70])
	}
	if got, want := len(strings.Fields(w.Abstract)), 140; got != want {
		t.Errorf("abstract is %d words, want %d, which is 141 positions less the Abstract heading", got, want)
	}
	// Both backends carry this abstract and both drop the heading, so the two
	// are comparable rather than differing by one word.
	cr := decodeCrossref(t)
	if !strings.HasPrefix(cr.Abstract, "The notion of uncertainty") {
		t.Errorf("crossref abstract starts %q", cr.Abstract[:40])
	}
}

func TestInvertAbstract(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   map[string][]int
		want string
	}{
		{"empty", nil, ""},
		{"one word twice", map[string][]int{"the": {0, 2}, "cat": {1}}, "the cat the"},
		{"heading dropped", map[string][]int{"Abstract": {0}, "we": {1}}, "we"},
		{"out of order input", map[string][]int{"c": {2}, "a": {0}, "b": {1}}, "a b c"},
	} {
		if got := invertAbstract(tc.in); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The two numbers OpenAlex publishes for the same question, and the reason
// neither is called the citation count.
func TestOpenAlexCountsDisagreeWithItself(t *testing.T) {
	w := decodeOpenAlex(t)

	if got, want := w.Counts.Citations, 1563; got != want {
		t.Errorf("openalex_citations = %d, want %d", got, want)
	}
	if got, want := w.Counts.References, 111; got != want {
		t.Errorf("openalex_references = %d, want %d", got, want)
	}
	if got, want := w.Counts.UpdatedDate, "2026-08-16T07:02:28.622633"; got != want {
		t.Errorf("updated = %q, want %q", got, want)
	}

	// Listing the citations of the same work in the same minute returns a
	// different number, because the record's count is a stored aggregate and
	// the list is the live index.
	list := capturedIndex(t, "openalex_cited_by.json")
	var page openAlexPage
	if err := json.Unmarshal(list.Body, &page); err != nil {
		t.Fatal(err)
	}
	if got, want := page.Meta.Count, 1554; got != want {
		t.Errorf("meta.count = %d, want %d", got, want)
	}
	if page.Meta.Count == w.Counts.Citations {
		t.Error("the stored count and the live listing now agree, so the comment explaining why they do not is out of date")
	}

	// And the record's own history is one short of its own total, 1,562
	// against 1,563, for the same reason.
	sum := 0
	for _, y := range w.Counts.ByYear {
		sum += y.Count
	}
	if got, want := sum, 1562; got != want {
		t.Errorf("counts_by_year sums to %d, want %d", got, want)
	}
	if len(w.Counts.ByYear) != 7 || w.Counts.ByYear[0].Year != 2026 {
		t.Errorf("by_year = %+v, want 7 years newest first", w.Counts.ByYear)
	}
}

// The three numbers this milestone exists to keep apart, asserted together so
// that a future merge of them fails here first.
func TestThreeCitationCountsStayApart(t *testing.T) {
	cr := decodeCrossref(t)
	oa := decodeOpenAlex(t)

	// The /metrics page says 1,906 and attributes it to Dimensions in its own
	// prose, which is read by the metrics reader and not by anything here.
	const dimensions = 1906

	if cr.Counts.Citations == oa.Counts.Citations {
		t.Error("crossref and openalex now agree, which would be news")
	}
	for _, n := range []int{cr.Counts.Citations, oa.Counts.Citations} {
		if n == dimensions {
			t.Errorf("a backend reports the Dimensions count of %d, which is a third corpus", dimensions)
		}
	}

	// Marshalled together, the three names survive and no fourth name appears
	// that would let a consumer read one of them as the count.
	raw, err := json.Marshal(struct {
		Crossref CrossrefCounts `json:"crossref"`
		OpenAlex OpenAlexCounts `json:"openalex"`
	}{cr.Counts, oa.Counts})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"crossref_citations":1553`, `"openalex_citations":1563`, `"fwci":113.99`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the marshalled counts are missing %s\n%s", want, raw)
		}
	}
}

func TestOpenAlexImpact(t *testing.T) {
	w := decodeOpenAlex(t)
	if got, want := w.Counts.FWCI, 113.99; got != want {
		t.Errorf("fwci = %v, want %v", got, want)
	}
	if got, want := w.Counts.Percentile, 0.99970283; got != want {
		t.Errorf("percentile = %v, want %v", got, want)
	}
	if !w.Counts.InTopOnePercent || !w.Counts.InTopTenPercent {
		t.Error("the two flags OpenAlex publishes next to the percentile were dropped")
	}
	// Concepts and topics are both published and they disagree. Both are kept.
	if len(w.Concepts) == 0 || w.Concepts[0].Name != "Machine learning" {
		t.Errorf("concepts = %+v", w.Concepts)
	}
	if w.Concepts[0].Level != 1 {
		t.Errorf("concept level = %d, want 1", w.Concepts[0].Level)
	}
	if len(w.Topics) == 0 || w.Topics[0].Name != "Adversarial Robustness in Machine Learning" {
		t.Errorf("topics = %+v", w.Topics)
	}
}

// The citing direction, which is the one link.springer.com does not publish.
func TestOpenAlexCitedByPage(t *testing.T) {
	resp := capturedIndex(t, "openalex_cited_by.json")
	var page openAlexPage
	if err := json.Unmarshal(resp.Body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(page.Results))
	}
	first := openAlexRecord(&page.Results[0])
	if got, want := first.ID, "W3183048323"; got != want {
		t.Errorf("first citing work = %q, want %q", got, want)
	}
	if got, want := string(first.DOI), "10.1007/s10462-023-10562-9"; got != want {
		t.Errorf("first citing doi = %q, want %q", got, want)
	}
	// A page of 200 against a count of 1,554 is eight requests, which is the
	// arithmetic a caller is entitled to before it starts.
	pages := (page.Meta.Count + OpenAlexPageSize - 1) / OpenAlexPageSize
	if pages != 8 {
		t.Errorf("a full listing is %d pages, want 8", pages)
	}
}

func TestOpenAlexCitedByYear(t *testing.T) {
	resp := capturedIndex(t, "openalex_by_year.json")
	var page openAlexPage
	if err := json.Unmarshal(resp.Body, &page); err != nil {
		t.Fatal(err)
	}
	if got, want := page.Meta.Count, 1554; got != want {
		t.Errorf("count = %d, want %d", got, want)
	}
	// Eight groups, and the source sends them by count rather than by year, so
	// 2026 arrives fourth. Sorting is this reader's job.
	if len(page.GroupBy) != 8 {
		t.Fatalf("got %d groups, want 8", len(page.GroupBy))
	}
	if page.GroupBy[0].Key != "2025" {
		t.Errorf("the source order changed, first group is %q", page.GroupBy[0].Key)
	}
	// One of those eight is a work published in 2008 that cites a 2021 paper.
	// It is one record out of 1,554 and it is real, so it is kept rather than
	// filtered out for being impossible.
	var oldest string
	for _, g := range page.GroupBy {
		if oldest == "" || g.Key < oldest {
			oldest = g.Key
		}
	}
	if oldest != "2008" {
		t.Errorf("oldest citing year = %q, want 2008", oldest)
	}
}

// Neither host sends a mailto back, so the pool is the only evidence a request
// was treated as polite.
func TestPoolIsReadAndNotInferred(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string
		mailto string
		want   string
		polite bool
	}{
		{"crossref polite", "polite-single", "a@example.org", "polite-single", true},
		{"crossref anonymous", "public-single", "", "public-single", false},
		{"crossref list endpoint", "polite-array", "a@example.org", "polite-array", true},
		{"openalex asked politely", "", "a@example.org", "polite requested, host names no pool", false},
		{"openalex anonymous", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New(WithMailto(tc.mailto))
			h := http.Header{}
			if tc.header != "" {
				h.Set("X-Api-Pool", tc.header)
			}
			c.readPool("api.crossref.org", h)

			p, ok := c.Pool("api.crossref.org")
			if tc.want == "" {
				if ok {
					t.Fatalf("recorded a pool for a request that asked for nothing and was told nothing: %+v", p)
				}
				return
			}
			if !ok {
				t.Fatal("no pool recorded")
			}
			if got := p.String(); got != tc.want {
				t.Errorf("pool = %q, want %q", got, tc.want)
			}
			if p.Polite() != tc.polite {
				t.Errorf("polite = %v, want %v", p.Polite(), tc.polite)
			}
		})
	}
}

// Both spellings of the rate limit header, and the two things a reset can mean.
func TestRateLimitHeaders(t *testing.T) {
	t.Run("crossref spells it with a hyphen", func(t *testing.T) {
		c := New()
		h := http.Header{}
		h.Set("X-Rate-Limit-Limit", "10")
		h.Set("X-Rate-Limit-Interval", "1s")
		c.readRateLimit("api.crossref.org", h)

		rl, ok := c.RateLimit("api.crossref.org")
		if !ok {
			t.Fatal("X-Rate-Limit-Limit was not read, and it is the spelling Crossref uses")
		}
		if rl.Limit != 10 || rl.Interval != time.Second {
			t.Errorf("got %+v, want 10 per 1s", rl)
		}
	})

	t.Run("openalex reset is a countdown", func(t *testing.T) {
		c := New()
		h := http.Header{}
		h.Set("X-RateLimit-Limit", "1000")
		h.Set("X-RateLimit-Remaining", "991")
		h.Set("X-RateLimit-Reset", "39286")
		h.Set("X-RateLimit-Limit-USD", "0.1")
		h.Set("X-RateLimit-Remaining-USD", "0.0991")
		c.readRateLimit("api.openalex.org", h)

		rl, _ := c.RateLimit("api.openalex.org")
		// 39,286 read as an epoch is the first of January 1970, which is the
		// bug this test exists to keep fixed.
		if rl.Reset.Year() < 2000 {
			t.Errorf("reset = %s, and a countdown was read as an epoch", rl.Reset)
		}
		if d := time.Until(rl.Reset); d < 10*time.Hour || d > 12*time.Hour {
			t.Errorf("reset is %s away, want about eleven hours", d)
		}
		if !rl.HasUSD || rl.RemainingUSD != 0.0991 {
			t.Errorf("the metered budget was dropped: %+v", rl)
		}
	})

	t.Run("a reset that really is an epoch", func(t *testing.T) {
		c := New()
		h := http.Header{}
		h.Set("X-RateLimit-Limit", "60")
		h.Set("X-RateLimit-Reset", "1800000000")
		c.readRateLimit("example.org", h)

		rl, _ := c.RateLimit("example.org")
		if rl.Reset.Year() != 2027 {
			t.Errorf("reset = %s, want an instant in 2027", rl.Reset)
		}
	})
}

// The key never reaches the cache, the record or the terminal. This is the one
// test in the package that runs a real request, against a local server.
func TestAPIKeyNeverLeaves(t *testing.T) {
	const key = "s3cr3t-key-do-not-print"

	var sawKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey = r.URL.Query().Get("api_key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	defer srv.Close()

	var debug strings.Builder
	dir := t.TempDir()
	c := New(WithCache(dir, time.Hour), WithDebug(&debug))

	resp, err := c.Get(context.Background(), srv.URL+"/meta/v2/json?q=doi%3A10.1007%2Fx&api_key="+key, KindJSON)
	if err != nil {
		t.Fatal(err)
	}

	// The key went out on the wire, which is the only way this API takes one.
	if sawKey != key {
		t.Fatalf("the server saw api_key=%q, want the real key", sawKey)
	}

	// And it is in nothing that came back.
	for name, got := range map[string]string{
		"Response.URL":   resp.URL,
		"Response.Final": resp.Final,
		"the debug log":  debug.String(),
	} {
		if strings.Contains(got, key) {
			t.Errorf("%s carries the key: %s", name, got)
		}
	}
	if !strings.Contains(resp.URL, "api_key=") {
		t.Errorf("Response.URL = %q, and the blanked parameter should stay to say a key was removed", resp.URL)
	}

	// Nor in anything written to disk. A key in a cache file outlives the
	// process that wrote it.
	files := 0
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// The file name is a hash of the url and the body holds the url, so
		// both halves of the entry are checked.
		if strings.Contains(string(body), key) || strings.Contains(path, key) {
			t.Errorf("the cache entry %s carries the key", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("nothing was cached, so this test proved nothing")
	}

	// The cache is keyed on the stripped url, so the same request with a
	// rotated key is a hit rather than a second fetch.
	sawKey = ""
	again, err := c.Get(context.Background(), srv.URL+"/meta/v2/json?q=doi%3A10.1007%2Fx&api_key=a-different-key", KindJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !again.FromCache && sawKey != "" {
		t.Error("a rotated key missed the cache, so the credential is part of the cache key")
	}
}

// Without a key nothing is requested at all, because the answer is known.
func TestSpringerAPIWithoutAKey(t *testing.T) {
	t.Setenv(KeyEnv, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, ok := APIKey(); ok {
		t.Fatal("found a key with none configured")
	}
	if src := KeySource(); src != "" {
		t.Errorf("key source = %q, want empty", src)
	}

	c := New()
	if _, err := c.SpringerSearch(context.Background(), SpringerQuery{DOI: DOI("10.1007/x")}); err != ErrNoKey {
		t.Errorf("err = %v, want %v", err, ErrNoKey)
	}
}

func TestSpringerAPIKeyFromConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(KeyEnv, "")
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := os.MkdirAll(filepath.Join(dir, "spr"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# the springer nature api key\napi_key = \"from-the-config\"\nmailto=someone@example.org\n"
	if err := os.WriteFile(filepath.Join(dir, "spr", "config"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := APIKey()
	if !ok || got != "from-the-config" {
		t.Errorf("key = %q %v, want from-the-config", got, ok)
	}
	if src := KeySource(); src != ConfigPath() {
		t.Errorf("source = %q, want %q", src, ConfigPath())
	}

	// The environment wins, so a key exported for one run is not silently
	// ignored in favour of a stale file.
	t.Setenv(KeyEnv, "from-the-environment")
	if got, _ := APIKey(); got != "from-the-environment" {
		t.Errorf("key = %q, want the environment to win", got)
	}
	if src := KeySource(); src != KeyEnv {
		t.Errorf("source = %q, want %q", src, KeyEnv)
	}
}

// The 401 is the only Springer API response anyone has seen, and it says the
// same thing for a missing key and a wrong one.
func TestSpringerAPI401(t *testing.T) {
	resp := capturedIndex(t, "springer_api_401.json")
	resp.Code = 401
	resp.Status = Classify(resp.Code, nil, resp.Body, KindJSON)
	if resp.Status != StatusNotFound {
		t.Errorf("status = %q", resp.Status)
	}

	var wire springerWire
	if err := json.Unmarshal(resp.Body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Status != "Fail" {
		t.Errorf("status = %q, want Fail", wire.Status)
	}
	if want := "Authentication failed. API key is invalid or missing"; wire.Message != want {
		t.Errorf("message = %q, want %q", wire.Message, want)
	}
	// "invalid or missing" is the host's own wording, and there is no field
	// anywhere in the body that separates the two.
	if !strings.Contains(wire.Message, "invalid or missing") {
		t.Error("the message no longer conflates the two failures, so the tool can now tell them apart")
	}
}

func TestSpringerQueryQuoting(t *testing.T) {
	q := SpringerQuery{
		Endpoint: EndpointOpenAccess,
		DOI:      DOI("10.1007/s10994-021-05946-3"),
		Title:    "aleatoric uncertainty",
		Subject:  "Computer Science",
		Year:     "2021",
	}
	// A phrase with a space is quoted. Without the quotes the API reads it as
	// two terms and returns a much larger set without saying it did.
	want := `doi:10.1007/s10994-021-05946-3 title:"aleatoric uncertainty" subject:"Computer Science" year:2021`
	if got := q.Q(); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
	u := q.URL("KEY")
	if !strings.HasPrefix(u, SpringerAPIBase+"/openaccess/json?") {
		t.Errorf("url = %s", u)
	}
	if !strings.Contains(u, "api_key=KEY") {
		t.Errorf("the key is not on the request: %s", u)
	}
	if got := stripAPIKey(u); strings.Contains(got, "KEY") {
		t.Errorf("stripAPIKey left the key: %s", got)
	}
}

func TestParseEndpoint(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want SpringerEndpoint
		bad  bool
	}{
		{"", EndpointMetaV2, false},
		{"meta/v2", EndpointMetaV2, false},
		{"METADATA", EndpointMetadata, false},
		{"openaccess", EndpointOpenAccess, false},
		{"metadata/v3", "", true},
	} {
		got, err := ParseEndpoint(tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("ParseEndpoint(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseEndpoint(%q) = %q, %v", tc.in, got, err)
		}
	}
}

// json is its own kind, and the challenge test does not run over it.
func TestJSONKind(t *testing.T) {
	if got := KindJSON.String(); got != "json" {
		t.Errorf("String = %q", got)
	}
	if !KindJSON.Matches("application/json; charset=utf-8", nil) {
		t.Error("application/json is not matching KindJSON")
	}
	if KindJSON.Matches("text/html", nil) {
		t.Error("text/html is matching KindJSON")
	}

	// A paper about bot detection is not an interstitial.
	body := []byte(`{"title":["A survey of the Client Challenge and other interstitials"]}`)
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	if got := Classify(200, h, body, KindJSON); got != StatusOK {
		t.Errorf("a json record naming the challenge was classified %q", got)
	}
	// And the same bytes served as html still are one.
	h.Set("Content-Type", "text/html")
	if got := Classify(200, h, body, KindHTML); got != StatusChallenged {
		t.Errorf("html carrying the challenge marker was classified %q", got)
	}
}

func TestJATS(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"empty", "", ""},
		{"heading dropped", "<jats:title>Abstract</jats:title><jats:p>We show.</jats:p>", "We show."},
		{"tags are word boundaries", "<jats:p>One.</jats:p><jats:p>Two.</jats:p>", "One. Two."},
		{"entities", "<jats:p>a &amp; b &lt; c</jats:p>", "a & b < c"},
		{"no markup at all", "Plain text.", "Plain text."},
	} {
		if got := jats(tc.in); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Three hosts, three pace buckets. A slow Crossref must not hold up OpenAlex.
func TestOpenIndexPaceBuckets(t *testing.T) {
	buckets := map[string]string{
		CrossrefBase + "/10.1007/x":       CrossrefHost,
		OpenAlexBase + "?filter=cites:W1": OpenAlexHost,
		SpringerAPIBase + "/meta/v2/json": SpringerAPIHost,
		Base + "/article/10.1007/x":       springerHost,
		Base + "/search.rss?query=x":      searchBucket,
	}
	seen := map[string]bool{}
	for raw, want := range buckets {
		got := PaceBucket(raw)
		if got != want {
			t.Errorf("PaceBucket(%s) = %q, want %q", raw, got, want)
		}
		if seen[got] {
			t.Errorf("%s shares a bucket with something else", got)
		}
		seen[got] = true
	}
	if len(seen) != 5 {
		t.Errorf("got %d buckets across three hosts and two springer paths, want 5", len(seen))
	}
}
