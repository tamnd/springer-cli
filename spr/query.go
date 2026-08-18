package spr

import (
	"net/url"
	"strconv"
	"strings"
)

// One query, two renderings.
//
// /search and /search.rss take the same parameters and answer from the same
// index, so there is one Query type and one Values method and the two paths
// differ only in the path. That is not tidiness. The four facet parameters have
// to be sent as quoted strings, quotes included, and a query with the quotes
// missing is syntactically valid, answers 200 and matches nothing. A silent
// zero is the worst failure a search can have, and the only way to be sure it
// cannot happen on one path and not the other is for there to be one place
// where the quoting happens.
//
// The page states the rule itself. Every facet checkbox carries the parameter
// name and the exact value the site would send:
//
//	name="taxonomy"              value="&quot;Machine Learning&quot;"
//	name="facet-discipline"      value="&quot;Computer Science&quot;"
//	name="content-type"          value="Article"
//	name="language"              value="En"
//
// So the quoting is read off the capture in a test rather than asserted from a
// comment, and if Springer ever drops the quotes the test says so.

// Query is a search, with every filter the form offers.
//
// The three field scoped terms come from /advanced-search, which exposes them
// as separate inputs alongside the all fields query. They are sent as ordinary
// parameters, so a query can mix them with the facets freely.
type Query struct {
	// Terms is the all fields query.
	Terms string `json:"terms,omitempty"`

	// Title, Contributor and Journal are the field scoped inputs.
	Title       string `json:"title,omitempty"`
	Contributor string `json:"contributor,omitempty"`
	Journal     string `json:"journal,omitempty"`

	// Types are content-type values, Article, Chapter and so on, sent
	// unquoted.
	Types []string `json:"types,omitempty"`

	// OpenAccess sends openAccess=true, which is the publishing model facet
	// under a different parameter name than its group.
	OpenAccess bool `json:"open_access,omitempty"`

	Languages []string `json:"languages,omitempty"`

	// The four quoted facets. The quotes are added by Values, never by the
	// caller, so --taxonomy "Machine Learning" is what a person types.
	Taxonomy       []string `json:"taxonomy,omitempty"`
	Disciplines    []string `json:"disciplines,omitempty"`
	SubDisciplines []string `json:"sub_disciplines,omitempty"`
	SDGs           []string `json:"sdgs,omitempty"`

	// From and To are years and travel with date=custom. Last is one of the
	// relative windows, m3, m6, m12 or m24, and the two forms are exclusive
	// because the form offers them as one radio group.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Last string `json:"last,omitempty"`

	// Sort is relevance, newestFirst or oldestFirst, which are the three
	// values in the page's own select. It has no effect on the feed, and
	// SortsFeed says so.
	Sort string `json:"sort,omitempty"`

	// Page is 1 based. Zero means page one and is not sent.
	Page int `json:"page,omitempty"`
}

// The four parameters whose values carry their own quotes, and the names the
// form sends them under. Three of the four are named differently from the
// facet group they belong to, which is why this is a table and not a rule.
const (
	paramTaxonomy      = "taxonomy"
	paramDiscipline    = "facet-discipline"
	paramSubDiscipline = "facet-sub-discipline"
	paramSDG           = "sustainableDevelopmentGoal"
)

// Values renders the query as the site's own parameters.
//
// Empty fields are left out entirely rather than sent empty, because an empty
// content-type is not the same request as no content-type and only one of them
// is what the caller asked for.
func (q Query) Values() url.Values {
	v := url.Values{}
	add := func(name, value string) {
		if value = strings.TrimSpace(value); value != "" {
			v.Set(name, value)
		}
	}
	add("query", q.Terms)
	add("title", q.Title)
	add("contributor", q.Contributor)
	add("journal", q.Journal)

	for _, t := range q.Types {
		if t = strings.TrimSpace(t); t != "" {
			v.Add("content-type", contentType(t))
		}
	}
	if q.OpenAccess {
		v.Set("openAccess", "true")
	}
	for _, l := range q.Languages {
		if l = strings.TrimSpace(l); l != "" {
			v.Add("language", l)
		}
	}
	addQuoted(v, paramTaxonomy, q.Taxonomy)
	addQuoted(v, paramDiscipline, q.Disciplines)
	addQuoted(v, paramSubDiscipline, q.SubDisciplines)
	addQuoted(v, paramSDG, q.SDGs)

	// A relative window and a custom range are one radio group on the form, so
	// sending both would be sending a radio two values. The explicit range
	// wins, because somebody who typed two years meant them.
	switch {
	case q.From != "" || q.To != "":
		v.Set("date", "custom")
		add("dateFrom", q.From)
		add("dateTo", q.To)
	case q.Last != "":
		v.Set("date", q.Last)
	}

	add("sortBy", sortValue(q.Sort))
	if q.Page > 1 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	return v
}

