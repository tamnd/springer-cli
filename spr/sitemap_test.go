package spr

import (
	"strings"
	"testing"
	"time"
)

// The sitemaps, read off the captures.
//
// Every number in this file was counted on the real maps on the day they were
// fetched, and the index in particular is the whole 1.27 MB of it, so these are
// assertions about the site rather than about a fixture somebody trimmed.

func TestIndexReadsEveryChild(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_index.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	if s.Form != FormIndex {
		t.Fatalf("form = %q, want index", s.Form)
	}
	if len(s.Children) != 10408 {
		t.Errorf("%d children, want the 10,408 in this capture", len(s.Children))
	}

	// Four name shapes and not one of them unrecognized. A name this tool
	// cannot read is a shard it would silently never enumerate.
	shapes := s.Shapes()
	for _, tc := range []struct {
		precision Precision
		want      int
	}{
		{PrecisionDay, 8106},
		{PrecisionMonth, 2252},
		{PrecisionYear, 50},
	} {
		if shapes[tc.precision] != tc.want {
			t.Errorf("%d children at %s precision, want %d", shapes[tc.precision], tc.precision, tc.want)
		}
	}
	if got := shapes[PrecisionDay] + shapes[PrecisionMonth] + shapes[PrecisionYear]; got != len(s.Children) {
		t.Errorf("%d of %d children carry a bucket, so %d names are in a shape this parser does not know", got, len(s.Children), len(s.Children)-got)
	}
}

// lastmod restates the bucket where it is present and is absent everywhere the
// site knows only a month or a year. That is the finding that makes it useless
// as a filter and worth recording as a fact about the index.
func TestLastModIsOnDayShardsAndNowhereElse(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_index.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}

	var withLastMod, disagreed int
	for _, c := range s.Children {
		if c.LastMod == "" {
			if c.Bucket.Precision == PrecisionDay {
				t.Fatalf("%s is a day shard with no lastmod", c.Name)
			}
			continue
		}
		withLastMod++
		if c.Bucket.Precision != PrecisionDay {
			t.Fatalf("%s carries a lastmod at %s precision", c.Name, c.Bucket.Precision)
		}
		if c.LastMod != c.Bucket.Raw {
			disagreed++
		}
	}
	if withLastMod != 8106 {
		t.Errorf("%d children carry a lastmod, want 8,106", withLastMod)
	}
	if disagreed != 0 {
		t.Errorf("%d children carry a lastmod that is not their bucket, and this tool treats the two as the same statement", disagreed)
	}

	// And the envelope says so, because a field absent on 2,302 of 10,408
	// children with no explanation reads like a parser that missed it.
	var said bool
	for _, m := range s.Envelope.Missed {
		if m.Field == "lastmod" && strings.Contains(m.Why, "2302") {
			said = true
		}
	}
	if !said {
		t.Errorf("the envelope does not account for the missing lastmods: %v", s.Envelope.Missed)
	}
}

// The whole reason the field is called bucket.
func TestOneNominalDayHoldsSixtySixShards(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_index.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	var shards []int
	for _, c := range s.Children {
		if c.Bucket.Raw == "2020-01-01" {
			shards = append(shards, c.Shard)
		}
	}
	if len(shards) != 66 {
		t.Fatalf("%d shards bucketed at 2020-01-01, want 66", len(shards))
	}
	// The shard numbers run 1 to 66, which is what says these are one bucket
	// split by size rather than 66 unrelated files that share a name.
	seen := map[int]bool{}
	for _, n := range shards {
		if n < 1 || n > 66 || seen[n] {
			t.Fatalf("shard number %d is outside a run of 1 to 66", n)
		}
		seen[n] = true
	}
}

func TestIndexSpansAlmostTwoCenturies(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_index.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	oldest, newest := s.Span()
	if oldest.Raw != "1850" || oldest.Precision != PrecisionYear {
		t.Errorf("oldest bucket is %q at %s precision, want 1850 at year", oldest.Raw, oldest.Precision)
	}
	if newest.Raw != "2026-08-18" || newest.Precision != PrecisionDay {
		t.Errorf("newest bucket is %q at %s precision, want 2026-08-18 at day", newest.Raw, newest.Precision)
	}
}

