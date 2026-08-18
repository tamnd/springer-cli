package spr

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The book page.
//
// It is the richest container page on the site and the only one that carries
// three sources worth reading. 8 meta names, of which access, doi and title are
// real. Three JSON-LD blocks, the first of which is a schema.org Book and the
// other two identical Product offers. An analytics payload in the assignment
// form with the print ISBN, the series id and Springer's own product type. And
// a printed bibliographic table with sixteen labelled rows that states things
// none of the other three do.
//
// The one thing to be careful about is the ISBNs. The page prints four numbers
// under three labels and they are four different objects: the electronic
// edition, the hardcover, the softcover and, in the analytics payload, a print
// isbn that equals the softcover. Merging them into one isbn field would make a
// record that is right about a book and wrong about which book you can buy.

// ErrNotABook is returned for a page that is not a book page.
var ErrNotABook = errors.New("this page is not a book, proceedings or reference work page")

// bookPaths maps the url prefixes to the kind the record carries. Springer
// serves all three with the same page shape, so they are one record with a kind
// rather than three records that would be identical.
var bookPaths = map[string]string{
	"/book/":          "book",
	"/referencework/": "referencework",
}

// BookKind returns the kind of book a url addresses, or the empty string.
//
// Proceedings are served at /book/ like everything else and are told apart by
// the page rather than by the path, which is why nothing here guesses at one.
func BookKind(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	for prefix, kind := range bookPaths {
		if strings.HasPrefix(u.Path, prefix) {
			return kind
		}
	}
	return ""
}

