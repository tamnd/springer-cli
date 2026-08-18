package spr

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The three subpage extractors, against the four captures.
//
// Every number asserted here was read off the stored page and counted, not
// taken from the spec. Where the two disagreed the capture won and the
// disagreement is written down beside the assertion.

func metricsOf(t *testing.T, file string) *Metrics {
	t.Helper()
	m, err := ExtractMetrics(capturedResponse(t, file))
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return m
}

func TestMetrics(t *testing.T) {
	m := metricsOf(t, "metrics.html")

	if m.DOI != "10.1007/s10994-021-05946-3" {
		t.Errorf("doi = %q", m.DOI)
	}
	if !strings.HasPrefix(m.Title, "Aleatoric and epistemic uncertainty") {
		t.Errorf("title = %q", m.Title)
	}
	if m.Updated == nil {
		t.Fatal("the page prints a last updated stamp and it was not read")
	}
	want := time.Date(2026, 8, 18, 10, 34, 56, 0, time.UTC)
	if !m.Updated.Equal(want) {
		t.Errorf("updated = %v, want %v", m.Updated, want)
	}

	if m.Accesses == nil {
		t.Fatal("no accesses count")
	}
	// 134k is the publisher's number and 134000 is this tool's reading of it.
	// Both are kept, and the page's own caveat about the count is read rather
	// than assumed.
	if m.Accesses.Raw != "134k" || m.Accesses.Value != 134_000 {
		t.Errorf("accesses = %+v, want raw 134k value 134000", m.Accesses)
	}
	if !m.Accesses.Approximate {
		t.Error("the page calls its accesses an approximate count and the record does not say so")
	}
}

// The whole reason this milestone exists: the count is worthless without the
// body that produced it, and the body is read off the page rather than compiled
// in.
func TestCitationsCarryTheirSource(t *testing.T) {
	m := metricsOf(t, "metrics.html")
	if m.Citations == nil {
		t.Fatal("no citation count")
	}
	if m.Citations.Count != 1906 {
		t.Errorf("citations = %d, want 1906", m.Citations.Count)
	}
	if m.Citations.Source != "Dimensions" {
		t.Errorf("citations source = %q, want Dimensions read from the page prose", m.Citations.Source)
	}

	// Springer says 1,906 here. Crossref says 1,553 and OpenAlex 1,563 for the
	// same doi. Three bodies counting different corpora, which is why nothing
	// in this package has a bare Citations int on it.
	for _, f := range []string{"metrics.html", "metrics_subscription.html"} {
		if got := metricsOf(t, f).Citations.Source; got == "" {
			t.Errorf("%s: a citation count arrived with nobody's name on it", f)
		}
	}
}