// addQuoted adds the values of one facet with the double quotes the site's own
// form emits around them.
//
// A value that already carries its quotes is left alone, so that a value copied
// straight out of a facet listing works as typed.
func addQuoted(v url.Values, name string, values []string) {
	for _, s := range values {
		if s = strings.TrimSpace(s); s != "" {
			v.Add(name, quoteFacet(s))
		}
	}
}

// quoteFacet puts a facet value in the quotes the site expects.
func quoteFacet(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) > 1 {
		return s
	}
	return `"` + s + `"`
}

// The sixteen content types, as the value the form sends and the label the
// page prints beside it.
//
// The two are not the same string and the difference is not cosmetic. The
// facet labelled "Research article" sends content-type=Research, "Review
// article" sends Review, and "Reference work entry" sends Reference Work
// Entry with three capitals. A tool that sent the label back would filter to
// nothing on three of the sixteen and would look like it worked on the other
// thirteen, which is the worst possible distribution of a bug.
//
// Read off an unfiltered /search, where all sixteen appear. The capture in
// testdata is a filtered query and shows only four of them, so this list is
// wider than the test that checks it.
var contentTypes = []struct{ value, label string }{
	{"Article", "Article"},
	{"Research", "Research article"},
	{"Chapter", "Chapter"},
	{"Conference Paper", "Conference paper"},
	{"Review", "Review article"},
	{"Reference Work Entry", "Reference work entry"},
	{"News", "News article"},
	{"Book", "Book"},
	{"Protocol", "Protocol"},
	{"Conference Proceedings", "Conference proceedings"},
	{"Collection", "Collection"},
	{"Call For Papers", "Call for papers"},
	{"Textbook", "Textbook"},
	{"Book Series", "Book series"},
	{"Journal", "Journal"},
	{"Video Segment", "Video segment"},
}

// contentTypeIndex accepts the value, the label, or the label without its
// trailing word, so that --type article, --type "research article" and --type
// research all reach the same filter.
var contentTypeIndex = func() map[string]string {
	m := map[string]string{}
	for _, t := range contentTypes {
		m[strings.ToLower(t.value)] = t.value
		m[strings.ToLower(t.label)] = t.value
	}
	// The two spellings a person is most likely to reach for, neither of which
	// is on the page.
	m["entry"] = "Reference Work Entry"
	m["proceedings"] = "Conference Proceedings"
	return m
}()

// contentType maps what a person typed to what the form sends, and passes
// anything unrecognized through unchanged so that a type Springer adds
// tomorrow is usable today.
func contentType(s string) string {
	if v, ok := contentTypeIndex[strings.ToLower(strings.TrimSpace(s))]; ok {
		return v
	}
	return s
}

// The three sort orders, read off the page's own select, plus the words a
// person would reach for.
//
// The select offers relevance, newestFirst and oldestFirst. Nobody types
// newestFirst, so --sort date and --sort newest arrive here and leave as the
// value the form posts.
var sorts = map[string]string{
	"relevance":   "relevance",
	"date":        "newestFirst",
	"newest":      "newestFirst",
	"newestfirst": "newestFirst",
	"oldest":      "oldestFirst",
	"oldestfirst": "oldestFirst",
}

func sortValue(s string) string {
	if v, ok := sorts[strings.ToLower(strings.TrimSpace(s))]; ok {
		return v
	}
	return s
}

// The two paths the same query is served under.
const (
	searchPath = "/search"
	feedPath   = "/search.rss"
)

// SearchURL is the html rendering, which is the only one with facets and a
// total.
func (q Query) SearchURL() string { return Base + q.path(searchPath) }

// FeedURL is the rss rendering, which is the one that keeps answering.
func (q Query) FeedURL() string { return Base + q.path(feedPath) }

// path is the site relative form, which is what the runner asks the client for
// so that the host stays the client's business.
func (q Query) path(p string) string { return p + "?" + q.Values().Encode() }

// At returns the query for one page of results.
func (q Query) At(page int) Query {
	q.Page = page
	return q
}

// SortsFeed reports whether the requested sort will actually be honoured by the
// feed, which it will not.
//
// Measured with sortBy absent, sortBy=relevance and sortBy=newestFirst against
// the same query in the same minute: twenty items, identical order, first guid
// 10.1007/s00193-024-01208-y and first pubDate 2024-12-28 in all three. The
// feed is always newest first. The html honours the parameter.
//
// So a run that falls back to rss with --sort relevance is returning results in
// an order nobody asked for, and this is what lets the command say so instead
// of quietly handing over a differently sorted list.
func (q Query) SortsFeed() bool {
	v := sortValue(q.Sort)
	return v == "" || v == "newestFirst"
}

// FeedPageSize is the number of items one feed page carries.
//
// Fixed at 20 by the site. size, results, per-page and limit were each tried
// and each returned 20 items, so there is no parameter to expose and a caller
// asking for 500 results is asking for 25 requests.
const FeedPageSize = 20