// ExtractBook reads a book record out of a fetched book page.
func ExtractBook(resp *Response) (*Book, error) {
	if resp == nil {
		return nil, errors.New("no response to extract from")
	}
	kind := BookKind(resp.URL)
	if kind == "" {
		kind = BookKind(resp.Final)
	}
	if kind == "" {
		return nil, ErrNotABook
	}
	p, err := newPage(resp)
	if err != nil {
		return nil, err
	}

	b := &Book{URL: resp.URL, Kind: kind}
	e := p.ld.book()
	bib := bibliographic(p.doc)

	b.DOI = p.set("doi", LevelHighwire, "doi", p.meta.First("doi"))
	if b.DOI == "" {
		b.DOI = p.set("doi", LevelSelector, "the bibliographic table, DOI", strings.TrimPrefix(bib["DOI"], "https://doi.org/"))
	}
	if b.DOI == "" {
		p.env.miss("doi", "neither the head nor the bibliographic table stated a doi")
	}

	b.Title = p.region("title", "book-title")
	if b.Title == "" {
		b.Title = p.set("title", LevelHighwire, "title", p.meta.First("title"))
	}
	b.Subtitle = p.region("subtitle", "book-subtitle")
	b.ProductType = p.layer("product_type", "content", "book", "bookProductType")

	// Four isbns under four names. The electronic one is the book's identity on
	// this site, and the analytics payload's pisbn is the softcover on the
	// measured capture, which is why it fills print rather than overwriting
	// anything the table said.
	b.ISBNElectronic = isbnOf(p, "isbn_electronic", bib["eBook ISBN"], "the bibliographic table, eBook ISBN")
	b.ISBNHardcover = isbnOf(p, "isbn_hardcover", bib["Hardcover ISBN"], "the bibliographic table, Hardcover ISBN")
	b.ISBNSoftcover = isbnOf(p, "isbn_softcover", bib["Softcover ISBN"], "the bibliographic table, Softcover ISBN")
	b.ISBNPrint = isbnOf(p, "isbn_print", p.dl.str("content", "book", "pisbn"), "datalayer.content.book.pisbn")
	if b.ISBNElectronic == "" && e != nil {
		b.ISBNElectronic = p.set("isbn_electronic", LevelLinkedData, "isbn", e.ISBN.First())
	}

	b.Publisher = p.set("publisher", LevelSelector, "the bibliographic table, Publisher", bib["Publisher"])
	b.Edition = p.set("edition", LevelSelector, "the bibliographic table, Edition Number", bib["Edition Number"])
	b.Pages = p.set("pages", LevelSelector, "the bibliographic table, Number of Pages", bib["Number of Pages"])
	b.Illustrations = p.set("illustrations", LevelSelector, "the bibliographic table, Number of Illustrations", bib["Number of Illustrations"])
	b.Copyright = p.set("copyright", LevelSelector, "the bibliographic table, Copyright Information", bib["Copyright Information"])
	b.SeriesISSN = p.set("series_issn", LevelSelector, "the bibliographic table, Series ISSN", bib["Series ISSN"])

	if e != nil && e.CopyrightYear != "" {
		if y, err := strconv.Atoi(strings.TrimSpace(e.CopyrightYear)); err == nil {
			b.CopyrightYear = y
			p.env.via("copyright_year", LevelLinkedData, "copyrightYear")
		}
	}
	if b.CopyrightYear == 0 {
		if y := lastYear(b.Copyright); y > 0 {
			b.CopyrightYear = y
			p.env.via("copyright_year", LevelSelector, "the bibliographic table, Copyright Information")
		}
	}

	if e != nil && len(e.Author) > 0 {
		b.Authors = people(e.Author)
		p.env.via("authors", LevelLinkedData, "author[]")
	}
	if e != nil && len(e.Editor) > 0 {
		b.Editors = people(e.Editor)
		p.env.via("editors", LevelLinkedData, "editor[]")
	}
	if len(b.Authors) == 0 && len(b.Editors) == 0 {
		if names := listItems(p.reg.first("authors-listing")); len(names) > 0 {
			b.Authors = named(names)
			p.env.via("authors", LevelRegion, "[data-test=authors-listing]")
		} else {
			p.env.miss("authors", "neither the schema.org block nor the author listing named anybody")
		}
	}

	b.Series = bookSeries(p, bib)
	b.Subjects = bookSubjects(p, e, bib)
	if kw := ldKeywords(e); len(kw) > 0 {
		b.Keywords = kw
		p.env.via("keywords", LevelLinkedData, "keywords")
	}
	if pkgs := splitList(bib["eBook Packages"]); len(pkgs) > 0 {
		b.Packages = pkgs
		p.env.via("packages", LevelSelector, "the bibliographic table, eBook Packages")
	}

	// Three publication dates, because there are three editions and they are a
	// year apart on the measured capture. The electronic one is the date that
	// matches the doi, so it is the one called published.
	b.Published = bookDate(p, "published", "electronic_isbn_publication_date")
	b.PublishedHardcover = bookDate(p, "published_hardcover", "hardcover_isbn_publication_date")
	b.PublishedSoftcover = bookDate(p, "published_softcover", "softcover_isbn_publication_date")

	b.access(p, e)
	b.Chapters = chaptersOf(p)
	b.ChapterCount = firstInt(p.reg.text("book-bits"))
	if b.ChapterCount == 0 && len(b.Chapters) > 0 {
		b.ChapterCount = len(b.Chapters)
	}
	if len(b.Chapters) > 0 {
		p.env.via("chapters", LevelRegion, "[data-test=chapter]")
	}

	if n := firstInt(p.reg.text("access-count")); n > 0 {
		b.Accesses = n
		p.env.via("accesses", LevelRegion, "[data-test=access-count]")
	}
	if n := firstInt(p.reg.text("citation-count")); n > 0 {
		b.Citations = n
		p.env.via("citations", LevelRegion, "[data-test=citation-count]")
	}

	b.Offers = offersOf(p)
	b.Conference = conferenceOf(b)

	b.Envelope = p.finish()
	return b, nil
}

// access reads what the page says this reader is being given.
//
// A book page states it in the head as meta access and again in the analytics
// payload as hasAccess. They agreed on the measured capture, both saying no,
// and the disagreement is what is worth recording rather than resolved.
func (b *Book) access(p *page, e *ldEntity) {
	raw := p.meta.First("access")
	if raw != "" {
		v := strings.EqualFold(raw, "yes")
		b.Access.Free = &v
		b.Access.Raw = raw
		p.env.via("access", LevelHighwire, "access")
	}
	if layer := p.dl.str("hasAccess"); layer != "" && b.Access.Free != nil {
		if strings.EqualFold(layer, "Y") != *b.Access.Free {
			p.env.miss("access", "meta access says "+raw+" and the analytics payload says hasAccess "+layer+
				"; the page is stating two different things and this tool does not pick")
		}
	}
	if b.Access.Free == nil && e != nil && e.IsAccessibleForFree != nil {
		v := *e.IsAccessibleForFree
		b.Access.Free = &v
		p.env.via("access", LevelLinkedData, "isAccessibleForFree")
	}
	if b.Access.Free == nil {
		p.env.miss("access", "the page declared neither meta access nor an analytics access flag")
	}
	if oa := p.dl.str("Open Access"); oa != "" {
		b.Access.OAStatus = p.set("access.oa_status", LevelRegion, "datalayer.Open Access", map[string]string{"Y": "open", "N": "closed"}[oa])
	}
}

