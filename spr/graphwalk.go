package spr

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Walking a seed into a graph.
//
// The walk only follows an edge to a page this tool knows how to read. That
// sounds obvious and it is the whole design: a crawler that follows every link
// on a publisher's site spends most of its requests on marketing pages, and one
// that follows references by DOI spends most of them on 404s, because a DOI
// from another publisher has no page here. So the frontier is small and every
// step in it was measured.
//
//	work     the container it names, and the works it recommends with --include-rails
//	journal  its volumes and issues page, which is the whole back catalogue for one request
//	book     the works in its table of contents
//	series   the books it lists
//	volumes  nothing, because an issue page has no reader in this tool yet
//
// A reference is followed only when its DOI carries this publisher's prefix,
// because those are the only ones with a page at the other end.

// SpringerDOIPrefix is the registrant prefix every DOI on this site carries.
const SpringerDOIPrefix = "10.1007"

// WalkOptions are the knobs on a graph walk.
type WalkOptions struct {
	// Depth is how many rounds of following to do. 0 reads the seed and stops.
	Depth int

	// IncludeRails turns the recommender strip into edges. Off by default,
	// because it is the publisher's recommender output and is neither a
	// citation nor an endorsement.
	IncludeRails bool

	// FollowRefs follows the references that carry a Springer DOI.
	FollowRefs bool

	// Crossref and OpenAlex ask the open indexes about every work the walk
	// reads, which is where the funders, the ROR ids and the reverse citation
	// direction come from. Neither is on by default: they are other people's
	// hosts and asking them is the caller's decision.
	Crossref bool
	OpenAlex bool

	// CitedBy is how many citing works to read per work, and 0 is none.
	CitedBy int

	// Limit stops the walk after this many pages. 0 means no limit.
	Limit int

	// Resume is the record of pages already read.
	Resume *Resume

	Note func(string)
}

// WalkStats is what a walk cost and what it did.
type WalkStats struct {
	// Pages is how many were fetched, Skipped is how many a resumed run already
	// had, and Failed is how many did not answer.
	Pages   int `json:"pages"`
	Skipped int `json:"skipped,omitempty"`
	Failed  int `json:"failed,omitempty"`

	// Backends is how many requests went to Crossref and OpenAlex, which are
	// counted apart because they are other hosts on their own pace buckets.
	Backends int `json:"backends,omitempty"`

	Bytes int `json:"bytes"`

	// Deepest is the depth actually reached, which is lower than --depth
	// whenever the frontier ran out first.
	Deepest int `json:"deepest"`
}

// target is one page to read, and how far from the seed it is.
type target struct {
	url   string
	depth int
}