// A shard's entries have nothing to do with the date in its file name. 5,000
// urls filed under the first of January 2020, carrying 173 different lastmods
// that run to 2026, and not one of them is the day on the tin.
func TestTheBucketIsNotAPublicationDate(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_day.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	if s.Form != FormURLSet {
		t.Fatalf("form = %q, want urlset", s.Form)
	}
	if len(s.Entries) != 5000 {
		t.Fatalf("%d entries, want the 5,000 a full shard holds", len(s.Entries))
	}

	stamps := map[string]bool{}
	for _, e := range s.Entries {
		if e.LastMod == "" {
			t.Fatalf("%s has no lastmod, and every entry on a day shard has one", e.URL)
		}
		stamps[e.LastMod] = true
	}
	if len(stamps) != 173 {
		t.Errorf("%d distinct lastmods, want 173", len(stamps))
	}
	if stamps["2020-01-01"] {
		t.Errorf("an entry in the 2020-01-01 shard is stamped 2020-01-01, which would be the first one ever seen")
	}

	kinds := map[string]int{}
	for _, e := range s.Entries {
		kinds[e.Kind]++
	}
	for kind, want := range map[string]int{
		"protocol": 2327, "chapter": 1268, "entry": 1249, "book": 136, "referencework": 20,
	} {
		if kinds[kind] != want {
			t.Errorf("%d %s urls, want %d", kinds[kind], kind, want)
		}
	}
	if kinds[""] != 0 {
		t.Errorf("%d urls in this shard are under a path with no name", kinds[""])
	}
}

// A year shard carries no lastmod at all, which is a normal shard and not a
// broken one, and the envelope has to say which.
func TestAYearShardCarriesNoLastMod(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_year.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	if len(s.Entries) != 1548 {
		t.Errorf("%d entries, want 1,548", len(s.Entries))
	}
	for _, e := range s.Entries {
		if e.LastMod != "" {
			t.Fatalf("%s carries a lastmod on a year shard", e.URL)
		}
	}
	var said bool
	for _, m := range s.Envelope.Missed {
		if m.Field == "lastmod" && strings.Contains(m.Why, "year shard") {
			said = true
		}
	}
	if !said {
		t.Errorf("the envelope does not say why there are no lastmods: %v", s.Envelope.Missed)
	}
}

// The trap. 200 ok, 108 bytes, a well formed urlset with nothing in it, which
// is what every shard past the end of a bucket looks like.
func TestAShardThatDoesNotExistIsAnEmptyURLSet(t *testing.T) {
	resp := capturedMap(t, "sitemap_empty.xml")
	if resp.Status != StatusOK {
		t.Fatalf("status = %s, and this response is a 200", resp.Status)
	}
	s, err := ParseSitemap(resp)
	if err != nil {
		t.Fatalf("an empty urlset failed to parse: %v", err)
	}
	if s.Form != FormURLSet {
		t.Errorf("form = %q, want urlset", s.Form)
	}
	if len(s.Entries) != 0 {
		t.Fatalf("%d entries in a 108 byte map", len(s.Entries))
	}
	var said bool
	for _, m := range s.Envelope.Missed {
		if m.Field == "urls" && strings.Contains(m.Why, "does not exist") {
			said = true
		}
	}
	if !said {
		t.Errorf("an empty map produced no explanation: %v", s.Envelope.Missed)
	}
}

// One url per line of text/plain at a .txt path among xml siblings, and the
// form is decided by the bytes rather than by the extension.
func TestTheShopsMapIsPlainText(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_shops.txt"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	if s.Form != FormText {
		t.Fatalf("form = %q, want text", s.Form)
	}
	if len(s.Entries) != 65 {
		t.Errorf("%d urls, want 65", len(s.Entries))
	}
	for _, e := range s.Entries {
		if e.Kind != "shop" {
			t.Fatalf("%s is kind %q on the shops map", e.URL, e.Kind)
		}
		if e.LastMod != "" {
			t.Fatalf("%s carries a lastmod, and a text map has nowhere to put one", e.URL)
		}
	}
}