// bookSeries reads the series this book belongs to.
//
// The printed line is "Part of the book series: Population Economics
// (POPULATION)", which is the name, an acronym in brackets and a link. The
// analytics payload states the series id, which is what the url is built from,
// so the two are put together rather than either being trusted alone.
func bookSeries(p *page, bib map[string]string) *Ref {
	id := p.dl.str("content", "book", "seriesId")
	name := p.dl.str("content", "book", "seriesTitle")
	node := p.reg.first("series-link")
	href := linkHref(node)

	if name == "" {
		name = bib["Series Title"]
	}
	if name == "" && node != nil {
		name = strings.TrimSpace(strings.TrimPrefix(text(node), "Part of the book series:"))
	}
	if name == "" && id == "" && href == "" {
		return nil
	}
	ref := &Ref{Kind: "series", ID: id, Name: strings.TrimSpace(name), URL: href}
	if ref.URL == "" && id != "" {
		ref.URL = Base + "/series/" + id
	}
	p.env.via("series", LevelRegion, "datalayer.content.book.seriesId + [data-test=series-link]")
	return ref
}

// bookSubjects reads Springer's own classification of the book.
//
// Three sources state it and they do not agree in shape. The analytics payload
// gives three broad subjects, the schema.org genre gives two with the coarse
// bucket in brackets, and the bibliographic table's Topics row gives three
// linked terms. The payload is taken because it is the only machine readable
// one, and the table is the fallback.
func bookSubjects(p *page, e *ldEntity, bib map[string]string) []string {
	if s := p.dl.list("content", "category", "snt"); len(s) > 0 {
		p.env.via("subjects", LevelRegion, "datalayer.content.category.snt")
		return s
	}
	if s := splitList(bib["Topics"]); len(s) > 0 {
		p.env.via("subjects", LevelSelector, "the bibliographic table, Topics")
		return s
	}
	if e != nil {
		if s := e.Genre.List(); len(s) > 0 {
			p.env.via("subjects", LevelLinkedData, "genre")
			return s
		}
	}
	return nil
}

// bookDate reads one of the three edition dates, which the page prints as
// "Published: 27 April 2023".
func bookDate(p *page, field, name string) *Date {
	raw := afterColon(p.reg.text(name))
	if raw == "" {
		return nil
	}
	d, err := ParseDate(raw)
	if err != nil {
		p.env.miss(field, "the page printed "+strconv.Quote(raw)+" and it is not a date in any form this site is known to print")
		return nil
	}
	p.env.via(field, LevelRegion, "[data-test="+name+"]")
	return &d
}

// isbnOf validates an isbn and keeps what the page printed when it does not
// check out, with the failure named in the envelope.
func isbnOf(p *page, field, raw, source string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	level := LevelSelector
	if strings.HasPrefix(source, "datalayer") {
		level = LevelRegion
	}
	p.env.via(field, level, source)
	isbn, err := ParseISBN(raw)
	if err != nil {
		p.env.miss(field, "the page printed "+strconv.Quote(raw)+" and it is not a valid isbn: "+err.Error())
		return raw
	}
	return string(isbn)
}

// chaptersOf reads the table of contents.
//
// Each row is one <li data-test="chapter"> holding a heading, an author list
// and its page range, so the page number is read from inside the row rather
// than by zipping two flat lists that happen to be the same length today. Front
// matter and back matter are rows in the same list and are kept, with their
// matter named, because a consumer counting chapters needs to be able to drop
// the two that are not chapters.
func chaptersOf(p *page) []Chapter {
	var out []Chapter
	for _, li := range p.reg.all("chapter") {
		ch := Chapter{Position: len(out)}
		if h := firstTag(li, atom.H3); h != nil {
			ch.Title = text(h)
			switch attr(h, "data-test") {
			case "front-matter":
				ch.Matter = "front"
			case "back-matter":
				ch.Matter = "back"
			}
			if a := firstTag(h, atom.A); a != nil {
				ch.URL = trimQuery(attr(a, "href"))
				ch.DOI = doiFromPath(attr(a, "href"))
			}
		}
		for _, span := range findClass(li, "c-meta__item") {
			if attr(span, "data-test") == "page-number" {
				ch.Pages = strings.TrimSpace(strings.TrimPrefix(text(span), "Pages"))
				break
			}
		}
		if ul := firstClass(li, "app-author-list"); ul != nil {
			ch.Authors = listItems(ul)
		}
		if ch.Title == "" {
			continue
		}
		out = append(out, ch)
	}
	// The page numbers were read out of the rows, and the region name is marked
	// read here so that it does not show up as something this tool ignored.
	p.reg.all("page-number")
	p.reg.all("front-matter")
	p.reg.all("back-matter")
	return out
}