// WalkGraph reads a seed and everything it leads to, into g.
//
// The graph is passed in rather than returned so that a walk that is stopped
// halfway, by a limit, a signal or a failing page, still hands back everything
// it had read up to that point. A partial graph is useful and throwing it away
// on the way out is not.
func (c *Client) WalkGraph(ctx context.Context, seeds []string, g *Graph, opts WalkOptions) (*WalkStats, error) {
	stats := &WalkStats{}
	note := func(format string, args ...any) {
		if opts.Note != nil {
			opts.Note(fmt.Sprintf(format, args...))
		}
	}

	seen := map[string]bool{}
	var queue []target
	push := func(raw string, depth int) {
		if raw == "" || depth > opts.Depth || seen[raw] {
			return
		}
		seen[raw] = true
		queue = append(queue, target{url: raw, depth: depth})
	}
	for _, s := range seeds {
		push(s, 0)
	}

	for len(queue) > 0 {
		if opts.Limit > 0 && stats.Pages >= opts.Limit {
			note("the walk stopped at the %d page limit with %d pages still queued", opts.Limit, len(queue))
			break
		}
		t := queue[0]
		queue = queue[1:]

		if opts.Resume != nil && opts.Resume.Done(t.url) {
			stats.Skipped++
			continue
		}
		resp, err := c.Get(ctx, t.url, KindHTML)
		if err != nil {
			if ctx.Err() != nil {
				return stats, err
			}
			// The seed failing is the walk failing, because a walk with no seed
			// read nothing. A page failing later is one gap in a long crawl and
			// it stays unmarked so that a resumed run comes back for it.
			if stats.Pages == 0 {
				return stats, err
			}
			stats.Failed++
			note("%s did not answer, %q, and it is left unread so that --resume comes back for it", t.url, err)
			continue
		}
		if resp.Status == StatusNotFound {
			stats.Failed++
			note("%s is not a page on this site", t.url)
			continue
		}
		stats.Pages++
		stats.Bytes += resp.Bytes()

		// The envelope keeps the seed url and the running byte count, and not
		// the url of every page. A graph of a whole journal is fourteen hundred
		// urls and a list of them is not provenance, it is a log. Which page
		// said what is on the nodes themselves.
		if g.Envelope.Fetched.IsZero() {
			g.Envelope.Fetched, g.Envelope.Status = resp.Fetched, resp.Status
			g.Envelope.URLs = append(g.Envelope.URLs, resp.URL)
		}
		g.Envelope.Bytes += resp.Bytes()
		g.Envelope.Redirects += resp.Redirects
		if t.depth > stats.Deepest {
			stats.Deepest = t.depth
		}

		next, err := c.readInto(ctx, g, resp, opts, stats)
		if err != nil {
			if ctx.Err() != nil {
				return stats, err
			}
			note("%s was fetched and not understood, %q", t.url, err)
		}
		for _, u := range next {
			push(u, t.depth+1)
		}

		if opts.Resume != nil {
			if err := opts.Resume.Mark(t.url); err != nil {
				note("the checkpoint could not be written, %q, so this run is not resumable", err)
				opts.Resume = nil
			}
		}
		if stats.Pages%25 == 0 && len(queue) > 0 {
			note("%d pages read, %d queued, %d nodes", stats.Pages, len(queue), len(g.Nodes))
		}
	}

	g.Envelope.sortMissed()
	return stats, nil
}

// readInto turns one fetched page into nodes and edges and reports where it
// leads. Everything it returns is a page this tool can read.
func (c *Client) readInto(ctx context.Context, g *Graph, resp *Response, opts WalkOptions, stats *WalkStats) ([]string, error) {
	switch {
	case VolumesPath(resp.URL) || VolumesPath(resp.Final):
		v, err := ExtractVolumes(resp)
		if err != nil {
			return nil, err
		}
		g.AddVolumes(v)
		return nil, nil

	case EntryKind(resp.URL) == "journal":
		j, err := ExtractJournal(resp)
		if err != nil {
			return nil, err
		}
		g.AddJournal(j)
		if j.Volumes != nil {
			return []string{j.Volumes.URL}, nil
		}
		return nil, nil

	case EntryKind(resp.URL) == "book" || EntryKind(resp.URL) == "referencework":
		b, err := ExtractBook(resp)
		if err != nil {
			return nil, err
		}
		g.AddBook(b)
		var next []string
		for _, ch := range b.Chapters {
			if d, err := ParseDOI(ch.DOI); err == nil {
				next = append(next, workPath(d))
			}
		}
		return next, nil

	case EntryKind(resp.URL) == "series":
		s, err := ExtractSeries(resp)
		if err != nil {
			return nil, err
		}
		g.AddSeries(s)
		var next []string
		for _, t := range s.LatestTitles {
			next = append(next, t.URL)
		}
		return next, nil
	}

	w, err := ExtractWork(resp)
	if err != nil {
		return nil, err
	}
	uri := g.AddWork(w, opts.IncludeRails)
	if uri == "" {
		return nil, nil
	}
	doi := mustDOI(w.DOI)
	c.enrich(ctx, g, doi, opts, stats)

	var next []string
	// The container, which is one page and is where the editors and the subject
	// list live. A journal is reached by its own number rather than by its ISSN
	// because there is no url on this site keyed on an ISSN.
	if id, ok := SpringerIDFromDOI(doi); ok {
		next = append(next, Base+"/journal/"+string(id))
	} else if w.ISBN != "" {
		if isbn, err := ParseISBN(w.ISBN); err == nil {
			next = append(next, Base+"/book/"+isbn.Key())
		}
	}
	if opts.IncludeRails {
		for _, r := range w.Rails {
			if d, err := ParseDOI(r.ID); err == nil && springerDOI(d) {
				next = append(next, workPath(d))
			}
		}
	}
	if opts.FollowRefs {
		for _, r := range w.References {
			if d, err := ParseDOI(r.DOI); err == nil && springerDOI(d) {
				next = append(next, workPath(d))
			}
		}
	}
	return next, nil
}