// The brands map is not only brands. Four of its 79 urls are under /partners/,
// so anything that keyed on /brands/ would drop them.
func TestTheBrandsMapCarriesPartnersToo(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_brands.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	kinds := map[string]int{}
	for _, e := range s.Entries {
		kinds[e.Kind]++
	}
	if len(s.Entries) != 79 || kinds["brand"] != 75 || kinds["partner"] != 4 {
		t.Errorf("%d urls, %d brands and %d partners, want 79, 75 and 4", len(s.Entries), kinds["brand"], kinds["partner"])
	}

	// The locale alternates are counted and not read, and the envelope says so
	// rather than letting the record look like the whole file.
	var said bool
	for _, u := range s.Envelope.Unread {
		if strings.Contains(u, "66 xhtml:link") {
			said = true
		}
	}
	if !said {
		t.Errorf("the 66 alternates are not named as unread: %v", s.Envelope.Unread)
	}
}

// An index at a name that reads like a list, whose children are subject phrases
// rather than dated shards.
func TestTheSubjectsMapIsAnIndex(t *testing.T) {
	s, err := ParseSitemap(capturedMap(t, "sitemap_subjects.xml"))
	if err != nil {
		t.Fatalf("ParseSitemap: %v", err)
	}
	if s.Form != FormIndex {
		t.Fatalf("form = %q, want index", s.Form)
	}
	if len(s.Children) != 10 {
		t.Errorf("%d children, want 10", len(s.Children))
	}
	for _, c := range s.Children {
		if c.Dated() {
			t.Errorf("%s parsed as a dated shard", c.Name)
		}
	}
	if s.Children[0].Name != "sitemap-subjects-FIRST.xml" {
		t.Errorf("first child is %q, and the map leads with the one literally named FIRST", s.Children[0].Name)
	}
}

