package spr

import (
	"strings"
	"testing"
)

// The numbers below were counted on the four container captures before any of
// this code existed, the same way the work tests were built. They are here so
// that a change which quietly drops half a table of contents has to argue with
// a specific number.

// capturedResponse returns one named capture as the client would have produced
// it.
func capturedResponse(t *testing.T, file string) *Response {
	t.Helper()
	for _, c := range captures {
		if c.file == file {
			return load(t, c)
		}
	}
	t.Fatalf("no capture named %s", file)
	return nil
}

func TestJournal(t *testing.T) {
	j, err := ExtractJournal(capturedResponse(t, "journal.html"))
	if err != nil {
		t.Fatal(err)
	}

	if j.SpringerID != "10994" || j.Title != "Machine Learning" {
		t.Errorf("id = %q title = %q", j.SpringerID, j.Title)
	}
	// Two numbers for one journal, and neither of them is the canonical one.
	if j.ElectronicISSN != "1573-0565" || j.PrintISSN != "0885-6125" {
		t.Errorf("issn electronic = %q print = %q", j.ElectronicISSN, j.PrintISSN)
	}
	// The field that justifies parsing the analytics payload at all. No meta
	// tag, no schema.org key and no region states it in machine readable form.
	if j.PublishingModel != "Hybrid Access" {
		t.Errorf("publishing model = %q", j.PublishingModel)
	}
	if j.ContinuousPublication == nil || !*j.ContinuousPublication {
		t.Error("the page declares continuous article publishing and the record does not")
	}
	if got := len(j.Subjects); got != 5 {
		t.Errorf("subjects = %d, want 5", got)
	}
	if got := len(j.IndexedIn); got != 29 {
		t.Errorf("indexed in = %d, want 29", got)
	}
	if j.OpenAccessArticles != 664 {
		t.Errorf("open access articles = %d, want 664", j.OpenAccessArticles)
	}
	if !strings.HasPrefix(j.About, "Machine Learning is an international forum") {
		t.Errorf("about = %.60q, which is not the promo text", j.About)
	}

	// The volumes are a request this command did not make. Saying so is the
	// point of a Conn: a journal with unread volumes and a journal with no
	// volumes are different facts.
	if j.Volumes == nil || j.Volumes.Loaded != 0 || j.Volumes.Complete {
		t.Errorf("volumes = %+v, want an unread pointer", j.Volumes)
	}
	if j.Volumes != nil && !strings.HasSuffix(j.Volumes.URL, "/journal/10994/volumes-and-issues") {
		t.Errorf("the volumes pointer does not point at the volumes page: %q", j.Volumes.URL)
	}
}