// enrich asks the open indexes about one work.
//
// Every failure here is a note and not an error. The walk is reading this
// publisher's pages and the indexes are a bonus on top; a Crossref timeout
// should not end a crawl that is otherwise working.
func (c *Client) enrich(ctx context.Context, g *Graph, d DOI, opts WalkOptions, stats *WalkStats) {
	if d == "" {
		return
	}
	if opts.Crossref {
		stats.Backends++
		if cw, err := c.CrossrefWork(ctx, d); err == nil {
			g.AddCrossrefWork(cw)
			stats.Backends++
			if refs, unresolved, err := c.CrossrefReferences(ctx, d); err == nil {
				g.AddCrossrefReferences(d, refs, unresolved, cw.Envelope.Fetched)
			}
		} else {
			g.Notes = append(g.Notes, fmt.Sprintf("crossref did not answer for %s, %q", d, err))
		}
	}
	if opts.OpenAlex || opts.CitedBy > 0 {
		stats.Backends++
		ow, err := c.OpenAlexWork(ctx, d)
		if err != nil {
			g.Notes = append(g.Notes, fmt.Sprintf("openalex did not answer for %s, %q", d, err))
			return
		}
		g.AddOpenAlexWork(ow)
		if opts.CitedBy > 0 {
			stats.Backends++
			citing, total, err := c.OpenAlexCitedBy(ctx, ow.ID, opts.CitedBy)
			if err == nil {
				g.AddCitedBy(d, citing, ow.Envelope.Fetched)
				if total > len(citing) {
					g.Notes = append(g.Notes, fmt.Sprintf(
						"%s is cited by %d works and the first %d of them are in this graph",
						d, total, len(citing)))
				}
			}
		}
	}
}

// AddVolumes adds a journal's volume and issue index.
//
// This is the one page on the site that is worth a walk on its own: 472 KB, one
// request, and the complete back catalogue of a journal as nodes.
func (g *Graph) AddVolumes(v *Volumes) string {
	if v == nil || v.Journal == nil {
		return ""
	}
	tier, at := v.Envelope.Tier, v.Envelope.Fetched
	id, err := ParseSpringerID(v.Journal.ID)
	if err != nil {
		return ""
	}
	journal := SpringerJournalURI(id)
	g.AddNode(Node{
		URI: journal, Kind: NodeJournal, Label: v.Journal.Name,
		Props: prop("springer_id", string(id)),
		Via:   "datalayer:Journal Id", Tier: tier, Fetched: at,
	})
	for _, vol := range v.Volumes {
		vu := VolumeURI(journal, vol.Number)
		if vu == "" {
			continue
		}
		props := map[string]string{}
		putIf(props, "name", firstNonEmpty(vol.Label, vol.Number))
		g.AddNode(Node{URI: vu, Kind: NodeVolume, Label: firstNonEmpty(vol.Label, "Volume "+vol.Number), Props: props, Via: "region:volumes-and-issues", Tier: tier, Fetched: at})
		g.AddEdge(Edge{From: vu, To: journal, Kind: EdgePartOf, Via: "region:volumes-and-issues", Tier: tier, Fetched: at})
		for _, iss := range vol.Issue {
			iu := IssueURI(vu, iss.Number)
			if iu == "" {
				continue
			}
			iprops := map[string]string{}
			putIf(iprops, "url", iss.URL)
			if iss.Date != nil {
				putIf(iprops, "published", iss.Date.String())
			}
			g.AddNode(Node{
				URI: iu, Kind: NodeIssue, Label: firstNonEmpty(iss.SpecialTitle, iss.Label, "Issue "+iss.Number),
				Props: iprops, Via: "region:volumes-and-issues", Tier: tier, Fetched: at,
			})
			g.AddEdge(Edge{From: iu, To: vu, Kind: EdgePartOf, Via: "region:volumes-and-issues", Tier: tier, Fetched: at})
		}
	}
	return journal
}

