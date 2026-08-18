package spr

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Running a search across the two paths.
//
// The feed is primary. It pages to the end of the result set, carries the full
// abstract rather than the card's truncated one, gives a bare doi in guid, and
// it kept answering while the html surface was serving challenges. The html
// page is enrichment: it is the only source of the total, the facet counts, and
// the per result content type, container and author list.
//
// The two do not agree on what the first twenty results are. Fetched in the
// same minute for the same query, html page 1 and rss page 1 share 3 of 20
// items, because the html honours sortBy=relevance and the feed ignores it and
// answers newest first. So enrichment joins on doi and never on position, and
// a run that wants the card fields for the whole result set has to say so with
// --enrich and pay for the pages.
//
// One page of enrichment is fetched by default anyway, because the total and
// the facets come from nowhere else and one request is what they cost.

// SearchOptions are the knobs the command exposes, in the terms the package
// thinks in.
type SearchOptions struct {
	// Limit is how many results are wanted. Zero means one page.
	Limit int

	// Path forces one surface. Empty runs both.
	Path string

	// Enrich fetches enough html pages to cover the whole result set rather
	// than the one page the total and the facets need.
	Enrich bool

	// FacetsOnly skips the feed entirely and returns the counts, which is the
	// cheapest way to look at a query before running it.
	FacetsOnly bool

	// Note receives the things the caller has a right to know while the run is
	// still going, once each. It is stderr in the command.
	Note func(string)
}

// The two path names, as they appear in Paths and in a result's Via.
const (
	PathRSS  = "rss"
	PathHTML = "html"
)

// ErrChallenged is a path answering with the edge's client challenge rather
// than with results.
//
// It is a sentinel because the command has a separate exit code for it and
// matching on the wording of a message is not a thing to build an exit code on.
// A challenge is a rate rather than a refusal: it is triggered by volume, it
// does not clear by waiting a few seconds, and coming back later works.
var ErrChallenged = errors.New("the search surface served a client challenge")

// Search runs one query across both paths and returns one merged answer.
func (c *Client) Search(ctx context.Context, q Query, opts SearchOptions) (*SearchResponse, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = FeedPageSize
	}

	res := &SearchResponse{
		Query:    q,
		Page:     q.Page,
		PerPage:  FeedPageSize,
		Envelope: Envelope{Tier: "search"},
	}
	note := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		res.Notes = append(res.Notes, s)
		if opts.Note != nil {
			opts.Note(s)
		}
	}

	wantRSS := opts.Path != PathHTML && !opts.FacetsOnly
	wantHTML := opts.Path != PathRSS

	if wantRSS && !q.SortsFeed() {
		note("the feed ignores sortBy and always answers newest first, so --sort %s applies to the html pass only", q.Sort)
	}

	// The feed first, because it is the path that decides how many results
	// there are to enrich.
	var feedFailed error
	if wantRSS {
		if err := c.searchFeed(ctx, q, limit, res, note); err != nil {
			if !wantHTML {
				return nil, err
			}
			feedFailed = err
			note("the rss path failed with %q, continuing on html alone", err)
		}
	}

	if wantHTML {
		pages := 1
		if opts.Enrich {
			pages = pagesFor(len(res.Results))
		}
		if opts.FacetsOnly || feedFailed != nil {
			pages = max(1, pages)
		}
		if err := c.searchHTML(ctx, q, pages, opts, res, note); err != nil {
			if feedFailed != nil || len(res.Results) == 0 {
				return nil, err
			}
			// This is the failure this whole design is for. The html surface is
			// the one that challenges, and losing it costs the total, the
			// facets and the card fields, not the results.
			note("html enrichment failed with %q, so the total and the facets are unavailable for this run", err)
		}
	}

	if len(res.Results) > limit {
		res.Results = res.Results[:limit]
	}
	for i := range res.Results {
		res.Results[i].Position = i + 1
	}
	res.Envelope.sortMissed()
	return res, nil
}

// searchFeed pages the feed until it has enough results or the feed runs out.
func (c *Client) searchFeed(ctx context.Context, q Query, limit int, res *SearchResponse, note func(string, ...any)) error {
	page := q.Page
	for len(res.Results) < limit {
		url := q.At(page).path(feedPath)
		resp, err := c.Get(ctx, url, KindXML)
		if err != nil {
			return err
		}
		if resp.Status == StatusChallenged {
			if len(res.Results) > 0 {
				note("the feed was challenged at page %d, so this run stops at %d results", page, len(res.Results))
				break
			}
			return fmt.Errorf("%w on the first feed page, so there are no results to return", ErrChallenged)
		}
		f, err := ParseFeed(resp)
		if err != nil {
			return fmt.Errorf("%s: %w", url, err)
		}
		res.Envelope.record(resp)
		for _, m := range f.Envelope.Missed {
			res.Envelope.miss(m.Field, m.Why)
		}
		for field, v := range f.Envelope.Via {
			res.Envelope.carry(field, v)
		}

		// The only termination rule that holds. Both empty feeds answer 200,
		// and they are 186 and 190 bytes, so neither the status nor the size
		// says anything. A page with no items is the end of the result set.
		if len(f.Items) == 0 {
			break
		}
		res.Results = append(res.Results, f.Items...)
		res.Paths = addPath(res.Paths, PathRSS)

		// A short page is the last page. 557 results is 27 pages of 20 and one
		// of 17, and the 17 is how the arithmetic is known to be exact.
		if len(f.Items) < FeedPageSize {
			break
		}
		page++
	}
	return nil
}

