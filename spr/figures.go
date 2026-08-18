package spr

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The /figures/N and /tables/N subpages.
//
// They look like a pair and they are not. The figures page names five regions,
// figure, top-caption, bottom-caption, subtitle and back-link, and reading it is
// rung 3 throughout. The tables page names exactly one, subtitle, and everything
// else on it is a bare <table class="data"> inside a container div, which is
// rung 4 from the heading down. Same site, same release, two components that
// were plainly written years apart, and the extraction table says which is
// which rather than pretending they are symmetrical.
//
// A figure number out of range is the trap here. /figures/99 on an article with
// 17 figures answers 200 with 224 KB of page furniture and an empty <main>, so
// the classifier sees a healthy page and only the extractor can tell. That is
// the M1 finding turning up in a new place and it gets its own error.

// ErrNoSuchFigure and ErrNoSuchTable are returned for a subpage that answered
// 200 with nothing in it, which is what an out of range number does here.
var (
	ErrNoSuchFigure = errors.New("the page answered but carries no figure, so this article has no figure with that number")
	ErrNoSuchTable  = errors.New("the page answered but carries no table, so this article has no table with that number")
)

// satelliteRe splits a work url from the subpage it addresses.
var satelliteRe = regexp.MustCompile(`^(.*)/(figures|tables)/(\d+)$`)

// FigureURL and TableURL address one asset of a work.
func FigureURL(work string, n int) string { return satelliteURL(work, "figures", n) }
func TableURL(work string, n int) string  { return satelliteURL(work, "tables", n) }

func satelliteURL(work, kind string, n int) string {
	return fmt.Sprintf("%s/%s/%d", strings.TrimSuffix(trimQuery(work), "/"), kind, n)
}

// satelliteParts splits a subpage url into the work it belongs to, which kind
// of asset it is, and the number.
func satelliteParts(raw string) (work, kind string, n int) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", 0
	}
	m := satelliteRe.FindStringSubmatch(strings.TrimSuffix(u.Path, "/"))
	if m == nil {
		return "", "", 0
	}
	num, err := strconv.Atoi(m[3])
	if err != nil {
		return "", "", 0
	}
	return m[1], m[2], num
}

// FigurePath and TablePath report whether a url addresses one of these pages.
func FigurePath(raw string) bool { _, k, _ := satelliteParts(raw); return k == "figures" }
func TablePath(raw string) bool  { _, k, _ := satelliteParts(raw); return k == "tables" }

// WorkURL trims a url back to the work its subpages hang off, and returns
// anything else unchanged.
//
// It exists so that pasting an address out of a browser does the obvious thing.
// Somebody looking at /figures/1 who asks for figure 4 means the fourth figure
// of that article, not /figures/1/figures/4, and the only way to know that is to
// recognize the address they pasted.
// Like the constructors above it, this returns an absolute url, so a path goes
// in and a link.springer.com address comes out.
func WorkURL(raw string) string {
	raw = strings.TrimSuffix(trimQuery(raw), "/")
	if m := satelliteRe.FindStringSubmatch(raw); m != nil {
		return m[1]
	}
	return strings.TrimSuffix(raw, "/metrics")
}

// ExtractFigure reads a figure record out of a fetched /figures/N page.
func ExtractFigure(resp *Response) (*FigurePage, error) {
	p, n, err := satellitePage(resp, "figures")
	if err != nil {
		return nil, err
	}
	fig := p.reg.first("figure")
	if fig == nil {
		return nil, ErrNoSuchFigure
	}

	f := &FigurePage{URL: resp.URL, Number: n}
	f.Label = p.region("label", "top-caption")
	if h := p.reg.first("top-caption"); h != nil {
		if id := attr(h, "id"); id != "" {
			f.Anchor = "#" + id
		}
	}
	if f.Number == 0 {
		f.Number = firstInt(f.Label)
	}
	f.Caption = p.region("caption", "bottom-caption")
	f.ArticleTitle, f.ArticleURL = satelliteParent(p)

	if img := firstTag(fig, atom.Img); img != nil {
		f.Image = &Image{
			URL:    absolute(attr(img, "src")),
			Alt:    attr(img, "alt"),
			Width:  firstInt(attr(img, "width")),
			Height: firstInt(attr(img, "height")),
		}
		// The webp is a <source srcset> beside the img, one url with no
		// descriptor on this page. Only that shape is read, because a srcset
		// with widths in it is a list of candidates and picking one of those
		// would be this tool choosing a rendition rather than reporting one.
		if s := firstTag(fig, atom.Source); s != nil {
			if set := strings.TrimSpace(attr(s, "srcset")); set != "" && !strings.Contains(set, ",") {
				f.Image.WebP = absolute(strings.Fields(set)[0])
			}
		}
		p.env.via("image", LevelRegion, "[data-test=figure] img")
	}

	// The caption's own citations. The link text is only the year and the whole
	// reference string is in the title attribute, so both are kept.
	for _, a := range p.reg.all("citation-ref") {
		r := CaptionRef{
			Text:     strings.TrimSpace(text(a)),
			URL:      absolute(attr(a, "href")),
			Citation: strings.TrimSpace(attr(a, "title")),
		}
		if r.Text == "" && r.Citation == "" {
			continue
		}
		f.Refs = append(f.Refs, r)
	}
	if len(f.Refs) > 0 {
		p.env.via("refs", LevelRegion, "[data-test=citation-ref]")
	}

	// The back link states the same anchor the heading id does. It is read as
	// the fallback, and asked for either way so that the region does not show
	// up as one this tool left on the table.
	if a := p.reg.first("back-link"); a != nil && f.Anchor == "" {
		if _, frag, ok := strings.Cut(attr(a, "href"), "#"); ok && frag != "" {
			f.Anchor = "#" + frag
		}
	}

	f.Envelope = p.finish()
	return f, nil
}

