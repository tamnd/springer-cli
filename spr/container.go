package spr

// The containers: the things works sit inside.
//
// A journal, a book, a series, a volume, an issue and a conference are six
// different records rather than one Container with a kind field, for the same
// reason Work is one record with a type field and not four. A journal has an
// impact factor and no ISBN. A book has three ISBNs, a price and no volumes. A
// volume has issues and nothing else. One merged record would be mostly empty
// on every page it was built from, and a reader could not tell a field that is
// meaningless here from a field that failed to fill.
//
// The whole file is rung 3 and rung 4 work. A journal home page carries 8 meta
// names, a book page 8, a series page 9 and a volumes page 8, against 66 on an
// article, and none of those heads carry a bibliographic vocabulary at all. So
// the region names and the class names are the contract here, and the ladder
// that decides a work record has nothing to arbitrate on a container page.
// Where a page ships a parseable analytics payload it outranks a region, since
// it is the publisher's own machine readable statement rather than something
// read out of a rendered label.

// Journal is one journal home page.
type Journal struct {
	// SpringerID is the journal number in the url, 10994 for Machine Learning.
	// It is publisher scoped and means nothing outside Springer, which is why a
	// journal keys itself on the ISSN and carries this beside it.
	SpringerID string `json:"springer_id,omitempty"`

	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`

	// Two ISSNs, under two names, because they are two different numbers for
	// the same journal and neither is the canonical one. Machine Learning is
	// 0885-6125 in print and 1573-0565 electronic.
	ElectronicISSN string `json:"electronic_issn,omitempty"`
	PrintISSN      string `json:"print_issn,omitempty"`

	PublisherBrand string `json:"publisher_brand,omitempty"`
	Imprint        string `json:"imprint,omitempty"`

	// PublishingModel is Hybrid Access on the measured journal. Only the
	// analytics payload states it, which is what makes that payload worth
	// parsing at all.
	PublishingModel string `json:"publishing_model,omitempty"`

	// ContinuousPublication is a pointer because a journal that did not declare
	// it is not a journal that declared false.
	ContinuousPublication *bool `json:"continuous_publication,omitempty"`

	Subjects []string `json:"subjects,omitempty"`
	Editors  []Author `json:"editors,omitempty"`
	Metrics  []Metric `json:"metrics,omitempty"`

	// IndexedIn is the abstracting and indexing list the journal prints about
	// itself, 40 services on the measured journal. It is the journal's own
	// claim about where it is covered and it is worth having verbatim, because
	// it is the shortest answer to whether a given database should be expected
	// to know about a work published here.
	IndexedIn []string `json:"indexed_in,omitempty"`

	// OpenAccessArticles is the count the page prints in its own sentence,
	// "This journal has 664 open access articles". It is a count of articles in
	// this journal and not a metric about the journal, so it is not a Metric.
	OpenAccessArticles int `json:"open_access_articles,omitempty"`

	Copyright string `json:"copyright,omitempty"`
	About     string `json:"about,omitempty"`

	// Volumes is a Conn rather than a list, because the journal page does not
	// carry the volumes and reading them costs another request. A journal with
	// 38 volumes of which none were read is a different fact from a journal
	// with no volumes, and a bare empty list cannot tell them apart.
	Volumes *Conn `json:"volumes,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Metric is one of a journal's published statistics.
//
// The year is part of the fact and is required. An impact factor with no year
// is not comparable with anything, so a metric whose year cannot be read is
// named in Envelope.Missed with its printed text rather than emitted yearless.
// On the measured journal that is exactly one metric, "Submission to first
// decision (median), 5 days", which the page prints without a year.
//
// Raw survives because 2.4M is what the page says and 2400000 is this tool's
// reading of it. Value is absent rather than zero when the printed form does
// not resolve to a number.
type Metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value,omitempty"`
	Raw   string  `json:"raw,omitempty"`
	Year  int     `json:"year"`
}