// MergeNames joins a name keyed person into an ORCID keyed one, when asked.
//
// This is off by default and it stays off by default. Merging on a name is the
// oldest known failure in bibliometrics and the failure is silent: a graph with
// two people fused into one looks exactly like a graph with one person in it.
// So the merge happens only when a caller asks for it, only when the normalized
// name matches exactly one ORCID node, and it records what it merged on the
// surviving node under spr:mergedFrom, which is the whole reason that term
// exists.
func (g *Graph) MergeNames() int {
	g.index()

	byName := map[string][]string{}
	for _, n := range g.Nodes {
		if n.Kind != NodePerson || !strings.HasPrefix(n.URI, URIPrefix+"person/orcid/") {
			continue
		}
		if key := normalizeName(n.Label); key != "" {
			byName[key] = append(byName[key], n.URI)
		}
	}

	merged := map[string]string{}
	for _, n := range g.Nodes {
		if n.Kind != NodePerson || !strings.HasPrefix(n.URI, URIPrefix+"person/name/") {
			continue
		}
		into := byName[normalizeName(firstNonEmpty(n.Label, n.Props["name"]))]
		// Exactly one, because two ORCIDs answering to one name is two people
		// with the same name, which is the case this whole rule exists for.
		if len(into) != 1 {
			continue
		}
		merged[n.URI] = into[0]
	}
	if len(merged) == 0 {
		return 0
	}

	kept := g.Nodes[:0]
	for _, n := range g.Nodes {
		to, gone := merged[n.URI]
		if !gone {
			kept = append(kept, n)
			continue
		}
		if i, ok := g.nodeAt[to]; ok {
			held := &g.Nodes[i]
			if held.Props == nil {
				held.Props = map[string]string{}
			}
			held.Props["merged_from"] = joinNonEmptyStrings(" ", held.Props["merged_from"], n.URI)
		}
	}
	// The surviving nodes were edited in place through the index, so the index
	// is rebuilt from the kept slice rather than trusted.
	g.Nodes = append([]Node(nil), kept...)
	g.nodeAt, g.edgeAt = nil, nil

	edges := make([]Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if to, ok := merged[e.From]; ok {
			e.From = to
		}
		if to, ok := merged[e.To]; ok {
			e.To = to
		}
		if e.From == e.To {
			continue
		}
		edges = append(edges, e)
	}
	g.Edges = nil
	g.index()
	for _, e := range edges {
		g.AddEdge(e)
	}
	g.Notes = append(g.Notes, fmt.Sprintf(
		"%d name keyed people were merged into an orcid on an exact name match, which is a guess and is recorded on each surviving node",
		len(merged)))
	return len(merged)
}

// GraphCost is what a walk will cost before it starts.
//
// The numbers are stated per depth rather than as one total because the shape
// of the bill is the point: depth 1 from a work is one container page, and
// depth 2 from a journal is the back catalogue. A person seeing 1,439 with no
// breakdown cannot tell those apart.
type GraphCost struct {
	Lines    []CostLine
	Requests int
	Backends int
	Duration time.Duration
}

// CostLine is one depth of the bill.
type CostLine struct {
	Depth    int
	Requests int
	What     string
}

// GraphThreshold is how many requests a walk may cost before the command stops
// for --yes. It is the same twenty the rest of this tool uses, because a person
// who has agreed to twenty requests once should not have to learn a second
// number for this command.
const GraphThreshold = 20