func TestSniffIgnoresTheExtension(t *testing.T) {
	for _, tc := range []struct {
		body string
		want Form
	}{
		// The index, declaration and three spaces of indentation before the root.
		{"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n   <sitemapindex xmlns=\"x\">", FormIndex},
		// The brands map, no declaration at all.
		{"<urlset xmlns=\"x\"><url><loc>a</loc></url></urlset>", FormURLSet},
		// A byte order mark in front of a declaration is legal and is not a root
		// element.
		{"\uFEFF<?xml version=\"1.0\"?><urlset>", FormURLSet},
		{"<!-- generated -->\n<sitemapindex>", FormIndex},
		{"https://link.springer.com/shop/springernature/titles/en-us/\n", FormText},
		{"", FormText},
		// An html error page under a .xml url, which is a text map to the sniff
		// and fails on the parse rather than being mistaken for a urlset.
		{"<!DOCTYPE html><html><body>gone</body></html>", FormText},
	} {
		if got := SniffForm([]byte(tc.body)); got != tc.want {
			t.Errorf("SniffForm(%.40q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestParseChildName(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       string
		precision Precision
		shard     int
		ok        bool
	}{
		{"sitemap_2026-08-18_1.xml", "2026-08-18", PrecisionDay, 1, true},
		{"sitemap_2020-01-01_66.xml", "2020-01-01", PrecisionDay, 66, true},
		{"sitemap_1900-01_1.xml", "1900-01", PrecisionMonth, 1, true},
		{"sitemap_1900-01_12.xml", "1900-01", PrecisionMonth, 12, true},
		{"sitemap_1850_1.xml", "1850", PrecisionYear, 1, true},
		{"sitemap-subjects-FIRST.xml", "", "", 0, false},
		{"sitemap_book_series.xml", "", "", 0, false},
		{"sitemap_2020-01-01.xml", "", "", 0, false},
		{"sitemap_2020-1-1_1.xml", "", "", 0, false},
	} {
		bucket, shard, ok := ParseChildName(tc.name)
		if ok != tc.ok {
			t.Errorf("ParseChildName(%q) ok = %v, want %v", tc.name, ok, tc.ok)
			continue
		}
		if !ok {
			continue
		}
		if bucket.Raw != tc.raw || bucket.Precision != tc.precision || shard != tc.shard {
			t.Errorf("ParseChildName(%q) = %q %s shard %d, want %q %s shard %d",
				tc.name, bucket.Raw, bucket.Precision, shard, tc.raw, tc.precision, tc.shard)
		}
	}
}

// A window is compared against the span a bucket covers, because a year bucket
// covers a year and a month bucket covers a month.
func TestBucketWindows(t *testing.T) {
	day := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		return v
	}
	bucket := func(name string) Bucket {
		t.Helper()
		b, _, ok := ParseChildName(name)
		if !ok {
			t.Fatalf("%q did not parse", name)
		}
		return b
	}

	year := bucket("sitemap_1850_1.xml")
	month := bucket("sitemap_1900-01_1.xml")
	today := bucket("sitemap_2026-08-18_1.xml")

	for _, tc := range []struct {
		what         string
		b            Bucket
		since, until time.Time
		want         bool
	}{
		{"a year bucket is kept by a since inside its year", year, day("1850-06-01"), time.Time{}, true},
		{"and dropped by a since after it", year, day("1851-01-01"), time.Time{}, false},
		{"and kept by an until inside its year", year, time.Time{}, day("1850-06-01"), true},
		{"a month bucket is kept by a since inside its month", month, day("1900-01-15"), time.Time{}, true},
		{"and dropped by a since in the next month", month, day("1900-02-01"), time.Time{}, false},
		{"a day bucket is kept by a since on its own day", today, day("2026-08-18"), time.Time{}, true},
		{"and dropped by a since on the day after", today, day("2026-08-19"), time.Time{}, false},
		{"and kept by an until on its own day", today, time.Time{}, day("2026-08-18"), true},
		{"an open window keeps everything", year, time.Time{}, time.Time{}, true},
		{"an undated child is in no window", Bucket{}, day("2020-01-01"), time.Time{}, false},
		{"and in an open one", Bucket{}, time.Time{}, time.Time{}, true},
	} {
		if got := tc.b.InWindow(tc.since, tc.until); got != tc.want {
			t.Errorf("%s: InWindow(%s) = %v, want %v", tc.what, tc.b.Raw, got, tc.want)
		}
	}
}

func TestEntryKind(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"https://link.springer.com/article/10.1007/s10994-021-05946-3", "article"},
		{"https://link.springer.com/chapter/10.1007/15695_2017_12", "chapter"},
		{"https://link.springer.com/protocol/10.1007/978-1-0716-2067-0_1", "protocol"},
		{"https://link.springer.com/rwe/10.1007/978-3-642-27737-5_100-2", "entry"},
		{"https://link.springer.com/referenceworkentry/10.1007/978-3-642-27737-5_100-2", "entry"},
		// The long spelling has to win over the short one, which is why the
		// table is a slice and not a map.
		{"https://link.springer.com/referencework/10.1007/978-3-642-27737-5", "referencework"},
		{"https://link.springer.com/book/10.1007/978-3-031-28170-9", "book"},
		{"https://link.springer.com/journal/10994", "journal"},
		{"https://link.springer.com/series/558", "series"},
		{"https://link.springer.com/collections/dhaabfhgfg", "collection"},
		{"https://link.springer.com/brands/springer/de/about", "brand"},
		{"https://link.springer.com/partners/embo-press", "partner"},
		{"https://link.springer.com/shop/springernature/titles/en-us/", "shop"},
		{"https://link.springer.com/", ""},
		{"https://link.springer.com/something-new/12", ""},
		// A path that only starts with the letters of a kind is not that kind.
		{"https://link.springer.com/articles-about-us", ""},
	} {
		if got := EntryKind(tc.url); got != tc.want {
			t.Errorf("EntryKind(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestNormalizeKind(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"article", "article", true},
		{"Articles", "article", true},
		{" rwe ", "entry", true},
		{"referenceworkentry", "entry", true},
		{"collections", "collection", true},
		{"paper", "paper", false},
	} {
		got, ok := NormalizeKind(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NormalizeKind(%q) = %q %v, want %q %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