// ExtractTable reads a table record out of a fetched /tables/N page.
func ExtractTable(resp *Response) (*TablePage, error) {
	p, n, err := satellitePage(resp, "tables")
	if err != nil {
		return nil, err
	}
	tbl := firstTag(firstClass(p.doc, "c-article-table-container"), atom.Table)
	if tbl == nil {
		return nil, ErrNoSuchTable
	}

	t := &TablePage{URL: resp.URL, Number: n}

	// The heading is one string with the label and the caption run together,
	// "Table 1 Notation used throughout the paper", where a figure prints the
	// two in separate elements. They are split on the label rather than left
	// joined, so that a caption is a caption on both records.
	if head := firstClass(p.doc, "c-article-satellite-title"); head != nil {
		t.Label, t.Caption = splitLabel(text(head))
		p.env.via("label", LevelSelector, ".c-article-satellite-title")
		if t.Caption != "" {
			p.env.via("caption", LevelSelector, ".c-article-satellite-title")
		}
	}
	if t.Number == 0 {
		t.Number = firstInt(t.Label)
	}
	t.ArticleTitle, t.ArticleURL = satelliteParent(p)

	// The heading's own id is table-1-title, which is this page's anchor and
	// not the article's. The article anchor is #Tab1 and the back link is the
	// only thing on the page that states it.
	for _, a := range findTag(p.doc, atom.A) {
		if _, frag, ok := strings.Cut(attr(a, "href"), "#Tab"); ok && frag != "" {
			t.Anchor = "#Tab" + frag
			break
		}
	}

	t.Head, t.Rows = tableCells(tbl)
	if len(t.Head) > 0 || len(t.Rows) > 0 {
		p.env.via("rows", LevelSelector, ".c-article-table-container table")
	}

	t.Envelope = p.finish()
	return t, nil
}

// satellitePage parses a subpage response and checks it addresses what the
// caller asked for.
func satellitePage(resp *Response, kind string) (*page, int, error) {
	if resp == nil {
		return nil, 0, errors.New("no response to extract from")
	}
	_, k, n := satelliteParts(resp.URL)
	if k != kind {
		if _, k2, n2 := satelliteParts(resp.Final); k2 == kind {
			k, n = k2, n2
		}
	}
	if k != kind {
		return nil, 0, fmt.Errorf("this url is not a /%s/N subpage", kind)
	}
	p, err := newPage(resp)
	if err != nil {
		return nil, 0, err
	}
	return p, n, nil
}

// satelliteParent names the work this asset was published in.
//
// All three subpages carry the parent article's whole bibliographic head, 66
// meta names in Highwire, Dublin Core and PRISM, exactly as the article page
// does. So the title comes off rung 1 here even though the page also prints it
// in a From line, and this is the one place in the tool where a subsidiary page
// is better identified than the container pages that outrank it. The printed
// link is the fallback and supplies the address either way.
func satelliteParent(p *page) (title, addr string) {
	title = p.set("article_title", LevelHighwire, "citation_title", p.meta.First("citation_title", "dc.title"))
	a := p.reg.first("subtitle")
	if a != nil {
		addr = absolute(trimQuery(attr(a, "href")))
		if title == "" {
			title = p.set("article_title", LevelRegion, "[data-test=subtitle]", text(a))
		}
	}
	if addr != "" {
		p.env.via("article_url", LevelRegion, "[data-test=subtitle] href")
	}
	return title, addr
}

// labelRe matches the printed label a satellite heading opens with.
var labelRe = regexp.MustCompile(`^((?:Fig\.?|Figure|Table|Scheme)\s*\d+)\.?\s*(.*)$`)

// splitLabel separates "Table 1" from the caption behind it. A heading that
// does not open with a label comes back whole as the caption, since inventing a
// label would be worse than not having one.
func splitLabel(s string) (label, caption string) {
	s = strings.TrimSpace(s)
	m := labelRe.FindStringSubmatch(s)
	if m == nil {
		return "", s
	}
	return m[1], strings.TrimSpace(m[2])
}

// tableCells reads the header row and the body rows as printed text.
//
// A cell's LaTeX is kept as the source string the publisher wrote. Rendering
// \(\mathcal{X}\) into anything else is this tool having an opinion about
// notation, and a consumer that wants it rendered has a renderer.
func tableCells(tbl *html.Node) (head []string, rows [][]string) {
	for _, tr := range findTag(tbl, atom.Tr) {
		var cells []string
		header := true
		for _, c := range rowCells(tr) {
			if c.DataAtom != atom.Th {
				header = false
			}
			cells = append(cells, strings.TrimSpace(text(c)))
		}
		if len(cells) == 0 {
			continue
		}
		if header && head == nil {
			head = cells
			continue
		}
		rows = append(rows, cells)
	}
	return head, rows
}

// rowCells returns the th and td of one row as immediate children only, so that
// a table nested inside a cell does not have its own cells flattened up into
// the row that contains it.
func rowCells(tr *html.Node) []*html.Node {
	var out []*html.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.DataAtom == atom.Th || c.DataAtom == atom.Td) {
			out = append(out, c)
		}
	}
	return out
}
