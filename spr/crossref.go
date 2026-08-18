package spr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Crossref, the DOI registration agency.
//
// What it holds is what the publisher deposited, which makes it the best source
// for a reference list and a poor source for anything the publisher had no
// reason to send. The measured article deposited 122 references, exactly the
// number the page prints, and 66 of those carry a DOI. The other 56 are a
// citation string and nothing resolvable, which is a fact about the deposit and
// not a gap in this reader.
//
// The envelope tier for everything in this file is "crossref", and every field
// records the JSON path it came out of, so a record can be audited against the
// same url a person can paste into a browser.

// CrossrefBase is the works endpoint.
const CrossrefBase = "https://" + CrossrefHost + "/works"

// CrossrefWork is one record from /works/{doi}.
type CrossrefWork struct {
	DOI  DOI    `json:"doi,omitempty"`
	URL  string `json:"url,omitempty"`
	Type string `json:"type,omitempty"`

	// Title and ContainerTitle are arrays in the source and one string here.
	// Crossref allows several of each for a translated title or a journal that
	// changed name, and every Springer record measured carried exactly one.
	// Extra entries are kept in Titles rather than dropped.
	Title          string   `json:"title,omitempty"`
	Titles         []string `json:"titles,omitempty"`
	ContainerTitle string   `json:"container_title,omitempty"`
	ShortContainer string   `json:"container_short,omitempty"`

	Publisher string `json:"publisher,omitempty"`
	Volume    string `json:"volume,omitempty"`
	Issue     string `json:"issue,omitempty"`
	Pages     string `json:"pages,omitempty"`
	Language  string `json:"language,omitempty"`

	// ISSNs are typed, because a journal has a print and an electronic one and
	// the untyped issn array does not say which is which. Machine Learning
	// deposits 0885-6125 as print and 1573-0565 as electronic.
	ISSNs []TypedISSN `json:"issns,omitempty"`
	ISBNs []string    `json:"isbns,omitempty"`

	// Abstract is the JATS markup stripped to text. The spec expected Crossref
	// to carry no abstract for most Springer works, and this one carries 1,061
	// characters of it, so the field is read rather than assumed empty.
	Abstract string `json:"abstract,omitempty"`

	// Four date fields, kept apart because they are four different claims. The
	// measured article says issued 2021-03, published 2021-03, published-print
	// 2021-03 and published-online 2021-03-08, and only the last one is a day.
	// Collapsing them would invent a day for three of them or lose one from the
	// fourth.
	Issued          *Date `json:"issued,omitempty"`
	Published       *Date `json:"published,omitempty"`
	PublishedOnline *Date `json:"published_online,omitempty"`
	PublishedPrint  *Date `json:"published_print,omitempty"`
	Deposited       *Date `json:"deposited,omitempty"`

	Authors []CrossrefPerson `json:"authors,omitempty"`
	Editors []CrossrefPerson `json:"editors,omitempty"`

	// Funders is what the publisher deposited about funding, which on the
	// measured article is one name and no DOI at all: {"name": "Projekt DEAL"}.
	// A funder with no id cannot be joined to anything, and saying so is more
	// use than an empty id field.
	Funders []CrossrefFunder `json:"funders,omitempty"`

	Licenses []CrossrefLicense `json:"licenses,omitempty"`
	Links    []CrossrefLink    `json:"links,omitempty"`

	// Subjects is present and empty on every Springer record measured.
	Subjects []string `json:"subjects,omitempty"`

	Counts   CrossrefCounts `json:"counts"`
	Envelope Envelope       `json:"envelope"`
}

// CrossrefCounts is what Crossref counts, under names that say Crossref.
//
// There is no field called citations anywhere in this package and there will
// not be one. The /metrics page says 1,906 and attributes it to Dimensions,
// Crossref says 1,553 and OpenAlex says 1,563, all three for the same DOI on
// the same day, and all three are right about different corpora.
type CrossrefCounts struct {
	// Citations is is-referenced-by-count: deposited citations Crossref knows
	// about, which is only what other publishers deposited with references
	// turned on.
	Citations int `json:"crossref_citations"`

	// References is reference-count, the size of the deposited reference list.
	References int `json:"crossref_references"`

	// ReferencesWithDOI is how many of those resolve to something. On the
	// measured article it is 66 of 122, and the gap is the reason this is a
	// separate number rather than a percentage in a comment.
	ReferencesWithDOI int `json:"crossref_references_with_doi"`
}

