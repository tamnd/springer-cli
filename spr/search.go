package spr

import (
	"errors"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// /search, the enrichment path.
//
// A search hit gets its own record rather than a Work, because a card carries
// eight facts and a Work has room for eighty, and a list of mostly empty Works
// is a list that lies about what was fetched.
//
// This page is rung 3 from top to bottom. There is no bibliographic meta head
// on a search page, no schema.org block describing the results, and the whole
// record comes from Springer's own data-test names: 20 search-result-item, and
// inside each of them content-type, title, description, authors, parent and
// published, with oa-label on the 10 that are open access and entitlements on
// 11.
//
// The one number that exists only here is the total. results-data-total prints
// "Showing 1-20 of 557 results", the feed carries nothing like it, and that is
// why a search that ran on rss alone reports no total instead of inferring one
// by paging to the end.

// Via names which path produced a result. It is on every result rather than
// only in the envelope, because one run can assemble a set from both paths and
// the field coverage differs between them. A consumer that sees rss knows why
// content_type is absent without having to be told.
const (
	ViaRSS  = "rss"
	ViaHTML = "html"
)

// SearchResult is one hit.
type SearchResult struct {
	DOI      string `json:"doi,omitempty"`
	Title    string `json:"title,omitempty"`
	Abstract string `json:"abstract,omitempty"`

	// Snippet is the card's truncated description, which ends in an ellipsis
	// and is not the abstract. Both are kept because a run on html alone has
	// only this one, and calling it abstract would make a hundred and eighty
	// characters look like a paper's summary.
	Snippet string `json:"snippet,omitempty"`

	URL       string `json:"url,omitempty"`
	Published *Date  `json:"published,omitempty"`

	ContentType string   `json:"content_type,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	Container   *Ref     `json:"container,omitempty"`
	OpenAccess  bool     `json:"open_access,omitempty"`

	// Access is the card's own entitlement wording, Full access or Preview,
	// which is a statement about the fetching client and not about the work.
	Access string `json:"access,omitempty"`

	Position int    `json:"position,omitempty"`
	Via      string `json:"via,omitempty"`
}

// Facet is one option inside a facet group, with the count the page printed
// beside it.
type Facet struct {
	Label string `json:"label,omitempty"`

	// Value is what the form would send, quotes included. They are kept
	// because sending a taxonomy value without them matches nothing and
	// answers 200, so a facet listing that stripped them would be handing out
	// values that silently fail.
	Value string `json:"value,omitempty"`

	Count    int  `json:"count,omitempty"`
	Selected bool `json:"selected,omitempty"`
}

// FacetGroup is one of the eight groups the sidebar offers.
type FacetGroup struct {
	Group string  `json:"group,omitempty"`
	Param string  `json:"param,omitempty"`
	Items []Facet `json:"items,omitempty"`

	// Applied is what this group currently has set when the group states it as
	// a value rather than as an option. Only the date group does this, with the
	// two text inputs holding the years the query was sent with.
	Applied []string `json:"applied,omitempty"`
}

// SearchPage is one page of html search results.
type SearchPage struct {
	// Total is the result count, which is stated only here.
	Total int `json:"total,omitempty"`

	// First and Last are the range this page covers, read off the same
	// sentence as the total.
	First int `json:"first,omitempty"`
	Last  int `json:"last,omitempty"`

	Results []SearchResult `json:"results,omitempty"`
	Facets  []FacetGroup   `json:"facets,omitempty"`

	// Applied is the filter chips the page prints back, Article and 2020-2024,
	// which is the site restating what it understood the query to be.
	Applied []string `json:"applied,omitempty"`

	// Sorts are the orders the select offers, read off the page rather than
	// compiled in.
	Sorts []string `json:"sorts,omitempty"`

	// HasNext is whether the page printed a next link, which is a cheaper
	// termination signal than arithmetic on the total.
	HasNext bool `json:"has_next,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// SearchResponse is one run of the search command, whichever paths answered.
type SearchResponse struct {
	Query   Query          `json:"query"`
	Total   int            `json:"total,omitempty"`
	Page    int            `json:"page,omitempty"`
	PerPage int            `json:"per_page,omitempty"`
	Results []SearchResult `json:"results,omitempty"`
	Facets  []FacetGroup   `json:"facets,omitempty"`

	// Paths names which surfaces answered, rss, html or both.
	Paths []string `json:"paths,omitempty"`

	// Notes are the things a caller has a right to know about this run: that
	// enrichment was challenged, that the requested sort was not honoured, that
	// the total is unavailable. They are printed on stderr once and carried in
	// the record so that a json consumer gets them too.
	Notes []string `json:"notes,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// ErrNotSearch is returned for a page that is not a search results page.
var ErrNotSearch = errors.New("this page is not a search results page")

// facetGroups maps the sidebar's own region names to the parameter each group
// posts under.
//
// Three of the eight post under a name that is not their group name, and one of
// those, publishing-model posting openAccess, is not even the same word. So
// this is a measured table and not a transformation of the group name.
var facetGroups = []struct{ group, param string }{
	{"content-type", "content-type"},
	{"publishing-model", "openAccess"},
	{"language", "language"},
	{"taxonomy", paramTaxonomy},
	{"discipline", paramDiscipline},
	{"sub-discipline", paramSubDiscipline},
	{"sustainable-development-goal", paramSDG},
	{"date", "date"},
}

// ExtractSearch reads one page of html search results.
func ExtractSearch(resp *Response) (*SearchPage, error) {
	p, err := newPage(resp)
	if err != nil {
		return nil, err
	}
	cards := p.reg.all("search-result-item")
	total := p.reg.first("results-data-total")
	if len(cards) == 0 && total == nil {
		return nil, ErrNotSearch
	}

	s := &SearchPage{}
	s.First, s.Last, s.Total = parseShowing(text(total))
	if s.Total > 0 {
		p.env.via("total", LevelRegion, "[data-test=results-data-total]")
	} else if total != nil {
		p.env.miss("total", "the page printed "+strconv.Quote(collapse(text(total)))+" and no count could be read out of it")
	} else {
		p.env.miss("total", "this page carries no results-data-total, so the result count is not available from it")
	}

	for i, card := range cards {
		s.Results = append(s.Results, searchCard(p, card, i+1))
	}
	if len(s.Results) > 0 {
		p.env.via("results", LevelRegion, "[data-test=search-result-item]")
	}

	s.Facets = searchFacets(p)
	if len(s.Facets) > 0 {
		p.env.via("facets", LevelRegion, "[data-test=*-facet-item]")
	}

	for _, a := range p.reg.all("applied-filter") {
		if t := visibleText(a); t != "" {
			s.Applied = append(s.Applied, t)
		}
	}
	if len(s.Applied) > 0 {
		p.env.via("applied", LevelRegion, "[data-test=applied-filter]")
	}

	if sel := firstTag(p.reg.first("sorting-options"), atom.Select); sel != nil {
		for _, o := range findTag(sel, atom.Option) {
			if v := strings.TrimSpace(attr(o, "value")); v != "" {
				s.Sorts = append(s.Sorts, v)
			}
		}
		p.env.via("sorts", LevelRegion, "[data-test=sorting-options] select")
	}

	s.HasNext = p.reg.first("next-page") != nil
	// Asked for whether or not it is read, so that the two pagination regions
	// do not show up in the drift list as components this tool ignored.
	p.reg.first("pagination")

	s.Envelope = p.finish()
	return s, nil
}

// searchCard reads one result card.
func searchCard(p *page, card *html.Node, pos int) SearchResult {
	r := SearchResult{Position: pos, Via: ViaHTML}
	r.ContentType = collapse(text(p.reg.firstIn(card, "content-type")))

	if h := p.reg.firstIn(card, "title"); h != nil {
		r.Title = collapse(text(h))
		if a := firstTag(h, atom.A); a != nil {
			r.URL = trimQuery(attr(a, "href"))
			r.DOI = doiFromPath(attr(a, "href"))
		}
	}
	r.Snippet = collapse(text(p.reg.firstIn(card, "description")))

	// The card prints the authors as one string with commas in it, and carries
	// no orcid, no affiliation and no author page link. Splitting on the comma
	// and stopping there is the whole of what the page supports. Promoting
	// these to Author records would create person records with a name and
	// nothing else in them.
	if a := p.reg.firstIn(card, "authors"); a != nil {
		for _, n := range strings.Split(text(a), ",") {
			if n = collapse(n); n != "" {
				r.Authors = append(r.Authors, n)
			}
		}
	}

	if a := p.reg.firstIn(card, "parent"); a != nil {
		href := attr(a, "href")
		r.Container = &Ref{
			Kind: containerKind(href),
			Name: collapse(text(a)),
			URL:  trimQuery(href),
			ID:   containerID(href),
		}
	}

	if d := p.reg.firstIn(card, "published"); d != nil {
		// The card prints 25 November 2024 where the feed sends 2024-11-25 for
		// the same work. Both are read by the same parser, which is why the
		// layouts in date.go are a measured list rather than one format string.
		if v, err := ParseDate(collapse(text(d))); err == nil {
			r.Published = &v
		}
	}

	r.OpenAccess = p.reg.firstIn(card, "oa-label") != nil
	r.Access = collapse(text(p.reg.firstIn(card, "entitlements")))
	return r
}

// searchFacets reads the eight facet groups.
func searchFacets(p *page) []FacetGroup {
	var out []FacetGroup
	for _, g := range facetGroups {
		items := p.reg.all(g.group + "-facet-item")
		if len(items) == 0 {
			continue
		}
		fg := FacetGroup{Group: g.group, Param: g.param}
		for _, in := range items {
			// The date group is 5 radios and 2 text inputs in one region name.
			// The radios are the windows on offer, m3 to m24 and custom. The
			// text inputs are the years this query was sent with, echoed back.
			// Reading all 7 as options produces "2020" and "2024" as things a
			// caller could pick, which they are not.
			if attr(in, "type") == "text" {
				if v := strings.TrimSpace(attr(in, "value")); v != "" {
					fg.Applied = append(fg.Applied, attr(in, "name")+"="+v)
				}
				continue
			}
			f := Facet{
				Value:    attr(in, "value"),
				Selected: hasAttr(in, "checked"),
			}
			f.Label, f.Count = facetLabel(p.doc, attr(in, "id"))
			if f.Label == "" {
				f.Label = f.Value
			}
			fg.Items = append(fg.Items, f)
		}
		out = append(out, fg)
	}
	return out
}

// facetLabel finds the label belonging to one facet input and splits the
// printed name from the count in brackets behind it.
//
// The two are separate spans with their own classes, so this reads the classes
// rather than parsing "(557)" back out of a joined string. A label with a
// bracket in its own text would otherwise lose the tail of its name.
func facetLabel(root *html.Node, id string) (label string, count int) {
	if id == "" {
		return "", 0
	}
	for _, l := range findTag(root, atom.Label) {
		if attr(l, "for") != id {
			continue
		}
		label = collapse(firstClassText(l, "app-search-filter__filter-name"))
		count = firstInt(firstClassText(l, "app-search-filter__filter-count"))
		if label == "" {
			label = collapse(text(l))
		}
		return label, count
	}
	return "", 0
}

// visibleText is the text of a node without the parts written for a screen
// reader.
//
// The applied filter chips are the reason. Each one is a link whose text reads
// "Remove filter Article", where "Remove filter " is a u-visually-hidden span
// naming the action and "Article" is the filter. Taking the whole string gives
// a list of filters called "Remove filter Article" and "Remove filter
// 2020-2024", which is the button's instruction and not the query's state.
func visibleText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, "u-visually-hidden") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return collapse(b.String())
}

// firstClassText is the collapsed text of the first node with this class.
func firstClassText(root *html.Node, class string) string {
	if n := firstClass(root, class); n != nil {
		return text(n)
	}
	return ""
}

// parseShowing reads "Showing 1-20 of 557 results".
//
// The numbers carry thousands separators once a query is broad enough, so an
// unfiltered search prints "Showing 1-20 of 1,334,344 results" and a reader
// that stopped at the first comma would report 1 result out of 334.
func parseShowing(s string) (first, last, total int) {
	s = collapse(s)
	if s == "" {
		return 0, 0, 0
	}
	var nums []int
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			if n, err := strconv.Atoi(cur.String()); err == nil {
				nums = append(nums, n)
			}
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] >= '0' && s[i] <= '9':
			cur.WriteByte(s[i])
		case s[i] == ',' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' && cur.Len() > 0:
			// A separator inside a number, not the end of one.
		default:
			flush()
		}
	}
	flush()
	switch len(nums) {
	case 0:
		return 0, 0, 0
	case 1:
		return 0, 0, nums[0]
	case 2:
		return nums[0], nums[1], 0
	default:
		return nums[0], nums[1], nums[2]
	}
}

// containerKind and containerID read the card's parent link, which points at a
// journal or at a book and says which in its own path.
func containerKind(href string) string {
	switch {
	case strings.Contains(href, "/journal/"):
		return "journal"
	case strings.Contains(href, "/book/"):
		return "book"
	case strings.Contains(href, "/series/"):
		return "series"
	}
	return ""
}

func containerID(href string) string {
	href = strings.TrimSuffix(trimQuery(href), "/")
	for _, seg := range []string{"/journal/", "/series/"} {
		if _, id, ok := strings.Cut(href, seg); ok {
			return id
		}
	}
	if _, id, ok := strings.Cut(href, "/book/"); ok {
		return id
	}
	return ""
}