// Book is one book, proceedings volume or reference work.
//
// Springer serves all three with the same page shape at /book/, /referencework/
// and again /book/ for proceedings, so they are one record with a Kind read off
// the url path rather than three records that would be identical.
type Book struct {
	DOI      string `json:"doi,omitempty"`
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url,omitempty"`

	// Kind is book, referencework or proceedings, from the url path.
	Kind string `json:"kind,omitempty"`

	// ProductType is Springer's own classification, Monograph on the measured
	// capture. It comes from the analytics payload and no region states it.
	ProductType string `json:"product_type,omitempty"`

	// Four ISBNs, because the page prints four and they are four different
	// objects you could hold. The electronic one is the book's identity on this
	// site and the other three are editions of it.
	ISBNElectronic string `json:"isbn_electronic,omitempty"`
	ISBNPrint      string `json:"isbn_print,omitempty"`
	ISBNHardcover  string `json:"isbn_hardcover,omitempty"`
	ISBNSoftcover  string `json:"isbn_softcover,omitempty"`

	Authors []Author `json:"authors,omitempty"`
	Editors []Author `json:"editors,omitempty"`

	Series     *Ref        `json:"series,omitempty"`
	SeriesISSN string      `json:"series_issn,omitempty"`
	Conference *Conference `json:"conference,omitempty"`

	Edition       string `json:"edition,omitempty"`
	CopyrightYear int    `json:"copyright_year,omitempty"`
	Copyright     string `json:"copyright,omitempty"`
	Publisher     string `json:"publisher,omitempty"`

	// Pages is the printed extent, "XI, 102", and it is a string on purpose.
	// Front matter is numbered in roman and the two counts are not addable.
	Pages         string `json:"pages,omitempty"`
	Illustrations string `json:"illustrations,omitempty"`

	Subjects []string `json:"subjects,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Packages []string `json:"packages,omitempty"`

	// Published is the electronic edition's date, which is the one that matches
	// the DOI. The print editions have their own and they differ by a year on
	// the measured capture, so they are separate fields rather than one date
	// that would be right for one edition and wrong for two.
	Published          *Date `json:"published,omitempty"`
	PublishedHardcover *Date `json:"published_hardcover,omitempty"`
	PublishedSoftcover *Date `json:"published_softcover,omitempty"`

	Chapters     []Chapter `json:"chapters,omitempty"`
	ChapterCount int       `json:"chapter_count,omitempty"`

	Access Access `json:"access"`

	// Accesses and Citations are the two counters the book page prints in its
	// metrics bar. They are the page's own numbers and this tool does not
	// reconcile them with anybody else's.
	Accesses  int `json:"accesses,omitempty"`
	Citations int `json:"citations,omitempty"`

	Offers []Offer `json:"offers,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Chapter is one row of a book's table of contents.
//
// The spec called for a Ref with a page number, and a Ref has no room for one
// and no room for the front matter, which has a page range and no DOI at all.
// So this is its own small record. It is deliberately not a Work: each chapter
// has its own page with its own complete metadata, and a book with 40 chapters
// that embedded 40 work records would produce a document nobody can read.
type Chapter struct {
	Position int    `json:"position"`
	Title    string `json:"title,omitempty"`
	DOI      string `json:"doi,omitempty"`
	URL      string `json:"url,omitempty"`

	// Pages is "Pages 1-14" reduced to "1-14", or "i-xi" on the front matter.
	Pages string `json:"pages,omitempty"`

	// Matter is front or back on the two unnumbered rows and empty on a real
	// chapter. The book page ships all three in the same list and a consumer
	// counting chapters needs to be able to drop the two that are not.
	Matter string `json:"matter,omitempty"`

	Authors []string `json:"authors,omitempty"`
}

// Offer is one way to buy the book, from the commerce block.
//
// Prices are localized by requesting IP, exactly as 3007 found for Amazon, so
// the currency is read off the page and never assumed. A record fetched from
// Hanoi and one fetched from Berlin can carry different currencies for the same
// book, and neither is wrong.
type Offer struct {
	// Kind is ebook, softcover, hardcover or chapter, read from the order
	// form's own hidden type field rather than from the printed label, because
	// the label is prose and the field is the value the publisher posts.
	Kind  string `json:"kind,omitempty"`
	Label string `json:"label,omitempty"`
	Price *Money `json:"price,omitempty"`
}

// Money keeps the printed string beside the parse. A book with no price
// serializes as absent rather than as zero.
type Money struct {
	Raw      string  `json:"raw,omitempty"`
	Amount   float64 `json:"amount,omitempty"`
	Currency string  `json:"currency,omitempty"`
}

// Series is one book series.
type Series struct {
	SeriesID string `json:"series_id,omitempty"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url,omitempty"`

	PrintISSN      string `json:"print_issn,omitempty"`
	ElectronicISSN string `json:"electronic_issn,omitempty"`

	Editors []Author `json:"editors,omitempty"`
	About   string   `json:"about,omitempty"`

	// IndexedIn is the abstracting and indexing list, the same fact a journal
	// prints about itself and in the same words.
	IndexedIn []string `json:"indexed_in,omitempty"`

	// LatestTitles is what the home page shows, which is the two most recent
	// books and not the series. Titles is where the rest of them are.
	LatestTitles []SeriesTitle `json:"latest_titles,omitempty"`
	Titles       *Conn         `json:"titles,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// SeriesTitle is one book on a series home page.
type SeriesTitle struct {
	Title         string   `json:"title,omitempty"`
	URL           string   `json:"url,omitempty"`
	CopyrightYear int      `json:"copyright_year,omitempty"`
	OpenAccess    bool     `json:"open_access,omitempty"`
	Authors       []string `json:"authors,omitempty"`
	Editors       []string `json:"editors,omitempty"`
}

// Volumes is one journal's volumes and issues page, which is 472 KB of html
// with no JSON-LD and no bibliographic meta tags on it at all.
type Volumes struct {
	Journal *Ref     `json:"journal,omitempty"`
	URL     string   `json:"url,omitempty"`
	Volumes []Volume `json:"volumes,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Volume is one volume of a journal.
type Volume struct {
	Number string `json:"number,omitempty"`

	// Label is the printed span of the volume, "January - August 2026", which
	// is not always a whole year and is not derivable from the issues.
	Label string  `json:"label,omitempty"`
	Year  int     `json:"year,omitempty"`
	Issue []Issue `json:"issues,omitempty"`
}

// Issue is one issue of a volume.
type Issue struct {
	Number string `json:"number,omitempty"`
	Label  string `json:"label,omitempty"`
	Date   *Date  `json:"date,omitempty"`
	URL    string `json:"url,omitempty"`

	// SpecialTitle is the themed collection line where the issue has one, for
	// example "Special Issue of the ACML 2021 Journal Track; Guest Editors:
	// Yu-Feng Li, Mehmet Gonen, Kee-Eung Kim". 78 of the measured issues carry
	// one and it is the only place the page states what the issue is about.
	SpecialTitle string `json:"special_title,omitempty"`

	// Articles is a Conn and is never filled by the volumes page, which lists
	// issues and not their contents. It says where the articles are.
	Articles *Conn `json:"articles,omitempty"`
}

// Conference is a conference, which is the one entity on this site with no page
// of its own.
//
// /conference/aaai returns 404 with a zero byte body, so this tool never builds
// a conference url out of an acronym. A conference is assembled from what a
// work says in citation_conference_title and from the proceedings book page it
// links to, and its Proceedings ref is followed rather than constructed.
type Conference struct {
	Name     string `json:"name,omitempty"`
	Acronym  string `json:"acronym,omitempty"`
	Year     int    `json:"year,omitempty"`
	Location string `json:"location,omitempty"`

	Proceedings *Ref `json:"proceedings,omitempty"`
	Series      *Ref `json:"series,omitempty"`
}

// Conn is a pointer to a collection that was not read.
//
// It says how many exist, how many are held, whether that is all of them, and
// where the rest are. A journal with 38 volumes of which 12 were read and a
// journal with 12 volumes are different facts, and a bare array cannot tell
// them apart.
type Conn struct {
	Loaded     int    `json:"loaded"`
	TotalCount int    `json:"total_count,omitempty"`
	Complete   bool   `json:"complete"`
	URL        string `json:"url,omitempty"`
}