// The editorial role is the point of a journal's editor list. Flattening every
// name to editor throws away the one thing the page went to the trouble of
// printing.
func TestJournalEditorsKeepTheirRole(t *testing.T) {
	j, err := ExtractJournal(capturedResponse(t, "journal.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(j.Editors) == 0 {
		t.Fatal("no editors")
	}
	for _, e := range j.Editors {
		if e.Role == "" {
			t.Errorf("%s has no role", e.Name)
		}
	}
	if j.Editors[0].Role != "Editor-in-Chief" {
		t.Errorf("the first role is %q, want Editor-in-Chief", j.Editors[0].Role)
	}
}

// A metric with no year is not emitted. An impact factor of 4.9 with no year is
// not comparable with anything, including itself a year later, so the yearless
// one is named in the envelope with its printed text instead.
func TestJournalMetricsRequireAYear(t *testing.T) {
	j, err := ExtractJournal(capturedResponse(t, "journal.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(j.Metrics); got != 3 {
		t.Fatalf("metrics = %d, want the 3 that carry a year", got)
	}
	for _, m := range j.Metrics {
		if m.Year == 0 {
			t.Errorf("%s was emitted with no year", m.Name)
		}
	}

	// 2.4M is what the page says and 2400000 is this tool's reading of it, and
	// both survive so the two are never confused in a table somebody cites.
	var downloads Metric
	for _, m := range j.Metrics {
		if strings.Contains(m.Name, "Downloads") {
			downloads = m
		}
	}
	if downloads.Raw != "2.4M" || downloads.Value != 2_400_000 {
		t.Errorf("downloads raw = %q value = %v", downloads.Raw, downloads.Value)
	}

	var why string
	for _, m := range j.Envelope.Missed {
		if m.Field == "metrics" {
			why = m.Why
		}
	}
	if !strings.Contains(why, "Submission to first decision") {
		t.Errorf("the yearless metric is not named in the envelope: %q", why)
	}
}

func TestBook(t *testing.T) {
	b, err := ExtractBook(capturedResponse(t, "book.html"))
	if err != nil {
		t.Fatal(err)
	}

	if b.DOI != "10.1007/978-3-031-28170-9" || b.Kind != "book" {
		t.Errorf("doi = %q kind = %q", b.DOI, b.Kind)
	}
	if b.Title != "The Economics of Family Taxation" {
		t.Errorf("title = %q", b.Title)
	}
	if !strings.HasPrefix(b.Subtitle, "Optimal Tax Issues") {
		t.Errorf("subtitle = %q", b.Subtitle)
	}
	if b.ProductType != "Monograph" {
		t.Errorf("product type = %q, and only the analytics payload states it", b.ProductType)
	}
	if b.Publisher != "Springer Cham" || b.Edition != "1" || b.Pages != "XI, 102" {
		t.Errorf("publisher = %q edition = %q pages = %q", b.Publisher, b.Edition, b.Pages)
	}
	if b.CopyrightYear != 2023 {
		t.Errorf("copyright year = %d, want 2023", b.CopyrightYear)
	}
	if got := len(b.Keywords); got != 16 {
		t.Errorf("keywords = %d, want 16", got)
	}
	if b.Accesses != 1169 || b.Citations != 22 {
		t.Errorf("accesses = %d citations = %d", b.Accesses, b.Citations)
	}

	// This book has one author and no editors, and an empty editor list is the
	// right answer rather than a missing one.
	if len(b.Authors) != 1 || b.Authors[0].Name != "Alessandro Balestrino" {
		t.Errorf("authors = %+v", b.Authors)
	}
	if len(b.Editors) != 0 {
		t.Errorf("editors = %+v, and this book has none", b.Editors)
	}
}

// Four numbers under four names, because they are four different objects you
// could hold. A record that is right about a book and wrong about which edition
// you can buy is worse than one that says less.
func TestBookKeepsFourISBNsApart(t *testing.T) {
	b, err := ExtractBook(capturedResponse(t, "book.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, got, want string }{
		{"electronic", b.ISBNElectronic, "978-3-031-28170-9"},
		{"hardcover", b.ISBNHardcover, "978-3-031-28169-3"},
		{"softcover", b.ISBNSoftcover, "978-3-031-28172-3"},
		{"print", b.ISBNPrint, "978-3-031-28172-3"},
	} {
		if c.got != c.want {
			t.Errorf("isbn %s = %q, want %q", c.name, c.got, c.want)
		}
	}
	// The doi resolves to the electronic edition and to no other.
	if !strings.HasSuffix(b.DOI, b.ISBNElectronic) {
		t.Errorf("the doi %q does not end in the electronic isbn %q", b.DOI, b.ISBNElectronic)
	}
}

// Three editions with three dates a year apart. One published field would be
// right about one edition and wrong about two.
func TestBookHasThreeEditionDates(t *testing.T) {
	b, err := ExtractBook(capturedResponse(t, "book.html"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Published == nil || b.PublishedHardcover == nil || b.PublishedSoftcover == nil {
		t.Fatalf("dates = %v %v %v", b.Published, b.PublishedHardcover, b.PublishedSoftcover)
	}
	if got := b.Published.String(); got != "2023-04-26" {
		t.Errorf("published = %q", got)
	}
	if got := b.PublishedSoftcover.String(); got != "2024-04-28" {
		t.Errorf("softcover = %q, and it is a year after the ebook", got)
	}
}

// Front matter and back matter are rows in the same list as the chapters. They
// are kept, with their matter named, so a consumer counting chapters can drop
// the two that are not chapters.
func TestBookChapters(t *testing.T) {
	b, err := ExtractBook(capturedResponse(t, "book.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(b.Chapters); got != 9 {
		t.Fatalf("rows = %d, want 7 chapters plus front and back matter", got)
	}
	if b.ChapterCount != 7 {
		t.Errorf("chapter count = %d, want the 7 the page states", b.ChapterCount)
	}

	front, back := b.Chapters[0], b.Chapters[8]
	if front.Matter != "front" || front.Pages != "i-xi" || front.DOI != "" {
		t.Errorf("front matter = %+v, and it has a page range and no doi", front)
	}
	if back.Matter != "back" || back.Pages != "99-102" {
		t.Errorf("back matter = %+v", back)
	}

	// Each row carries its own page range, read from inside the row rather than
	// zipped from a flat list that happens to be the same length today.
	first := b.Chapters[1]
	if first.Matter != "" || first.Pages != "1-14" {
		t.Errorf("first chapter = %+v", first)
	}
	if first.DOI != "10.1007/978-3-031-28170-9_1" {
		t.Errorf("first chapter doi = %q", first.DOI)
	}
	for _, ch := range b.Chapters[1:8] {
		if ch.DOI == "" || ch.Pages == "" || len(ch.Authors) == 0 {
			t.Errorf("chapter %d is incomplete: %+v", ch.Position, ch)
		}
	}
}

// The kind comes from the order form's own hidden type field, not from the
// printed label, because the label is prose that changes with the locale and
// the field is the value the publisher posts to its own cart. The currency is
// read rather than assumed, since prices here are localized by requesting
// address.
func TestBookOffers(t *testing.T) {
	b, err := ExtractBook(capturedResponse(t, "book.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(b.Offers); got != 3 {
		t.Fatalf("offers = %d, want 3", got)
	}
	for _, o := range b.Offers {
		if o.Kind == "" {
			t.Errorf("%q has no kind, so the hidden type field was not read", o.Label)
		}
		if o.Price == nil || o.Price.Currency == "" || o.Price.Amount == 0 {
			t.Errorf("%q has no usable price: %+v", o.Label, o.Price)
		}
		if o.Price != nil && o.Price.Raw == "" {
			t.Errorf("%q lost the printed price string", o.Label)
		}
	}
	if b.Offers[0].Price.Currency != "EUR" || b.Offers[0].Price.Amount != 85.59 {
		t.Errorf("the ebook price is %+v", b.Offers[0].Price)
	}
}

// The book states its series in two places and neither alone is enough: the
// analytics payload has the id the url is built from, and the printed line has
// the name.
func TestBookSeries(t *testing.T) {
	b, err := ExtractBook(capturedResponse(t, "book.html"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Series == nil {
		t.Fatal("no series")
	}
	if b.Series.ID != "2190" || b.Series.Name != "Population Economics" {
		t.Errorf("series = %+v", b.Series)
	}
	if b.SeriesISSN != "1431-6978" {
		t.Errorf("series issn = %q", b.SeriesISSN)
	}
	// This is a monograph, not a proceedings volume, so it names no conference
	// and inventing one out of the title would be worse than saying nothing.
	if b.Conference != nil {
		t.Errorf("conference = %+v, and this book is not a proceedings volume", b.Conference)
	}
}

func TestSeries(t *testing.T) {
	s, err := ExtractSeries(capturedResponse(t, "series.html"))
	if err != nil {
		t.Fatal(err)
	}
	if s.SeriesID != "558" || s.Title != "Lecture Notes in Computer Science" {
		t.Errorf("id = %q title = %q", s.SeriesID, s.Title)
	}
	// The series id arrives as a JSON number here and as a string on a journal,
	// which is why the payload reader accepts both.
	if s.ElectronicISSN != "1611-3349" || s.PrintISSN != "0302-9743" {
		t.Errorf("issn electronic = %q print = %q", s.ElectronicISSN, s.PrintISSN)
	}
	if got := len(s.Editors); got != 4 {
		t.Fatalf("editors = %d, want 4", got)
	}
	if s.Editors[0].Role != "Series Editor" {
		t.Errorf("the role is %q, want Series Editor", s.Editors[0].Role)
	}
	if got := len(s.IndexedIn); got != 12 {
		t.Errorf("indexed in = %d, want 12", got)
	}
}

// The home page shows five books and not the series, which for Lecture Notes in
// Computer Science is five out of many thousands. The record has to say so.
func TestSeriesLatestTitles(t *testing.T) {
	s, err := ExtractSeries(capturedResponse(t, "series.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s.LatestTitles); got != 5 {
		t.Fatalf("latest titles = %d, want 5", got)
	}
	if s.Titles == nil || s.Titles.Complete || s.Titles.Loaded != 5 {
		t.Errorf("titles pointer = %+v, want 5 held and not complete", s.Titles)
	}
	if s.Titles != nil && !strings.HasSuffix(s.Titles.URL, "/series/558/books") {
		t.Errorf("the titles pointer does not point at the rest of them: %q", s.Titles.URL)
	}

	// Authors and editors are told apart by the card's own printed label, not
	// by the itemprop beside it, which says editor on both.
	first, second := s.LatestTitles[0], s.LatestTitles[1]
	if len(first.Authors) != 1 || len(first.Editors) != 0 {
		t.Errorf("the first card is credited %+v", first)
	}
	if len(second.Editors) != 4 || len(second.Authors) != 0 {
		t.Errorf("the second card is credited %+v", second)
	}
	if first.CopyrightYear != 2027 {
		t.Errorf("copyright year = %d", first.CopyrightYear)
	}

	var open int
	for _, tt := range s.LatestTitles {
		if tt.OpenAccess {
			open++
		}
	}
	if open != 1 {
		t.Errorf("open access titles = %d, want 1", open)
	}
}

// 472 KB of html, no JSON-LD at all, and the whole back catalogue for the price
// of one request. That is why this page has a record of its own.
func TestVolumes(t *testing.T) {
	v, err := ExtractVolumes(capturedResponse(t, "volumes.html"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Journal == nil || v.Journal.ID != "10994" {
		t.Errorf("journal = %+v, and the page states it without a second fetch", v.Journal)
	}
	if got := len(v.Volumes); got != 114 {
		t.Errorf("volumes = %d, want 114", got)
	}
	if got := v.Count(); got != 348 {
		t.Errorf("issues = %d, want 348", got)
	}

	// The volume year is read off the printed span rather than off the issues,
	// because volume 115 runs January to August 2026 and a volume that spans a
	// year boundary would otherwise take the year of whichever issue came first.
	top := v.Volumes[0]
	if top.Number != "115" || top.Year != 2026 {
		t.Errorf("the newest volume is %q of %d", top.Number, top.Year)
	}
	if top.Label != "January - August 2026" {
		t.Errorf("volume label = %q", top.Label)
	}
	if got := len(top.Issue); got != 8 {
		t.Fatalf("issues in volume 115 = %d, want 8", got)
	}

	iss := top.Issue[0]
	if iss.Number != "8" || iss.Date == nil || iss.Date.String() != "2026-08" {
		t.Errorf("the newest issue is %+v, and its date is a month rather than a day", iss)
	}
	if iss.Articles == nil || iss.Articles.Loaded != 0 {
		t.Errorf("the issue's articles pointer = %+v, and this page lists issues and not contents", iss.Articles)
	}

	// The themed collection line is the only place the page says what an issue
	// is about, and 86 of the 348 issues carry one.
	var special int
	for _, vol := range v.Volumes {
		for _, i := range vol.Issue {
			if i.SpecialTitle != "" {
				special++
			}
		}
	}
	if special != 86 {
		t.Errorf("special issues = %d, want 86", special)
	}
}

// Every container extractor refuses a page of the wrong kind, so that a typo in
// a url is a usage error rather than an empty record.
func TestContainerExtractorsRefuseTheWrongPage(t *testing.T) {
	article := capturedResponse(t, "article_oa.html")
	if _, err := ExtractJournal(article); err != ErrNotAJournal {
		t.Errorf("ExtractJournal on an article: %v", err)
	}
	if _, err := ExtractSeries(article); err != ErrNotASeries {
		t.Errorf("ExtractSeries on an article: %v", err)
	}
	if _, err := ExtractVolumes(article); err != ErrNotVolumes {
		t.Errorf("ExtractVolumes on an article: %v", err)
	}
	if _, err := ExtractBook(article); err != ErrNotABook {
		t.Errorf("ExtractBook on an article: %v", err)
	}
	// A journal home page and its volumes page share a prefix and are different
	// records, so neither extractor may answer for the other.
	if _, err := ExtractJournal(capturedResponse(t, "volumes.html")); err != ErrNotAJournal {
		t.Errorf("ExtractJournal on the volumes page: %v", err)
	}
	if _, err := ExtractVolumes(capturedResponse(t, "journal.html")); err != ErrNotVolumes {
		t.Errorf("ExtractVolumes on the journal home page: %v", err)
	}
}

// Every page on this site ships two analytics payloads: an assignment that is
// strict JSON and parses, and a push that is javascript and does not. The split
// is by form, not by page type, and the article page is the one most likely to
// be assumed otherwise.
func TestBothAnalyticsFormsOnEveryPage(t *testing.T) {
	for _, c := range captures {
		doc, err := parseDoc(load(t, c).Body)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		d := parseDataLayer(doc)
		if !d.ok() {
			t.Errorf("%s: the assignment form did not parse", c.file)
		}
		if d.broken != 0 {
			t.Errorf("%s: %d assignment blocks did not parse", c.file, d.broken)
		}
		if len(d.pushes) == 0 {
			t.Errorf("%s: no push form was carried", c.file)
		}
	}
}

// The payload reader accepts a value the page states as a JSON number as well
// as one it states as a string, because the series id is a number and the
// journal id is a string and both are identifiers.
func TestDataLayerReadsNumbersAndStrings(t *testing.T) {
	series, err := ExtractSeries(capturedResponse(t, "series.html"))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := ExtractJournal(capturedResponse(t, "journal.html"))
	if err != nil {
		t.Fatal(err)
	}
	if series.SeriesID != "558" {
		t.Errorf("the series id arrived as a json number and read as %q", series.SeriesID)
	}
	if journal.SpringerID != "10994" {
		t.Errorf("the journal id arrived as a json string and read as %q", journal.SpringerID)
	}
}

func TestParseConferenceTitle(t *testing.T) {
	cases := []struct {
		title   string
		ok      bool
		acronym string
		year    int
	}{
		{
			"Advances in Artificial Intelligence: 34th Canadian Conference on Artificial Intelligence, Canadian AI 2021, Vancouver, BC, Canada, May 25-28, 2021, Proceedings",
			true, "Canadian AI", 2021,
		},
		{
			"Computer Vision: 16th European Conference, ECCV 2020, Glasgow, UK, August 23-28, 2020, Proceedings, Part I",
			true, "ECCV", 2020,
		},
		// An ordinary monograph names no conference, and that is the answer for
		// almost every book on the site rather than a failure.
		{"The Economics of Family Taxation", false, "", 0},
		{"", false, "", 0},
	}
	for _, c := range cases {
		got, ok := ParseConferenceTitle(c.title)
		if ok != c.ok {
			t.Errorf("%.40q: ok = %v, want %v", c.title, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.Acronym != c.acronym || got.Year != c.year {
			t.Errorf("%.40q: acronym = %q year = %d, want %q %d", c.title, got.Acronym, got.Year, c.acronym, c.year)
		}
		if got.Name == "" {
			t.Errorf("%.40q: no event name", c.title)
		}
		// The location is deliberately never read. Telling a city apart from
		// the rest of a Springer title needs a gazetteer this tool does not
		// carry, and a guessed city is worse than an absent one.
		if got.Location != "" {
			t.Errorf("%.40q: a location was guessed: %q", c.title, got.Location)
		}
	}
}

func TestContainerPaths(t *testing.T) {
	if !JournalPath("https://link.springer.com/journal/10994") {
		t.Error("a journal home page was not recognised")
	}
	if JournalPath("https://link.springer.com/journal/10994/volumes-and-issues") {
		t.Error("the volumes page was taken for a journal home page")
	}
	if !VolumesPath("/journal/10994/volumes-and-issues") {
		t.Error("a volumes page was not recognised")
	}
	if !SeriesPath("/series/558") || SeriesPath("/series/558/books") {
		t.Error("the series home page and its book list were not told apart")
	}
	if BookKind("/book/10.1007/978-3-031-28170-9") != "book" {
		t.Error("a book path was not recognised")
	}
	if BookKind("/referencework/10.1007/978-3-642-27737-5") != "referencework" {
		t.Error("a reference work path was not recognised")
	}
	if BookKind("/article/10.1007/s10994-021-05946-3") != "" {
		t.Error("an article path was taken for a book")
	}
}