// TypedISSN is one ISSN and the medium it belongs to.
type TypedISSN struct {
	Value ISSN `json:"value"`

	// Type is Crossref's own word, "print" or "electronic".
	Type string `json:"type,omitempty"`
}

// CrossrefPerson is an author or an editor as deposited.
//
// Affiliation is an empty array on the measured article, on every one of its
// authors, while OpenAlex has a ROR id and a raw affiliation string for the
// same people. That difference is the reason both backends exist here.
type CrossrefPerson struct {
	Given  string `json:"given,omitempty"`
	Family string `json:"family,omitempty"`
	Name   string `json:"name,omitempty"`

	ORCID ORCID `json:"orcid,omitempty"`

	// ORCIDAuthenticated is Crossref's authenticated-orcid flag, which says
	// whether the person signed in to attach the id or the publisher typed it.
	// It was false on the measured article, and a tool that dropped the flag
	// would be presenting a typed in id as a verified one.
	ORCIDAuthenticated bool `json:"orcid_authenticated,omitempty"`

	// Sequence is "first" or "additional", which is the deposit's own word for
	// author order rather than this reader's array index.
	Sequence string `json:"sequence,omitempty"`

	Affiliations []string `json:"affiliations,omitempty"`
}

// Display is the name as a person would write it.
func (p CrossrefPerson) Display() string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Given != "" && p.Family != "":
		return p.Given + " " + p.Family
	case p.Family != "":
		return p.Family
	}
	return p.Given
}

// CrossrefFunder is one funding body, with whatever identifies it.
type CrossrefFunder struct {
	Name string `json:"name,omitempty"`

	// DOI is the Funder Registry id, absent on the measured article. Awards is
	// the grant numbers, also absent there.
	DOI    string   `json:"doi,omitempty"`
	Awards []string `json:"awards,omitempty"`
}

// CrossrefLicense is one licence and the version of the work it covers.
//
// The measured article deposits two under the same url, and they differ only by
// content-version and delay: tdm from day zero, vor after seven days.
// Deduplicating them on the url would leave one licence claiming to cover both
// versions from whichever date survived.
type CrossrefLicense struct {
	URL         string `json:"url,omitempty"`
	Version     string `json:"content_version,omitempty"`
	DelayInDays int    `json:"delay_in_days,omitempty"`
	Start       *Date  `json:"start,omitempty"`
}

