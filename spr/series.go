package spr

import (
	"errors"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The series page.
//
// 9 meta names, none bibliographic, and an analytics payload that states two
// things and only two: the series id and the series title. That is the thinnest
// payload on the site, and it is worth noting that the id arrives there as a
// JSON number where the journal's arrives as a string, which is why the reader
// accepts both rather than assuming either.
//
// Everything else is printed. The two ISSNs, the editorial board, the promo
// text and the five most recent books all come out of named regions, and the
// rest of the catalogue is not on this page at all.

// ErrNotASeries is returned for a page that is not a series home page.
var ErrNotASeries = errors.New("this page is not a series home page")

// SeriesPath reports whether a url addresses a series home page. A path with
// anything after the series id is a subpage and a different record.
func SeriesPath(raw string) bool {
	id, sub := seriesParts(raw)
	return id != "" && sub == ""
}

// seriesParts splits a series url into the series id and whatever follows.
func seriesParts(raw string) (id, sub string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	rest, ok := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/series/")
	if !ok {
		return "", ""
	}
	id, sub, _ = strings.Cut(rest, "/")
	if _, err := ParseSpringerID(id); err != nil {
		return "", ""
	}
	return id, sub
}

// ExtractSeries reads a series record out of a fetched series home page.
func ExtractSeries(resp *Response) (*Series, error) {
	if resp == nil {
		return nil, errors.New("no response to extract from")
	}
	if !SeriesPath(resp.URL) && !SeriesPath(resp.Final) {
		return nil, ErrNotASeries
	}
	p, err := newPage(resp)
	if err != nil {
		return nil, err
	}

	s := &Series{URL: resp.URL}

	s.SeriesID = p.layer("series_id", "Book Series Id")
	if s.SeriesID == "" {
		if id, _ := seriesParts(resp.URL); id != "" {
			s.SeriesID = p.set("series_id", LevelSelector, "the url path", id)
		}
	}
	s.Title = p.layer("title", "Book Series Title")
	if s.Title == "" {
		s.Title = p.region("title", "series-title")
	}
	if s.Title == "" {
		p.env.miss("title", "neither the analytics payload nor the masthead named the series")
	}

	s.ElectronicISSN = issnOf(p, "electronic_issn", "series-eissn")
	s.PrintISSN = issnOf(p, "print_issn", "series-issn")

	// The board is one dl per role, exactly as on a journal, and the role is
	// Series Editor rather than Editor-in-Chief. The same reader handles both.
	s.Editors = editorsOf(p, "editor-links-1")

	s.About = p.region("about", "series-about-text-description")
	if names := listItems(p.reg.first("series-abstract-and-index-services-list")); len(names) > 0 {
		s.IndexedIn = names
		p.env.via("indexed_in", LevelRegion, "[data-test=series-abstract-and-index-services-list]")
	}

	s.LatestTitles, s.Titles = seriesTitles(p)
	if s.Titles == nil && s.SeriesID != "" {
		s.Titles = &Conn{Loaded: len(s.LatestTitles), URL: Base + "/series/" + s.SeriesID + "/books"}
	}

	s.Envelope = p.finish()
	return s, nil
}

// seriesTitles reads the recent books box and the link to the rest of them.
//
// The page ships [data-test=latest-titles] twice: once around the list of five
// cards and once around the View all book titles button. They are told apart by
// whether the node holds an ordered list, rather than by taking the first, so
// that a page which ever reorders the two still reads correctly.
func seriesTitles(p *page) ([]SeriesTitle, *Conn) {
	var out []SeriesTitle
	var conn *Conn
	for _, box := range p.reg.all("latest-titles") {
		lists := findTag(box, atom.Ol)
		if len(lists) == 0 {
			if href := linkHref(box); href != "" {
				conn = &Conn{URL: href}
			}
			continue
		}
		for _, card := range findClass(box, "c-card") {
			t := seriesTitleOf(card)
			if t.Title != "" {
				out = append(out, t)
			}
		}
	}
	if len(out) > 0 {
		p.env.via("latest_titles", LevelRegion, "[data-test=latest-titles] .c-card")
	}
	if conn != nil {
		conn.Loaded = len(out)
	}
	// The two flags live inside the cards and are read there.
	p.reg.all("book-copyright-year")
	p.reg.all("book-open-access")
	return out, conn
}

// seriesTitleOf reads one book card.
//
// Authors and editors are told apart by the card's own printed dt, "Authors:"
// or "Editors:", and not by the itemprop beside it, which says editor on both.
// A card that credits authors and a card that credits editors are different
// facts about the book and the page is careful to say which, so this is too.
func seriesTitleOf(card *html.Node) SeriesTitle {
	t := SeriesTitle{}
	if a := firstTag(firstTag(card, atom.H3), atom.A); a != nil {
		t.Title = strings.TrimSpace(text(a))
		t.URL = trimQuery(attr(a, "href"))
	}
	for _, dl := range findTag(card, atom.Dl) {
		label := strings.ToLower(strings.TrimSpace(term(dl)))
		names := listItems(dl)
		switch {
		case len(names) == 0:
		case strings.HasPrefix(label, "editor"):
			t.Editors = append(t.Editors, names...)
		default:
			t.Authors = append(t.Authors, names...)
		}
	}
	for _, li := range findClass(card, "c-meta__item") {
		switch attr(li, "data-test") {
		case "book-copyright-year":
			t.CopyrightYear = lastYear(text(li))
		case "book-open-access":
			t.OpenAccess = true
		}
	}
	return t
}
