package spr

import (
	"context"
	"fmt"
	"strings"
)

// spr search --also crossref --also openalex.
//
// A search of link.springer.com returns what Springer publishes. An open index
// asked the same question returns what everybody publishes, and the difference
// between the two sets is a fact about the query that neither set shows on its
// own. So the results are merged rather than replaced, every result says which
// backends answered for it, and the counts each backend reported go to stderr
// where they cannot be mistaken for part of the result set.
//
// The join is on the normalized DOI and on nothing else. Titles vary in
// punctuation and case between the three sources, positions are meaningless
// across corpora that were sorted differently, and a merge on either would
// silently fuse two different works. A result with no DOI joins to nothing and
// stays where it is, which is the correct answer rather than a limitation.

// Also is one open index a search can be widened with.
type Also string

const (
	AlsoCrossref Also = "crossref"
	AlsoOpenAlex Also = "openalex"
)

// AlsoNames are the backends --also accepts, for the flag help and the error.
var AlsoNames = []string{string(AlsoCrossref), string(AlsoOpenAlex)}

// ParseAlso reads a backend name.
func ParseAlso(s string) (Also, error) {
	switch Also(strings.ToLower(strings.TrimSpace(s))) {
	case AlsoCrossref:
		return AlsoCrossref, nil
	case AlsoOpenAlex:
		return AlsoOpenAlex, nil
	}
	return "", fmt.Errorf("--also %q is not one of %s", s, strings.Join(AlsoNames, ", "))
}

// Widen asks the named backends the same query and merges their answers into a
// search that has already run.
//
// A backend that fails is reported through note and does not fail the run. The
// point of asking three hosts is that two of them answering is still worth
// having, and a Crossref outage is not a reason to throw away results that
// link.springer.com already returned.
func (c *Client) Widen(ctx context.Context, res *SearchResponse, backends []Also, limit int, note func(string)) {
	if res == nil || len(backends) == 0 {
		return
	}
	if note == nil {
		note = func(string) {}
	}
	if len(res.Query.Types) > 0 {
		// Springer's content-type values are its own words and Crossref and
		// OpenAlex each have a different vocabulary for the same distinction.
		// Mapping between them was not measured, so the filter is dropped and
		// said out loud rather than guessed at and applied quietly.
		note("--type is a Springer content type and is not sent to the open indexes, so their results are unfiltered by type")
	}

	for _, b := range backends {
		items, total, err := c.alsoSearch(ctx, b, res.Query, limit)
		if err != nil {
			note(fmt.Sprintf("%s did not answer, and the results below are the ones that did: %v", b, err))
			continue
		}
		msg := mergeAlso(res, b, items, total)
		note(msg)
		res.Notes = append(res.Notes, msg)
	}
}

// mergeAlso folds one backend's answers into a result set and returns the
// sentence describing what it did.
//
// It is separate from Widen because everything worth arguing about is here and
// none of it needs a network: which key the join is on, what happens to a
// result with no key, and which fields a backend is allowed to fill in.
func mergeAlso(res *SearchResponse, b Also, items []SearchResult, total int) string {
	// The index the merge is done through. A Springer result with no DOI is not
	// in it, because there is nothing to key it on.
	at := map[DOI]int{}
	for i, r := range res.Results {
		if d, err := ParseDOI(r.DOI); err == nil {
			at[d] = i
		}
	}

	var held, added, nodoi int
	for _, it := range items {
		d, err := ParseDOI(it.DOI)
		if err != nil {
			nodoi++
			continue
		}
		if i, ok := at[d]; ok {
			held++
			res.Results[i].Via = addVia(res.Results[i].Via, string(b))
			fillFrom(&res.Results[i], it, &res.Envelope, b)
			continue
		}
		added++
		it.Position = len(res.Results) + 1
		at[d] = len(res.Results)
		res.Results = append(res.Results, it)
	}

	res.Paths = append(res.Paths, string(b))
	res.Envelope.carry("results[]."+string(b), string(b)+":works")

	msg := fmt.Sprintf("%s matched %d and returned %d, %d already in the Springer results and %d new",
		b, total, len(items), held, added)
	if nodoi > 0 {
		msg += fmt.Sprintf(", and %d with no doi to merge on", nodoi)
	}
	return msg
}