// searchHTML fetches the enrichment pages and merges them into the results.
func (c *Client) searchHTML(ctx context.Context, q Query, pages int, opts SearchOptions, res *SearchResponse, note func(string, ...any)) error {
	// The results already held, indexed by the identifier both paths agree on.
	index := map[string]int{}
	for i, r := range res.Results {
		if k := doiKey(r.DOI); k != "" {
			index[k] = i
		}
	}

	// Whether a card the feed did not return joins the result set.
	//
	// It does when the html pass covered the same ground the feed did, which
	// means an html only run or an enriched one. It does not when the default
	// single page was fetched for the total and the facets, because that page
	// is page 1 of a different ordering and taking its leftovers would mean a
	// 500 result run carrying the stragglers of the first twenty and none of
	// the rest. Whichever way it goes, the count is said out loud.
	addExtras := opts.Enrich || opts.Path == PathHTML || len(res.Results) == 0

	var matched, added, skipped int
	for i := 0; i < pages; i++ {
		page := q.Page + i
		url := q.At(page).path(searchPath)
		resp, err := c.Get(ctx, url, KindHTML)
		if err != nil {
			return err
		}
		if resp.Status == StatusChallenged {
			return fmt.Errorf("%w for the html pass at page %d", ErrChallenged, page)
		}
		s, err := ExtractSearch(resp)
		if err != nil {
			return fmt.Errorf("%s: %w", url, err)
		}
		res.Envelope.record(resp)

		if i == 0 {
			res.Total = s.Total
			res.Facets = s.Facets
			for field, v := range s.Envelope.Via {
				res.Envelope.carry(field, v)
			}
			res.Envelope.Unread = s.Envelope.Unread
			for _, m := range s.Envelope.Missed {
				res.Envelope.miss(m.Field, m.Why)
			}
		}
		res.Paths = addPath(res.Paths, PathHTML)

		for _, card := range s.Results {
			k := doiKey(card.DOI)
			if k == "" {
				continue
			}
			if at, ok := index[k]; ok {
				merge(&res.Results[at], card)
				matched++
				continue
			}
			if opts.FacetsOnly {
				continue
			}
			// A card the feed did not return. On an html only run every card is
			// one of these. On a merged run it is the two orderings disagreeing.
			if !addExtras {
				skipped++
				continue
			}
			index[k] = len(res.Results)
			res.Results = append(res.Results, card)
			added++
		}
		if !s.HasNext {
			break
		}
	}

	if res.Total == 0 && !res.hasPath(PathHTML) {
		res.Envelope.miss("total", "no html page answered, and the feed does not carry a result count")
	}
	// The disagreement, stated in numbers rather than in a comment. On the
	// measured query one html page of 20 matched 3 of the feed's 20.
	if matched+added+skipped > 0 && res.hasPath(PathRSS) {
		if addExtras {
			note("html enrichment matched %d results by doi and added %d the feed did not return", matched, added)
		} else {
			note("html enrichment matched %d of its %d results to the feed's by doi", matched, matched+skipped)
		}
	}
	if skipped > 0 {
		note("the other %d are the two orderings disagreeing, and they are left out because only page %d of the html was read, so pass --enrich to read the rest and keep them", skipped, q.Page)
	}
	return nil
}

// merge fills the fields only the card carries, and leaves everything the feed
// said alone.
//
// The feed's abstract is the full one and the card's is a truncated snippet, so
// they go in different fields. Everything else here is html only: the feed has
// no content type, no author list, no container and no open access flag.
func merge(into *SearchResult, card SearchResult) {
	if into.Title == "" {
		into.Title = card.Title
	}
	if into.URL == "" {
		into.URL = card.URL
	}
	if into.Published == nil {
		into.Published = card.Published
	}
	if into.Snippet == "" {
		into.Snippet = card.Snippet
	}
	if into.ContentType == "" {
		into.ContentType = card.ContentType
	}
	if len(into.Authors) == 0 {
		into.Authors = card.Authors
	}
	if into.Container == nil {
		into.Container = card.Container
	}
	if into.Access == "" {
		into.Access = card.Access
	}
	into.OpenAccess = into.OpenAccess || card.OpenAccess

	// A result assembled from both paths says so. The spec has via as rss or
	// html, and a merged record that claimed either one would misattribute
	// half its fields to a path that does not carry them.
	if into.Via != "" && into.Via != card.Via {
		into.Via = ViaRSS + "+" + ViaHTML
	}
}

// pagesFor is how many html pages cover n results at the site's fixed page
// size.
func pagesFor(n int) int {
	if n <= 0 {
		return 1
	}
	return (n + FeedPageSize - 1) / FeedPageSize
}

// doiKey normalizes a doi for joining. Case is the only thing that varies
// between the two paths, since the feed states the doi bare in guid and the
// card states it in its href.
func doiKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func addPath(paths []string, p string) []string {
	for _, s := range paths {
		if s == p {
			return paths
		}
	}
	return append(paths, p)
}

func (r *SearchResponse) hasPath(p string) bool {
	for _, s := range r.Paths {
		if s == p {
			return true
		}
	}
	return false
}

// Requests is what this query will cost before it is run, which is what
// --dry-run prints.
func (q Query) Requests(limit int, enrich, facetsOnly bool) (rss, html int) {
	if facetsOnly {
		return 0, 1
	}
	if limit <= 0 {
		limit = FeedPageSize
	}
	rss = pagesFor(limit)
	html = 1
	if enrich {
		html = rss
	}
	return rss, html
}