func TestProvidedBy(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Citation counts are provided by Dimensions and depend on their data availability.", "Dimensions"},
		{"Counts are provided by Web of Science, updated daily.", "Web of Science"},
		{"Counts are provided by Crossref.", "Crossref"},
		{"We update counts daily.", ""},
	}
	for _, c := range cases {
		if got := providedBy(c.in); got != c.want {
			t.Errorf("providedBy(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The donut legend names five kinds of attention and the record keeps the
// machine name apart from the printed noun, because "20 tweeters" and "1307
// Mendeley" are not the same grammar.
func TestAltmetricBreakdown(t *testing.T) {
	m := metricsOf(t, "metrics.html")
	if m.Altmetric == nil {
		t.Fatal("no altmetric block")
	}
	if m.Altmetric.Score != 52 {
		t.Errorf("score = %d, want 52", m.Altmetric.Score)
	}
	if m.Altmetric.ID != "69076743" {
		t.Errorf("altmetric id = %q, want 69076743", m.Altmetric.ID)
	}

	want := map[string]int{"twitter": 20, "blogs": 3, "news": 2, "reddit": 2, "mendeley": 1307}
	got := map[string]int{}
	for _, k := range m.Altmetric.Breakdown {
		got[k.Kind] = k.Count
		if k.Label == "" {
			t.Errorf("%s came through with no printed label", k.Kind)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("breakdown = %v, want %v", got, want)
	}
	for kind, n := range want {
		if got[kind] != n {
			t.Errorf("%s = %d, want %d", kind, got[kind], n)
		}
	}
}

// Two cohorts, not one. The issue this milestone came from says "the
// percentile, the rank and the cohort size" as though there were one of each,
// and the page states two comparisons in one sentence.
func TestAltmetricHasTwoCohorts(t *testing.T) {
	m := metricsOf(t, "metrics.html")
	c := m.Altmetric.Cohorts
	if len(c) != 2 {
		t.Fatalf("cohorts = %d, want 2: all journals and the journal itself", len(c))
	}
	if c[0].Scope != "all journals" || c[0].Percentile != 95 || c[0].Rank != 22032 || c[0].Size != 474090 {
		t.Errorf("the wide cohort = %+v, want all journals 95th ranked 22032 of 474090", c[0])
	}
	if c[1].Scope != "Machine Learning" || c[1].Percentile != 96 || c[1].Rank != 1 || c[1].Size != 29 {
		t.Errorf("the journal cohort = %+v, want Machine Learning 96th ranked 1 of 29", c[1])
	}

	// 96th of 29 and 95th of 474,090 are not comparable, and the sizes are the
	// only thing on the page that says so.
	if c[0].Size <= c[1].Size {
		t.Error("the two cohort sizes came out the same, so one of them was not read")
	}
}

// 22,032 has a comma in it. firstInt stops at the comma and would read it as
// 22, which is the kind of wrong that looks right in a table.
func TestParseCountReadsThousandsSeparators(t *testing.T) {
	cases := map[string]int{"22,032": 22032, "474,090": 474090, "29": 29, "": 0, "n/a": 0}
	for in, want := range cases {
		if got := parseCount(in); got != want {
			t.Errorf("parseCount(%q) = %d, want %d", in, got, want)
		}
	}
	if firstInt("22,032") == 22032 {
		t.Error("firstInt now reads separators, so parseCount is redundant and one of them should go")
	}
}

// Mentions are the named coverage only. A consumer that reads len(Mentions) as
// the attention total is wrong by three orders of magnitude on this capture,
// which is why the two are separate fields.
func TestMentionsAreNamedCoverageOnly(t *testing.T) {
	m := metricsOf(t, "metrics.html")
	if len(m.Mentions) != 5 {
		t.Fatalf("mentions = %d, want 5, the 2 news outlets and 3 blogs", len(m.Mentions))
	}
	for _, mn := range m.Mentions {
		if mn.Title == "" || mn.URL == "" || mn.Outlet == "" {
			t.Errorf("a mention came through incomplete: %+v", mn)
		}
	}
	if m.Mentions[0].Outlet != "Medium US" {
		t.Errorf("first outlet = %q, want Medium US", m.Mentions[0].Outlet)
	}

	// The breakdown counts 1,307 Mendeley readers on the same page and names
	// none of them.
	total := 0
	for _, k := range m.Altmetric.Breakdown {
		total += k.Count
	}
	if total <= len(m.Mentions) {
		t.Error("the breakdown total is not larger than the named mentions, so one of the two was misread")
	}
}

// A second article, to keep the first from being the only shape the reader
// knows. It has a plain accesses count rather than an abbreviated one, and no
// news or blog coverage at all.
func TestMetricsSecondArticle(t *testing.T) {
	m := metricsOf(t, "metrics_subscription.html")

	if m.Accesses.Raw != "1069" || m.Accesses.Value != 1069 {
		t.Errorf("accesses = %+v, want raw 1069 value 1069", m.Accesses)
	}
	if m.Citations.Count != 7 {
		t.Errorf("citations = %d, want 7", m.Citations.Count)
	}
	if len(m.Mentions) != 0 {
		t.Errorf("mentions = %d, want none: this article has no news or blog coverage", len(m.Mentions))
	}
	if len(m.Altmetric.Breakdown) != 2 {
		t.Errorf("breakdown = %d, want 2, tweeters and Mendeley", len(m.Altmetric.Breakdown))
	}
	if len(m.Altmetric.Cohorts) != 2 {
		t.Errorf("cohorts = %d, want 2", len(m.Altmetric.Cohorts))
	}

	// "Tue, 18 Aug 2026 8:11:20 UTC", a single digit hour where the other
	// capture has two. Go's parser takes both against one layout.
	if m.Updated == nil || m.Updated.Hour() != 8 {
		t.Errorf("updated = %v, want an 8am stamp from a single digit hour", m.Updated)
	}
}

// A timestamp is the one thing on this page that is missed rather than simply
// absent, because a daily count with no date cannot be compared with anything,
// including a later reading of the same page.
func TestUpdatedIsRequired(t *testing.T) {
	for _, f := range []string{"metrics.html", "metrics_subscription.html"} {
		m := metricsOf(t, f)
		if m.Updated == nil {
			t.Errorf("%s: no timestamp", f)
		}
		for _, miss := range m.Envelope.Missed {
			if miss.Field == "updated" {
				t.Errorf("%s: updated is in missed and also set", f)
			}
		}
	}
}

func TestFigure(t *testing.T) {
	f, err := ExtractFigure(capturedResponse(t, "figure.html"))
	if err != nil {
		t.Fatal(err)
	}

	if f.Label != "Fig. 1" || f.Number != 1 {
		t.Errorf("label = %q number = %d", f.Label, f.Number)
	}
	if f.Anchor != "#Fig1" {
		t.Errorf("anchor = %q, want #Fig1", f.Anchor)
	}
	if !strings.HasPrefix(f.Caption, "Predictions by EfficientNet") {
		t.Errorf("caption = %q", f.Caption)
	}
	if !strings.HasPrefix(f.ArticleTitle, "Aleatoric and epistemic uncertainty") {
		t.Errorf("article title = %q", f.ArticleTitle)
	}

	// The caption cites a work, and the link text is only the year while the
	// whole reference sits in the title attribute.
	if len(f.Refs) != 1 {
		t.Fatalf("caption refs = %d, want 1", len(f.Refs))
	}
	if f.Refs[0].Text != "2019" {
		t.Errorf("ref text = %q, want the year the caption prints", f.Refs[0].Text)
	}
	if !strings.Contains(f.Refs[0].Citation, "EfficientNet") {
		t.Errorf("ref citation = %q, want the full reference from the title attribute", f.Refs[0].Citation)
	}
}

// The whole reason this subpage is worth a request. The article page carries
// the same figure at lw685, 685 pixels wide, and this page carries it at full,
// 1,177 wide. The full url is guessable by swapping one path segment, and
// guessing a CDN's scheme is a thing that works until it does not.
func TestFigureCarriesTheFullRendition(t *testing.T) {
	f, err := ExtractFigure(capturedResponse(t, "figure.html"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Image == nil {
		t.Fatal("no image")
	}
	if !strings.Contains(f.Image.URL, "/full/") {
		t.Errorf("image url = %q, want the full rendition", f.Image.URL)
	}
	if strings.Contains(f.Image.URL, "lw685") {
		t.Error("this is the inline rendition, which the article page already had")
	}
	if f.Image.Width != 1177 || f.Image.Height != 420 {
		t.Errorf("image = %dx%d, want 1177x420", f.Image.Width, f.Image.Height)
	}
	if !strings.HasSuffix(f.Image.WebP, "?as=webp") {
		t.Errorf("webp = %q, want the webp rendition beside the jpg", f.Image.WebP)
	}

	// The article page states 685x244 for the same figure, so the two
	// renditions are genuinely different assets and not one url twice.
	w, err := ExtractWork(load(t, captures[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Figures) == 0 || !strings.Contains(w.Figures[0].Image, "lw685") {
		t.Errorf("the article page figure is %q, and the contrast this test rests on is gone", w.Figures[0].Image)
	}
}

// The table body is published here and nowhere else. The article page carries
// the caption and a link and zero table elements in 718 KB of html.
func TestTableIsTheOnlyPlaceTheBodyIsPublished(t *testing.T) {
	tb, err := ExtractTable(capturedResponse(t, "table.html"))
	if err != nil {
		t.Fatal(err)
	}

	if tb.Label != "Table 1" {
		t.Errorf("label = %q, want Table 1 split off the heading", tb.Label)
	}
	if tb.Caption != "Notation used throughout the paper" {
		t.Errorf("caption = %q", tb.Caption)
	}
	if tb.Anchor != "#Tab1" {
		t.Errorf("anchor = %q, want #Tab1 off the back link", tb.Anchor)
	}

	if got := tb.Head; len(got) != 2 || got[0] != "Notation" || got[1] != "Meaning" {
		t.Errorf("header row = %v, want Notation and Meaning", got)
	}
	if len(tb.Rows) != 14 {
		t.Errorf("rows = %d, want 14", len(tb.Rows))
	}
	if tb.Cols() != 2 {
		t.Errorf("cols = %d, want 2", tb.Cols())
	}
	for i, r := range tb.Rows {
		if len(r) != 2 {
			t.Errorf("row %d has %d cells", i, len(r))
		}
	}

	// LaTeX is kept as the publisher wrote it. Rendering it here would be this
	// tool having an opinion about notation.
	if !strings.Contains(tb.Rows[0][0], `\(P\)`) {
		t.Errorf("first cell = %q, want the source latex", tb.Rows[0][0])
	}
	if tb.Rows[0][1] != "Probability measure, density or mass function" {
		t.Errorf("first meaning = %q", tb.Rows[0][1])
	}
}

// Both subpages carry the parent article's whole bibliographic head, which no
// container page does. It is the one place in this tool where a subsidiary page
// identifies its subject better than the page that owns it.
func TestSubpagesIdentifyTheirWorkAtRungOne(t *testing.T) {
	m := metricsOf(t, "metrics.html")
	if got := m.Envelope.Via["doi"]; !strings.HasPrefix(got, "highwire:") {
		t.Errorf("metrics doi came from %q, want highwire", got)
	}
	if got := m.Envelope.Via["title"]; !strings.HasPrefix(got, "highwire:") {
		t.Errorf("metrics title came from %q, want highwire", got)
	}

	f, err := ExtractFigure(capturedResponse(t, "figure.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Envelope.Via["article_title"]; !strings.HasPrefix(got, "highwire:") {
		t.Errorf("figure article title came from %q, want highwire", got)
	}
}

// An out of range number answers 200 with page furniture and an empty main, so
// the classifier sees a healthy page and only the extractor can tell.
func TestOutOfRangeSubpageIsNotAnError200(t *testing.T) {
	resp := capturedResponse(t, "figure.html")
	resp.URL = "https://link.springer.com/article/10.1007/s10994-021-05946-3/figures/99"
	resp.Final = resp.URL
	resp.Body = []byte(`<html><head><title>x</title></head><body><main></main></body></html>`)

	if _, err := ExtractFigure(resp); err == nil {
		t.Error("a figures page with an empty main extracted without complaint")
	} else if err != ErrNoSuchFigure {
		t.Errorf("err = %v, want ErrNoSuchFigure", err)
	}

	resp.URL = "https://link.springer.com/article/10.1007/s10994-021-05946-3/tables/99"
	resp.Final = resp.URL
	if _, err := ExtractTable(resp); err != ErrNoSuchTable {
		t.Errorf("err = %v, want ErrNoSuchTable", err)
	}
}

func TestSubpageExtractorsRefuseTheWrongPage(t *testing.T) {
	article := load(t, captures[0])
	if _, err := ExtractMetrics(article); err != ErrNotMetrics {
		t.Errorf("an article page extracted as metrics: %v", err)
	}
	if _, err := ExtractFigure(article); err == nil {
		t.Error("an article page extracted as a figure subpage")
	}
	if _, err := ExtractTable(article); err == nil {
		t.Error("an article page extracted as a table subpage")
	}

	metrics := capturedResponse(t, "metrics.html")
	if _, err := ExtractFigure(metrics); err == nil {
		t.Error("a metrics page extracted as a figure subpage")
	}
}

func TestSubpagePaths(t *testing.T) {
	const work = "https://link.springer.com/article/10.1007/s10994-021-05946-3"

	if got := MetricsURL(work); got != work+"/metrics" {
		t.Errorf("MetricsURL = %q", got)
	}
	if got := MetricsURL(work + "?error=cookies_not_supported"); got != work+"/metrics" {
		t.Errorf("MetricsURL kept the query: %q", got)
	}
	if got := FigureURL(work, 3); got != work+"/figures/3" {
		t.Errorf("FigureURL = %q", got)
	}
	if got := TableURL(work, 1); got != work+"/tables/1" {
		t.Errorf("TableURL = %q", got)
	}

	if !MetricsPath(work + "/metrics") {
		t.Error("a metrics url was not recognized")
	}
	if MetricsPath(work) {
		t.Error("an article url was taken for a metrics url")
	}
	if !FigurePath(work + "/figures/1") {
		t.Error("a figures url was not recognized")
	}
	if FigurePath(work + "/figures/") {
		t.Error("a figures url with no number was accepted")
	}
	if !TablePath(work + "/tables/12") {
		t.Error("a tables url was not recognized")
	}
	if TablePath(work + "/figures/1") {
		t.Error("a figures url was taken for a tables url")
	}

	parent, kind, n := satelliteParts(work + "/figures/7")
	if parent != "/article/10.1007/s10994-021-05946-3" || kind != "figures" || n != 7 {
		t.Errorf("satelliteParts = %q %q %d", parent, kind, n)
	}
}

// The article page announces its tables and publishes none of them. That is
// what the Table record on a work is for: a caption, an anchor and a pointer,
// and no rows, because there are no rows on that page to read.
func TestTheArticlePageAnnouncesTablesWithoutPublishingThem(t *testing.T) {
	w, err := ExtractWork(load(t, captures[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Tables) != 1 {
		t.Fatalf("tables = %d, want 1", len(w.Tables))
	}
	tb := w.Tables[0]
	if tb.Label != "Table 1" || tb.Caption != "Notation used throughout the paper" {
		t.Errorf("table = %+v", tb)
	}
	if tb.Anchor != "#Tab1" {
		t.Errorf("anchor = %q, want #Tab1 off the caption's own id", tb.Anchor)
	}
	if !strings.HasSuffix(tb.PageURL, "/tables/1") {
		t.Errorf("page url = %q", tb.PageURL)
	}

	// 718 KB of html, one table announced, zero table elements in it. The
	// figures on the same page ship their images inline, which is the contrast
	// that makes the tables subpage a fetch you cannot avoid and the figures
	// subpage a fetch you make only for resolution.
	if n := len(findTag(mustDoc(t, captures[0]), atom.Table)); n != 0 {
		t.Errorf("the article page has %d table elements, and the reason spr tables makes a request is gone", n)
	}
	if len(w.Figures) == 0 || w.Figures[0].Image == "" {
		t.Error("the figures on the same page stopped carrying their images inline")
	}
}

func mustDoc(t *testing.T, c capture) *html.Node {
	t.Helper()
	doc, err := parseDoc(load(t, c).Body)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// A figure has two caption elements on the article page and only one of them is
// a caption. c-article-section__figure-caption prints "Fig. 1", which is the
// label, and the prose is in the description below the image under the same
// bottom-caption region the figure subpage uses.
func TestAFigureLabelIsNotItsCaption(t *testing.T) {
	w, err := ExtractWork(load(t, captures[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Figures) != 17 {
		t.Fatalf("figures = %d, want 17", len(w.Figures))
	}

	f := w.Figures[0]
	if f.Label != "Fig. 1" {
		t.Errorf("label = %q", f.Label)
	}
	if f.Caption == f.Label {
		t.Fatalf("the caption is the label again: %q", f.Caption)
	}
	if !strings.HasPrefix(f.Caption, "Predictions by EfficientNet") {
		t.Errorf("caption = %q", f.Caption)
	}
	if f.Anchor != "#Fig1" {
		t.Errorf("anchor = %q, want #Fig1 off the label element's own id", f.Anchor)
	}

	// The same figure on its own subpage carries the same caption, which is
	// what makes spr figures and spr figures N agree with each other.
	sub, err := ExtractFigure(capturedResponse(t, "figure.html"))
	if err != nil {
		t.Fatal(err)
	}
	if sub.Caption != f.Caption {
		t.Errorf("the article page and the subpage disagree about figure 1:\n  %q\n  %q", f.Caption, sub.Caption)
	}
}

// The same fact that makes a subpage well identified makes it dangerous. It
// carries the article's whole head, so extracting one as a work succeeds and
// hands back an article record with no body, no sections and no references. The
// url is the only thing that can tell the two apart.
func TestASubpageIsNotItsWork(t *testing.T) {
	const work = "https://link.springer.com/article/10.1007/s10994-021-05946-3"

	if WorkType(work) != "article" {
		t.Fatalf("the article url itself stopped being an article")
	}
	for _, u := range []string{work + "/metrics", work + "/figures/1", work + "/tables/1", work + "/metrics/"} {
		if got := WorkType(u); got != "" {
			t.Errorf("WorkType(%q) = %q, want no work type", u, got)
		}
	}

	// And it is not the head that decides, so proving the head is there is part
	// of the point.
	doc, err := parseDoc(capturedResponse(t, "metrics.html").Body)
	if err != nil {
		t.Fatal(err)
	}
	if ParseMeta(doc).First("citation_title") == "" {
		t.Error("the metrics capture lost its inherited head, and this test no longer proves anything")
	}
	if _, err := ExtractWork(capturedResponse(t, "metrics.html")); err != ErrNotAWork {
		t.Errorf("a metrics page extracted as a work: %v", err)
	}
}

func TestSplitLabel(t *testing.T) {
	cases := []struct{ in, label, caption string }{
		{"Table 1 Notation used throughout the paper", "Table 1", "Notation used throughout the paper"},
		{"Fig. 4", "Fig. 4", ""},
		{"Figure 12 Results", "Figure 12", "Results"},
		{"An untitled heading", "", "An untitled heading"},
	}
	for _, c := range cases {
		label, caption := splitLabel(c.in)
		if label != c.label || caption != c.caption {
			t.Errorf("splitLabel(%q) = %q, %q; want %q, %q", c.in, label, caption, c.label, c.caption)
		}
	}
}