// alsoSearch runs one query against one backend and returns its answers in the
// shape a search result set is already in.
func (c *Client) alsoSearch(ctx context.Context, b Also, q Query, limit int) ([]SearchResult, int, error) {
	switch b {
	case AlsoCrossref:
		got, err := c.CrossrefSearch(ctx, CrossrefQuery{
			Bibliographic: q.Terms,
			Title:         q.Title,
			Author:        q.Contributor,
			From:          fullDate(q.From, false),
			Until:         fullDate(q.To, true),
			Rows:          limit,
		})
		if err != nil {
			return nil, 0, err
		}
		out := make([]SearchResult, 0, len(got.Items))
		for _, w := range got.Items {
			out = append(out, crossrefResult(w))
		}
		return out, got.Total, nil

	case AlsoOpenAlex:
		perPage := limit
		if perPage > OpenAlexPageSize {
			perPage = OpenAlexPageSize
		}
		got, err := c.OpenAlexSearch(ctx, OpenAlexQuery{
			Search:  q.Terms,
			Title:   q.Title,
			Author:  q.Contributor,
			From:    fullDate(q.From, false),
			Until:   fullDate(q.To, true),
			PerPage: perPage,
		})
		if err != nil {
			return nil, 0, err
		}
		out := make([]SearchResult, 0, len(got.Items))
		for _, w := range got.Items {
			out = append(out, openAlexResult(w))
		}
		return out, got.Total, nil
	}
	return nil, 0, fmt.Errorf("%q is not a backend this tool knows", b)
}

// fullDate widens a bare year into the full date the open indexes want.
//
// Springer's form takes years and both indexes document a full date on their
// date filters, so 2020 becomes 2020-01-01 as a lower bound and 2024 becomes
// 2024-12-31 as an upper one. Sending the year alone would either be rejected
// or read as the first of January at both ends, and the second of those loses
// eleven months of the range the caller asked for without saying so.
func fullDate(s string, end bool) string {
	s = strings.TrimSpace(s)
	if len(s) != 4 {
		return s
	}
	if end {
		return s + "-12-31"
	}
	return s + "-01-01"
}

// addVia appends a backend to a result's via string without repeating one that
// is already there, so a result answered by rss, html and Crossref reads
// rss+html+crossref.
func addVia(have, add string) string {
	if have == "" {
		return add
	}
	for _, part := range strings.Split(have, "+") {
		if part == add {
			return have
		}
	}
	return have + "+" + add
}

// fillFrom copies the fields a backend has and the Springer card did not,
// without touching a field the site already answered.
//
// Only the abstract is copied, and it is copied because the gap is real: the
// html search card carries a truncated snippet ending in an ellipsis, and
// Crossref carries 1,061 characters of JATS for the same work. Everything else
// a backend holds is either already on the card or is a different claim about
// the same thing, and overwriting the publisher's own statement with a derived
// index's version of it would make the record harder to trust rather than
// fuller.
func fillFrom(dst *SearchResult, src SearchResult, e *Envelope, b Also) {
	if dst.Abstract == "" && src.Abstract != "" {
		dst.Abstract = src.Abstract
		e.carry("results[].abstract", string(b)+":abstract")
	}
}

// crossrefResult turns one Crossref record into a search result.
//
// Issued is the date, out of the five Crossref deposits, because it is the one
// that is present on every record measured and it is the publication date the
// registration agency itself sorts on.
func crossrefResult(w CrossrefWork) SearchResult {
	r := SearchResult{
		DOI:         string(w.DOI),
		Title:       w.Title,
		Abstract:    w.Abstract,
		URL:         w.URL,
		Published:   w.Issued,
		ContentType: w.Type,
		Via:         string(AlsoCrossref),
	}
	if w.URL == "" && w.DOI != "" {
		r.URL = w.DOI.URL()
	}
	for _, p := range w.Authors {
		r.Authors = append(r.Authors, p.Display())
	}
	if w.ContainerTitle != "" {
		r.Container = &Ref{Name: w.ContainerTitle}
	}
	return r
}

// openAlexResult turns one OpenAlex record into a search result.
func openAlexResult(w OpenAlexWork) SearchResult {
	r := SearchResult{
		DOI:         string(w.DOI),
		Title:       w.Title,
		Abstract:    w.Abstract,
		ContentType: w.Type,
		Via:         string(AlsoOpenAlex),
	}
	if w.DOI != "" {
		r.URL = w.DOI.URL()
	}
	if d, err := ParseDate(w.PublicationDate); err == nil && !d.Zero() {
		r.Published = &d
	}
	for _, a := range w.Authors {
		r.Authors = append(r.Authors, a.Name)
	}
	if w.Source != nil && w.Source.DisplayName != "" {
		r.Container = &Ref{Name: w.Source.DisplayName, ID: ShortOpenAlexID(w.Source.ID)}
	}
	if w.OpenAccess != nil {
		r.OpenAccess = w.OpenAccess.IsOA
	}
	return r
}