// CrossrefLink is a machine readable link, which is a url and what it is for.
// The measured article deposits three: the pdf and the html for text mining,
// and the pdf again for similarity checking. The application is the reason they
// are not one list of two urls.
type CrossrefLink struct {
	URL         string `json:"url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Version     string `json:"content_version,omitempty"`
	Application string `json:"intended_application,omitempty"`
}

// crossrefEnvelope is the shape every Crossref answer arrives in.
type crossrefEnvelope struct {
	Status         string          `json:"status"`
	MessageType    string          `json:"message-type"`
	MessageVersion string          `json:"message-version"`
	Message        json.RawMessage `json:"message"`
}

// crossrefItem is the wire form of one work. It is separate from CrossrefWork
// because the wire form has date-parts arrays, typed and untyped issn lists and
// a title that is an array, and a record a consumer reads should have none of
// that.
type crossrefItem struct {
	DOI            string   `json:"DOI"`
	URL            string   `json:"URL"`
	Type           string   `json:"type"`
	Title          []string `json:"title"`
	ContainerTitle []string `json:"container-title"`
	ShortContainer []string `json:"short-container-title"`
	Publisher      string   `json:"publisher"`
	Volume         string   `json:"volume"`
	Issue          string   `json:"issue"`
	Page           string   `json:"page"`
	Language       string   `json:"language"`
	Abstract       string   `json:"abstract"`
	Subject        []string `json:"subject"`
	ISBN           []string `json:"ISBN"`

	ISSNType []struct {
		Value string `json:"value"`
		Type  string `json:"type"`
	} `json:"issn-type"`

	Issued          crossrefDate `json:"issued"`
	Published       crossrefDate `json:"published"`
	PublishedOnline crossrefDate `json:"published-online"`
	PublishedPrint  crossrefDate `json:"published-print"`
	Deposited       crossrefDate `json:"deposited"`

	Author []crossrefPerson `json:"author"`
	Editor []crossrefPerson `json:"editor"`

	Funder []struct {
		Name  string   `json:"name"`
		DOI   string   `json:"DOI"`
		Award []string `json:"award"`
	} `json:"funder"`

	License []struct {
		URL            string       `json:"URL"`
		ContentVersion string       `json:"content-version"`
		DelayInDays    int          `json:"delay-in-days"`
		Start          crossrefDate `json:"start"`
	} `json:"license"`

	Link []struct {
		URL                 string `json:"URL"`
		ContentType         string `json:"content-type"`
		ContentVersion      string `json:"content-version"`
		IntendedApplication string `json:"intended-application"`
	} `json:"link"`

	Reference []struct {
		DOI string `json:"DOI"`
		Key string `json:"key"`
	} `json:"reference"`

	IsReferencedByCount int `json:"is-referenced-by-count"`
	ReferenceCount      int `json:"reference-count"`
}

type crossrefPerson struct {
	Given              string `json:"given"`
	Family             string `json:"family"`
	Name               string `json:"name"`
	ORCID              string `json:"ORCID"`
	AuthenticatedORCID bool   `json:"authenticated-orcid"`
	Sequence           string `json:"sequence"`
	Affiliation        []struct {
		Name string `json:"name"`
	} `json:"affiliation"`
}

// crossrefDate is Crossref's date-parts form, where the length of the inner
// array is the precision. [[2021,3]] is a month and [[2021,3,8]] is a day, and
// this is the one place in the whole API where the shape carries the meaning.
type crossrefDate struct {
	DateParts [][]int `json:"date-parts"`
}

// date turns date-parts into a Date, or returns nil when nothing was stated.
// Crossref writes an unstated date as [[null]], which decodes to one empty
// inner array, so a length check on the outer array alone is not enough.
func (d crossrefDate) date() *Date {
	if len(d.DateParts) == 0 || len(d.DateParts[0]) == 0 {
		return nil
	}
	p := d.DateParts[0]
	out := Date{Precision: PrecisionYear}
	year, month, day := p[0], 1, 1
	if len(p) > 1 && p[1] > 0 {
		month, out.Precision = p[1], PrecisionMonth
	}
	if len(p) > 2 && p[2] > 0 {
		day, out.Precision = p[2], PrecisionDay
	}
	if year <= 0 {
		return nil
	}
	out.Value = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	out.Raw = out.String()
	return &out
}

// CrossrefWork fetches one work by DOI.
func (c *Client) CrossrefWork(ctx context.Context, d DOI) (*CrossrefWork, error) {
	// The DOI goes in the path with no escaping. Crossref's own examples do the
	// same, a DOI is defined to be url safe after the prefix, and url.PathEscape
	// would turn the slash between prefix and suffix into %2F, which this
	// endpoint answers 404 for.
	target := CrossrefBase + "/" + strings.TrimSpace(string(d))

	var env crossrefEnvelope
	resp, err := c.getJSON(ctx, target, &env)
	if err != nil {
		return nil, err
	}
	var item crossrefItem
	if err := json.Unmarshal(env.Message, &item); err != nil {
		return nil, fmt.Errorf("%s: decoding the message body: %w", target, err)
	}
	w := crossrefRecord(&item)
	w.Envelope.Tier = "crossref"
	w.Envelope.record(resp)
	crossrefProvenance(&w.Envelope, &item)
	w.Envelope.sortMissed()
	return w, nil
}

// crossrefRecord turns the wire form into the record.
func crossrefRecord(it *crossrefItem) *CrossrefWork {
	w := &CrossrefWork{
		DOI:             DOI(it.DOI),
		URL:             it.URL,
		Type:            it.Type,
		Publisher:       it.Publisher,
		Volume:          it.Volume,
		Issue:           it.Issue,
		Pages:           it.Page,
		Language:        it.Language,
		Abstract:        jats(it.Abstract),
		Subjects:        it.Subject,
		ISBNs:           it.ISBN,
		Issued:          it.Issued.date(),
		Published:       it.Published.date(),
		PublishedOnline: it.PublishedOnline.date(),
		PublishedPrint:  it.PublishedPrint.date(),
		Deposited:       it.Deposited.date(),
		Counts: CrossrefCounts{
			Citations:  it.IsReferencedByCount,
			References: it.ReferenceCount,
		},
	}
	if len(it.Title) > 0 {
		w.Title = it.Title[0]
	}
	if len(it.Title) > 1 {
		w.Titles = it.Title
	}
	if len(it.ContainerTitle) > 0 {
		w.ContainerTitle = it.ContainerTitle[0]
	}
	if len(it.ShortContainer) > 0 {
		w.ShortContainer = it.ShortContainer[0]
	}
	for _, is := range it.ISSNType {
		parsed, err := ParseISSN(is.Value)
		if err != nil {
			continue
		}
		w.ISSNs = append(w.ISSNs, TypedISSN{Value: parsed, Type: is.Type})
	}
	w.Authors = crossrefPeople(it.Author)
	w.Editors = crossrefPeople(it.Editor)
	for _, f := range it.Funder {
		w.Funders = append(w.Funders, CrossrefFunder{Name: f.Name, DOI: f.DOI, Awards: f.Award})
	}
	for _, l := range it.License {
		w.Licenses = append(w.Licenses, CrossrefLicense{
			URL:         l.URL,
			Version:     l.ContentVersion,
			DelayInDays: l.DelayInDays,
			Start:       l.Start.date(),
		})
	}
	for _, l := range it.Link {
		w.Links = append(w.Links, CrossrefLink{
			URL:         l.URL,
			ContentType: l.ContentType,
			Version:     l.ContentVersion,
			Application: l.IntendedApplication,
		})
	}
	for _, r := range it.Reference {
		if r.DOI != "" {
			w.Counts.ReferencesWithDOI++
		}
	}
	return w
}

func crossrefPeople(in []crossrefPerson) []CrossrefPerson {
	var out []CrossrefPerson
	for _, p := range in {
		person := CrossrefPerson{
			Given:              p.Given,
			Family:             p.Family,
			Name:               p.Name,
			ORCIDAuthenticated: p.AuthenticatedORCID,
			Sequence:           p.Sequence,
		}
		if id, err := ParseORCID(p.ORCID); err == nil {
			person.ORCID = id
		}
		for _, a := range p.Affiliation {
			if a.Name != "" {
				person.Affiliations = append(person.Affiliations, a.Name)
			}
		}
		out = append(out, person)
	}
	return out
}

// CrossrefReferences returns the DOIs in a work's deposited reference list.
//
// This is the citing direction and it comes from the publisher's own deposit,
// which is why it is here and not on OpenAlex. The count of entries that carry
// no DOI is returned alongside, because a list of 66 that came from 122 is a
// different thing from a complete list of 66.
func (c *Client) CrossrefReferences(ctx context.Context, d DOI) (dois []DOI, unresolved int, err error) {
	target := CrossrefBase + "/" + strings.TrimSpace(string(d))
	var env crossrefEnvelope
	if _, err := c.getJSON(ctx, target, &env); err != nil {
		return nil, 0, err
	}
	var item crossrefItem
	if err := json.Unmarshal(env.Message, &item); err != nil {
		return nil, 0, fmt.Errorf("%s: decoding the message body: %w", target, err)
	}
	seen := map[DOI]bool{}
	for _, r := range item.Reference {
		parsed, err := ParseDOI(r.DOI)
		if err != nil {
			unresolved++
			continue
		}
		if seen[parsed] {
			continue
		}
		seen[parsed] = true
		dois = append(dois, parsed)
	}
	return dois, unresolved, nil
}

// CrossrefQuery is a search of the works endpoint.
//
// Every field here was measured against the live API. The filters go into one
// comma separated filter parameter and the free text goes into query.* fields,
// which is Crossref's own split and not this tool's.
type CrossrefQuery struct {
	// Bibliographic is the catch all free text field, which matches title,
	// container, author and year together. Title and Author are the narrower
	// ones, and all three can be combined.
	Bibliographic string
	Title         string
	Author        string

	ISSN   ISSN
	ISBN   string
	Type   string
	Funder string

	// From and Until filter on the publication date, at whatever precision the
	// caller states. Crossref wants a full date and accepts a year or a year
	// and month, filling the rest in itself.
	From  string
	Until string

	// Rows is the page size, capped by Crossref at 1,000. Cursor is the deep
	// paging token, and "*" asks for the first page of one.
	Rows   int
	Cursor string

	// Select trims the fields returned, which on a 42 KB record is the
	// difference between a listing that costs 3 MB and one that costs 40 KB.
	Select []string

	// Facet asks for counts by group, as "type-name:5" or "publisher-name:3".
	Facet []string

	// Sort and Order are Crossref's own vocabulary, "relevance", "published",
	// "is-referenced-by-count", with "asc" or "desc".
	Sort  string
	Order string
}

// CrossrefResult is one page of a query.
type CrossrefResult struct {
	// Total is total-results, which is the size of the whole match and not of
	// this page.
	Total int `json:"total"`

	Items []CrossrefWork `json:"items"`

	// NextCursor is present only when the query asked for a cursor. Crossref
	// keeps returning one past the end of the result set, so a caller loops
	// until a page comes back empty rather than until the cursor is absent.
	NextCursor string `json:"next_cursor,omitempty"`

	// Facets are the group counts, when any were asked for.
	Facets []CrossrefFacet `json:"facets,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// CrossrefFacet is one group of counts.
type CrossrefFacet struct {
	Name string `json:"name"`

	// ValueCount is how many distinct values the group has in total, which is
	// usually more than the number returned.
	ValueCount int `json:"value_count"`

	Values []FacetValue `json:"values"`
}

// FacetValue is one label and its count.
type FacetValue struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// URL builds the request url for a query.
func (q CrossrefQuery) URL() string {
	v := url.Values{}
	set := func(k, s string) {
		if strings.TrimSpace(s) != "" {
			v.Set(k, s)
		}
	}
	set("query.bibliographic", q.Bibliographic)
	set("query.title", q.Title)
	set("query.author", q.Author)

	var filters []string
	add := func(k, s string) {
		if strings.TrimSpace(s) != "" {
			filters = append(filters, k+":"+s)
		}
	}
	add("issn", string(q.ISSN))
	add("isbn", q.ISBN)
	add("type", q.Type)
	add("funder", strings.TrimPrefix(strings.TrimSpace(q.Funder), "https://doi.org/"))
	add("from-pub-date", q.From)
	add("until-pub-date", q.Until)
	if len(filters) > 0 {
		v.Set("filter", strings.Join(filters, ","))
	}

	if q.Rows > 0 {
		v.Set("rows", strconv.Itoa(q.Rows))
	}
	set("cursor", q.Cursor)
	set("sort", q.Sort)
	set("order", q.Order)
	if len(q.Select) > 0 {
		v.Set("select", strings.Join(q.Select, ","))
	}
	if len(q.Facet) > 0 {
		v.Set("facet", strings.Join(q.Facet, ","))
	}
	return CrossrefBase + "?" + v.Encode()
}

// crossrefPage is the wire form of a query answer.
//
// Facets is a raw message rather than a typed map so that a change in its shape
// costs the facet counts and not the whole page. It is an object keyed by group
// name on every response measured, empty when no facet parameter was sent, and
// it is the one block here whose shape is set by a request parameter rather
// than by the record.
type crossrefPage struct {
	TotalResults int             `json:"total-results"`
	ItemsPerPage int             `json:"items-per-page"`
	NextCursor   string          `json:"next-cursor"`
	Items        []crossrefItem  `json:"items"`
	Facets       json.RawMessage `json:"facets"`
}

// CrossrefSearch runs one query and returns one page.
func (c *Client) CrossrefSearch(ctx context.Context, q CrossrefQuery) (*CrossrefResult, error) {
	target := q.URL()
	var env crossrefEnvelope
	resp, err := c.getJSON(ctx, target, &env)
	if err != nil {
		return nil, err
	}
	var page crossrefPage
	if err := json.Unmarshal(env.Message, &page); err != nil {
		return nil, fmt.Errorf("%s: decoding the message body: %w", target, err)
	}

	out := &CrossrefResult{Total: page.TotalResults, NextCursor: page.NextCursor}
	for i := range page.Items {
		w := crossrefRecord(&page.Items[i])
		w.Envelope.Tier = "crossref"
		out.Items = append(out.Items, *w)
	}
	out.Facets = crossrefFacets(page.Facets)
	out.Envelope.Tier = "crossref"
	out.Envelope.record(resp)
	out.Envelope.carry("total", "crossref:message.total-results")
	if len(out.Items) > 0 {
		out.Envelope.carry("items", "crossref:message.items[]")
	}
	if len(out.Facets) > 0 {
		out.Envelope.carry("facets", "crossref:message.facets")
	}
	return out, nil
}

// crossrefFacets decodes the facet block, which is an object when it is there
// and an empty array when it is not.
func crossrefFacets(raw json.RawMessage) []CrossrefFacet {
	if len(raw) == 0 {
		return nil
	}
	var groups map[string]struct {
		ValueCount int            `json:"value-count"`
		Values     map[string]int `json:"values"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil
	}
	var out []CrossrefFacet
	for name, g := range groups {
		f := CrossrefFacet{Name: name, ValueCount: g.ValueCount}
		for label, count := range g.Values {
			f.Values = append(f.Values, FacetValue{Label: label, Count: count})
		}
		// The values arrive as a map, so an order has to be chosen. Count
		// first, then label, which is the order the site's own facet panel
		// prints and the order a reader expects.
		sort.Slice(f.Values, func(i, j int) bool {
			if f.Values[i].Count != f.Values[j].Count {
				return f.Values[i].Count > f.Values[j].Count
			}
			return f.Values[i].Label < f.Values[j].Label
		})
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// crossrefProvenance writes where each field came from, and why the ones that
// are empty are empty. The paths are the JSON keys as Crossref spells them,
// capitals and all, so that a record can be checked against the raw answer.
func crossrefProvenance(e *Envelope, it *crossrefItem) {
	say := func(field, path string) { e.carry(field, "crossref:"+path) }

	if it.DOI != "" {
		say("doi", "DOI")
	}
	if len(it.Title) > 0 {
		say("title", "title[0]")
	}
	if len(it.ContainerTitle) > 0 {
		say("container_title", "container-title[0]")
	}
	if len(it.ISSNType) > 0 {
		say("issns", "issn-type[]")
	}
	if it.Abstract != "" {
		say("abstract", "abstract, jats markup stripped")
	} else {
		e.miss("abstract", "not deposited, which is the common case for this publisher and not an error")
	}
	if len(it.Author) > 0 {
		say("authors", "author[]")
		affiliated := 0
		for _, a := range it.Author {
			if len(a.Affiliation) > 0 {
				affiliated++
			}
		}
		if affiliated == 0 {
			e.miss("authors.affiliations", fmt.Sprintf("all %d authors deposited an empty affiliation array, and OpenAlex has institutions for the same work", len(it.Author)))
		}
	}
	if len(it.Funder) > 0 {
		say("funders", "funder[]")
		withID := 0
		for _, f := range it.Funder {
			if f.DOI != "" {
				withID++
			}
		}
		if withID == 0 {
			e.miss("funders.doi", "the funders were deposited by name with no Funder Registry id, so they join to nothing")
		}
	}
	say("counts.crossref_citations", "is-referenced-by-count")
	say("counts.crossref_references", "reference-count")

	withDOI := 0
	for _, r := range it.Reference {
		if r.DOI != "" {
			withDOI++
		}
	}
	if len(it.Reference) > 0 {
		say("counts.crossref_references_with_doi", "reference[].DOI")
		if withDOI < len(it.Reference) {
			e.miss("references.doi", fmt.Sprintf("%d of %d deposited references carry no DOI and are a citation string only", len(it.Reference)-withDOI, len(it.Reference)))
		}
	} else if it.ReferenceCount > 0 {
		e.miss("references", fmt.Sprintf("reference-count says %d and the reference list is closed, which Crossref allows a publisher to choose", it.ReferenceCount))
	}
	if len(it.Subject) == 0 {
		e.miss("subjects", "the subject array is present and empty, which is every Springer record measured")
	}
}