// offersOf reads the commerce block.
//
// The kind comes from the order form's own hidden type field rather than from
// the printed label, because the label is prose that changes with the locale
// and the field is the value the publisher posts to its own cart. The price
// keeps the printed string beside the parse, and the currency is read rather
// than assumed, since prices on this site are localized by requesting IP.
func offersOf(p *page) []Offer {
	box := p.reg.first("buy-box-mobile")
	if box == nil {
		return nil
	}
	var out []Offer
	seen := map[string]bool{}
	for _, opt := range findClass(box, "buying-option") {
		o := Offer{}
		for _, in := range findTag(opt, atom.Input) {
			if attr(in, "name") == "type" {
				o.Kind = attr(in, "value")
				break
			}
		}
		if dt := firstClass(opt, "dt"); dt != nil {
			o.Label = text(dt)
		}
		if amt := firstClass(opt, "price-amount"); amt != nil {
			o.Price = parseMoney(text(amt))
		}
		key := o.Kind + "|" + o.Label
		if o.Kind == "" && o.Label == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, o)
	}
	if len(out) > 0 {
		p.env.via("offers", LevelRegion, "[data-test=buy-box-mobile] .buying-option")
	}
	// The institutional access line is part of the same block and says the
	// other way in, which is not a price.
	p.reg.all("access-via-institution")
	return out
}

// parseMoney reads "EUR 85.59" into a currency and an amount, keeping the
// printed string. A price whose number does not parse still carries its raw
// text, because what the page said is the evidence for whatever comes next.
func parseMoney(s string) *Money {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	m := &Money{Raw: s}
	fields := strings.Fields(s)
	for _, f := range fields {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(f, ",", ""), 64); err == nil {
			m.Amount = v
			continue
		}
		if m.Currency == "" && len(f) == 3 && strings.ToUpper(f) == f {
			m.Currency = f
		}
	}
	return m
}

// conferenceOf builds a conference from what the book page says, and only when
// the page says something.
//
// There is no conference page on this site: /conference/aaai is a 404 with a
// zero byte body. So a conference is never fetched and never has a url built
// for it out of an acronym. What exists is a proceedings volume whose title
// names the conference, and this reads that title and nothing else.
func conferenceOf(b *Book) *Conference {
	c, ok := ParseConferenceTitle(b.Title)
	if !ok {
		return nil
	}
	c.Proceedings = &Ref{Kind: "book", ID: b.ISBNElectronic, Name: b.Title, URL: b.URL}
	c.Series = b.Series
	return &c
}

// bibliographic reads the printed bibliographic table into a label to value
// map.
//
// It is rung 4, it is the only source for six fields, and it is matched on the
// printed English label, which is the weakest kind of match this tool makes.
// The alternative is not having an edition number, a page extent, a publisher
// or a series issn at all, since nothing above rung 4 states any of them on a
// book page.
func bibliographic(root *html.Node) map[string]string {
	out := map[string]string{}
	for _, li := range findClass(root, "c-bibliographic-information__list-item") {
		label := strings.TrimSpace(text(firstClass(li, "u-text-bold")))
		if label == "" {
			continue
		}
		if s := firstClass(li, "c-bibliographic-information__value"); s != nil {
			if v := strings.TrimSpace(text(s)); v != "" && out[label] == "" {
				out[label] = v
			}
		}
	}
	return out
}

// splitList breaks a comma joined printed list, which is how the table prints
// its topics and its ebook packages.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ldKeywords returns the schema.org keywords, which arrive as one comma joined
// string on a book and as a list elsewhere.
func ldKeywords(e *ldEntity) []string {
	if e == nil {
		return nil
	}
	return e.Keywords.Split()
}