// BillGraph bills a walk from the fan-out that was measured on this site.
//
// Nothing here is fetched. The figures are the measured ones and they are named
// in the line they produced, so a bill that turns out wrong says which
// assumption was wrong rather than just being wrong.
func BillGraph(seedKind string, opts WalkOptions, pace time.Duration) GraphCost {
	cost := GraphCost{}
	add := func(depth, n int, what string) {
		if n <= 0 {
			return
		}
		cost.Lines = append(cost.Lines, CostLine{Depth: depth, Requests: n, What: what})
		cost.Requests += n
	}

	add(0, 1, seedKind+" page, the seed")
	frontier, what := 1, seedKind
	for d := 1; d <= opts.Depth; d++ {
		var n int
		// Parts is what the line says, and it is a list rather than one string
		// because a work at depth 1 can be a journal page and four references
		// and five recommendations, and calling all ten of those journal pages
		// would be a bill that lies about where the requests went.
		var parts []string
		switch what {
		case "journal":
			n, what = frontier*1, "volumes"
			parts = []string{countOf(n, "volumes page")}
		case "book", "referencework":
			// The measured proceedings book holds 40 chapters, which is the
			// figure this line is built from and is not a ceiling.
			n, what = frontier*40, "work"
			parts = []string{countOf(n, "work page")}
		case "series":
			// A series page lists its latest titles, 10 of them on the measured
			// page, and the rest are behind a connection this tool does not
			// page through.
			n, what = frontier*10, "book"
			parts = []string{countOf(n, "book page")}
		case "work":
			n, what = frontier*1, "journal"
			parts = []string{countOf(n, "journal page")}
			if opts.FollowRefs {
				// 122 references on the measured article, 66 with a DOI, and 4
				// of those carry this publisher's prefix.
				n += frontier * 4
				parts = append(parts, countOf(frontier*4, "reference")+" this publisher has a page for")
			}
			if opts.IncludeRails {
				n += frontier * 5
				parts = append(parts, countOf(frontier*5, "recommendation"))
			}
		default:
			n = 0
		}
		if n == 0 {
			break
		}
		add(d, n, strings.Join(parts, ", "))
		frontier = n
	}

	works := 0
	for _, l := range cost.Lines {
		if strings.HasPrefix(l.What, "work") || seedKind == "work" && l.Depth == 0 {
			works += l.Requests
		}
	}
	if opts.Crossref {
		cost.Backends += works * 2
	}
	if opts.OpenAlex {
		cost.Backends += works
	}
	if opts.CitedBy > 0 {
		cost.Backends += works * 2
	}

	// The backends are not in the estimate. They are separate hosts with their
	// own pace buckets, so their requests run alongside these rather than
	// behind them.
	if cost.Requests > 1 {
		cost.Duration = time.Duration(cost.Requests-1) * pace
	}
	return cost
}

// SeedKind names what a seed argument addresses, from the argument alone.
func SeedKind(raw string) string {
	switch k := EntryKind(raw); k {
	case "journal":
		if VolumesPath(raw) {
			return "volumes"
		}
		return "journal"
	case "book", "referencework", "series":
		return k
	case "article", "chapter", "protocol", "entry":
		return "work"
	}
	if _, err := ParseDOI(raw); err == nil {
		return "work"
	}
	return ""
}

// GraphSeedURL turns one seed argument into one url.
//
// A DOI is enough for a work, and a bare journal number is enough for a
// journal, because those are the two things a person has in their hand when
// they want a graph.
func GraphSeedURL(arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", errors.New("a graph needs a seed to start from")
	}
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
		return arg, nil
	}
	if strings.HasPrefix(arg, "/") {
		return Base + arg, nil
	}
	if d, err := ParseDOI(arg); err == nil {
		return workPath(d), nil
	}
	if id, err := ParseSpringerID(arg); err == nil {
		return Base + "/journal/" + string(id), nil
	}
	return "", fmt.Errorf("%q is not a doi, a journal number, a path or a url", arg)
}

// workPath is where a work by this publisher lives.
//
// The prefix does not say what a work is, so this picks the first of the paths
// the DOI could live under. The walk pays one request for a miss and a work
// seed given as a path pays none, which is why the command takes both.
func workPath(d DOI) string {
	paths := d.Paths()
	if len(paths) == 0 {
		return ""
	}
	return Base + paths[0]
}

// countOf writes a count and its noun, which the bill prints as it stands.
func countOf(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func springerDOI(d DOI) bool {
	return strings.HasPrefix(string(d), SpringerDOIPrefix+"/")
}

// joinNonEmptyStrings joins what is there, and is not the cli helper of a
// similar name because this one keeps the order and drops duplicates.
func joinNonEmptyStrings(sep string, vals ...string) string {
	seen := map[string]bool{}
	var out []string
	for _, v := range vals {
		for _, part := range strings.Fields(v) {
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	sort.Strings(out)
	return strings.Join(out, sep)
}
