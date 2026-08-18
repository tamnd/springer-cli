package spr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
)

// The graph: what a thing is called, and what edges exist between things.
//
// Identity is the easy half here and it was not on the last site this shape of
// tool was written for. Scholarly publishing solved naming in the 1990s and
// Springer emits the results in the page head: a DOI names a work, an ISSN a
// journal, an ISBN a book, an ORCID a person, a ROR an institution. This
// package mints no identifier for anything that already has one.
//
// The hard half is the opposite one. Four separate authorities are in play and
// the job is keeping them from being quietly conflated. That is what the uri
// space below is for: every node's uri names the authority that identified it,
// so a person known by a name string and a person known by an ORCID are two
// visibly different nodes rather than one node with a confidence problem.

// NodeKind is what a node is.
type NodeKind string

const (
	NodeWork       NodeKind = "work"
	NodeJournal    NodeKind = "journal"
	NodeBook       NodeKind = "book"
	NodeSeries     NodeKind = "series"
	NodeVolume     NodeKind = "volume"
	NodeIssue      NodeKind = "issue"
	NodePerson     NodeKind = "person"
	NodeOrg        NodeKind = "org"
	NodeFunder     NodeKind = "funder"
	NodeConference NodeKind = "conference"
	NodeSubject    NodeKind = "subject"
	NodePublisher  NodeKind = "publisher"
)

// EdgeKind is one of the fifteen relations this tool asserts.
//
// The list is closed on purpose. Every entry is a relation something in the
// measured data states, and there is nothing here that was inferred from the
// shape of the rest of the graph.
type EdgeKind string

const (
	// Containment, all four of which come off the page's own meta tags.
	EdgePartOf   EdgeKind = "partOf"
	EdgeInVolume EdgeKind = "inVolume"
	EdgeInIssue  EdgeKind = "inIssue"
	EdgeInSeries EdgeKind = "inSeries"

	EdgePresentedAt EdgeKind = "presentedAt"

	// People, and where they say they work.
	EdgeAuthoredBy     EdgeKind = "authoredBy"
	EdgeEditedBy       EdgeKind = "editedBy"
	EdgeAffiliatedWith EdgeKind = "affiliatedWith"

	// The two citation directions. Only the first is derivable from a page,
	// and only after a reference has been resolved to an identifier.
	EdgeCites    EdgeKind = "cites"
	EdgeCitedBy  EdgeKind = "citedBy"
	EdgeFundedBy EdgeKind = "fundedBy"

	EdgeHasSubject  EdgeKind = "hasSubject"
	EdgePublishedBy EdgeKind = "publishedBy"

	// Recommends is the publisher's recommender output and is neither a
	// citation nor an endorsement, which is why it has its own name and is off
	// by default.
	EdgeRecommends EdgeKind = "recommends"

	// SameAs is identifier equivalence and nothing else. It is never asserted
	// between two nodes on the strength of a matching label.
	EdgeSameAs EdgeKind = "sameAs"
)

// EdgeKinds is every edge this tool emits, in the order the reference table
// lists them.
var EdgeKinds = []EdgeKind{
	EdgePartOf, EdgeInVolume, EdgeInIssue, EdgeInSeries, EdgePresentedAt,
	EdgeAuthoredBy, EdgeEditedBy, EdgeAffiliatedWith,
	EdgeCites, EdgeCitedBy, EdgeFundedBy,
	EdgeHasSubject, EdgePublishedBy, EdgeRecommends, EdgeSameAs,
}

// Node is one thing in the graph.
//
// Props is a string map rather than a struct per kind because the graph is a
// projection and not a second copy of the records. A consumer that wants every
// field of a work reads the work record; what belongs here is the handful of
// attributes that make a node readable in a graph tool with no other context.
type Node struct {
	URI   string            `json:"uri"`
	Kind  NodeKind          `json:"kind"`
	Label string            `json:"label,omitempty"`
	Props map[string]string `json:"props,omitempty"`

	// Via and Tier are where this node came from, carried on the node for the
	// same reason they are carried on an edge: a graph merged from four
	// sources where you cannot tell which source said what is a graph you
	// cannot audit.
	Via     string    `json:"via,omitempty"`
	Tier    string    `json:"tier,omitempty"`
	Fetched time.Time `json:"fetched,omitempty"`
}

// Edge is one asserted relation.
type Edge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Kind EdgeKind `json:"type"`

	// Position is the author's place in the list, one based, and 0 on every
	// edge kind where order means nothing. Author position is a fact with
	// meaning in every field that uses this publisher: first, last and
	// corresponding are three different roles.
	Position int `json:"position,omitempty"`

	// Sequence is Crossref's own word for the same thing, first or additional,
	// and fills only when that tier answered.
	Sequence string `json:"sequence,omitempty"`

	// Role is the title a container page printed over a name, Editor-in-Chief
	// on a journal and Series Editor on a series.
	Role string `json:"role,omitempty"`

	Via     string    `json:"via,omitempty"`
	Tier    string    `json:"tier,omitempty"`
	Fetched time.Time `json:"fetched,omitempty"`
}

// key is what makes an edge the same edge on a second run. Position is in it
// because the same person can appear twice in one author list under two
// affiliations, and the two are not one edge.
func (e Edge) key() string {
	return strings.Join([]string{e.From, string(e.Kind), e.To, fmt.Sprint(e.Position)}, "\x00")
}

// Graph is a set of nodes and edges with no duplicates in it.
//
// The two indexes are what make Add cheap and what make a merge of two runs a
// set union rather than a join. Both are keyed on the uri, which is content
// addressed, so the same node discovered by two runs from two seeds is the same
// node without anything having to compare labels.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`

	// Notes are the sentences a build wants a reader to see, such as how many
	// references produced no edge.
	Notes []string `json:"notes,omitempty"`

	Envelope Envelope `json:"envelope"`

	nodeAt map[string]int
	edgeAt map[string]bool
}

// NewGraph returns an empty graph with its indexes ready.
func NewGraph() *Graph {
	return &Graph{
		nodeAt:   map[string]int{},
		edgeAt:   map[string]bool{},
		Envelope: Envelope{Tier: "graph", Status: StatusOK},
	}
}

// index rebuilds the maps, which a graph decoded from json does not have.
func (g *Graph) index() {
	if g.nodeAt == nil {
		g.nodeAt = make(map[string]int, len(g.Nodes))
		for i, n := range g.Nodes {
			g.nodeAt[n.URI] = i
		}
	}
	if g.edgeAt == nil {
		g.edgeAt = make(map[string]bool, len(g.Edges))
		for _, e := range g.Edges {
			g.edgeAt[e.key()] = true
		}
	}
}

// AddNode adds a node, or fills in what an earlier sighting of it did not know.
//
// A node is commonly seen twice: once as a bare reference from a work that
// names its journal, and once with everything on it when the journal page is
// read. The second sighting fills the blanks and never overwrites, because the
// first one to answer is the rung that won and a later, thinner source should
// not quietly replace it.
func (g *Graph) AddNode(n Node) {
	if n.URI == "" {
		return
	}
	g.index()
	i, ok := g.nodeAt[n.URI]
	if !ok {
		g.nodeAt[n.URI] = len(g.Nodes)
		g.Nodes = append(g.Nodes, n)
		return
	}
	held := &g.Nodes[i]
	if held.Label == "" {
		held.Label = n.Label
	}
	if held.Via == "" {
		held.Via, held.Tier, held.Fetched = n.Via, n.Tier, n.Fetched
	}
	for k, v := range n.Props {
		if v == "" {
			continue
		}
		if held.Props == nil {
			held.Props = map[string]string{}
		}
		if held.Props[k] == "" {
			held.Props[k] = v
		}
	}
}

// AddEdge adds an edge unless the same one is already there.
//
// An edge to or from an empty uri is dropped rather than emitted with a blank
// end. That is the mechanism behind the rule about unresolved references: a
// reference with no identifier produces no uri, so it produces no edge, without
// anything having to check for that case a second time.
func (g *Graph) AddEdge(e Edge) {
	if e.From == "" || e.To == "" || e.Kind == "" {
		return
	}
	g.index()
	if g.edgeAt[e.key()] {
		return
	}
	g.edgeAt[e.key()] = true
	g.Edges = append(g.Edges, e)
}

// Node returns one node by uri.
func (g *Graph) Node(uri string) (Node, bool) {
	g.index()
	i, ok := g.nodeAt[uri]
	if !ok {
		return Node{}, false
	}
	return g.Nodes[i], true
}

// Merge folds another graph into this one and reports how many nodes and edges
// were already held.
//
// This never merges on anything but identifier equality. Two runs from two
// seeds produce the same uri for the same thing because the uri is derived from
// the identifier, so a merge is a set union and there is no similarity
// threshold anywhere in it.
func (g *Graph) Merge(other *Graph) (nodes, edges int) {
	if other == nil {
		return 0, 0
	}
	g.index()
	for _, n := range other.Nodes {
		if _, held := g.nodeAt[n.URI]; held {
			nodes++
		}
		g.AddNode(n)
	}
	for _, e := range other.Edges {
		if g.edgeAt[e.key()] {
			edges++
			continue
		}
		g.AddEdge(e)
	}
	g.Notes = append(g.Notes, other.Notes...)
	for k, v := range other.Envelope.Via {
		g.Envelope.carry(k, v)
	}
	return nodes, edges
}

// Sort orders the graph so that two runs that found the same things write the
// same bytes.
//
// Output that is stable under reordering is what makes a diff of two runs
// readable and what makes a checksum of a graph mean anything. Nothing in the
// walk visits in a guaranteed order, so the order is imposed here at the end
// rather than defended everywhere along the way.
func (g *Graph) Sort() {
	sort.SliceStable(g.Nodes, func(i, j int) bool { return g.Nodes[i].URI < g.Nodes[j].URI })
	sort.SliceStable(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		switch {
		case a.From != b.From:
			return a.From < b.From
		case a.Kind != b.Kind:
			return a.Kind < b.Kind
		case a.Position != b.Position:
			return a.Position < b.Position
		default:
			return a.To < b.To
		}
	})
	g.nodeAt, g.edgeAt = nil, nil
	g.index()
}

// Counts is how many of each kind, for the summary a run prints.
func (g *Graph) Counts() (nodes map[NodeKind]int, edges map[EdgeKind]int) {
	nodes, edges = map[NodeKind]int{}, map[EdgeKind]int{}
	for _, n := range g.Nodes {
		nodes[n.Kind]++
	}
	for _, e := range g.Edges {
		edges[e.Kind]++
	}
	return nodes, edges
}

// The uri space.
//
// Every node gets a uri so that a graph can be emitted as RDF and merged with
// somebody else's. The prefix is spr: and the first segment is the kind, so a
// uri is readable without a schema in front of you.

// URIPrefix is the namespace every node uri sits in.
const URIPrefix = "spr:"

// WorkURI names a work by its DOI, normalized to the bare lowercased form.
func WorkURI(d DOI) string {
	if d == "" {
		return ""
	}
	return URIPrefix + "work/" + string(d)
}

// JournalURI names a journal by an ISSN, which is its identity.
func JournalURI(i ISSN) string {
	if i == "" {
		return ""
	}
	return URIPrefix + "journal/" + string(i)
}

// SpringerJournalURI is a second uri for the same journal, keyed on the
// publisher's own number.
//
// 10994 is in the url, in the DOI suffix and in the analytics payload. It is
// stable and it is what this site's own urls are built from, so a graph built
// from urls alone, before any ISSN has been read, needs somewhere to hang a
// journal. It is emitted alongside the ISSN uri with a sameAs between the two,
// so the two halves join up when the ISSN arrives later. It is never the
// journal's identity, because a number that means nothing outside one publisher
// is not an identifier anybody else can use.
func SpringerJournalURI(id SpringerID) string {
	if id == "" {
		return ""
	}
	return URIPrefix + "journal/springer/" + string(id)
}

// BookURI names a book by its ISBN-13 with the hyphens stripped.
func BookURI(i ISBN) string {
	if i == "" {
		return ""
	}
	return URIPrefix + "book/" + i.Key()
}

// BookDOIURI names a book by its DOI, for the books whose ISBN was not read.
func BookDOIURI(d DOI) string {
	if d == "" {
		return ""
	}
	return URIPrefix + "book/doi/" + string(d)
}

// SeriesURI names a book series by its ISSN.
func SeriesURI(i ISSN) string {
	if i == "" {
		return ""
	}
	return URIPrefix + "series/" + string(i)
}

// VolumeURI and IssueURI are scoped under the journal, because a volume number
// is meaningless on its own. Volume 110 exists in several thousand journals.
func VolumeURI(journal, number string) string {
	if journal == "" || number == "" {
		return ""
	}
	return journal + "/volume/" + urlSegment(number)
}

func IssueURI(volume, number string) string {
	if volume == "" || number == "" {
		return ""
	}
	return volume + "/issue/" + urlSegment(number)
}

// PersonORCIDURI names a person by an ORCID, which is the only identifier on
// this site that names a human being.
func PersonORCIDURI(o ORCID) string {
	if o == "" {
		return ""
	}
	return URIPrefix + "person/orcid/" + string(o)
}

// PersonNameURI names a string that appeared in an author position.
//
// It is not a person, and the name/ segment says so in every uri that carries
// it. On the measured article 1 of 2 authors had an ORCID, so most people in
// any graph built from this site arrive as a name and nothing else. Merging one
// of these into an orcid/ node on the strength of a matching name is the oldest
// known failure in bibliometrics and it produces a graph that is wrong in a way
// nobody downstream can see, so this tool does not do it unless it is asked
// and, when it is asked, records what it merged.
func PersonNameURI(name string) string {
	h := nameHash(name)
	if h == "" {
		return ""
	}
	return URIPrefix + "person/name/" + h
}

// OrgRORURI names an institution by its ROR, which arrives from OpenAlex only.
func OrgRORURI(r ROR) string {
	if r == "" {
		return ""
	}
	return URIPrefix + "org/ror/" + string(r)
}

// OrgNameURI names an affiliation string, with the same caveat PersonNameURI
// carries and for the same reason.
func OrgNameURI(name string) string {
	h := nameHash(name)
	if h == "" {
		return ""
	}
	return URIPrefix + "org/name/" + h
}

// FunderURI names a funder by its registry DOI, and falls back to the name when
// the deposit carried no id.
//
// Projekt DEAL is deposited on the measured work with a name and no DOI at all,
// so the fallback is not a defensive branch: it is the case that actually
// occurs, and a funder with no id joins to nothing, which is what the name/
// segment says.
func FunderURI(d DOI, name string) string {
	if d != "" {
		return URIPrefix + "funder/" + string(d)
	}
	if h := nameHash(name); h != "" {
		return URIPrefix + "funder/name/" + h
	}
	return ""
}

// ConferenceURI names a conference by acronym and year, which together are the
// only handle a conference has. There is no page for one: /conference/aaai
// answers 404 with a zero byte body, so nothing here builds a url from this.
func ConferenceURI(acronym string, year int) string {
	acronym = strings.ToLower(strings.TrimSpace(acronym))
	if acronym == "" {
		return ""
	}
	if year <= 0 {
		return URIPrefix + "conference/" + urlSegment(acronym)
	}
	return fmt.Sprintf("%sconference/%s/%d", URIPrefix, urlSegment(acronym), year)
}

// SubjectURI names one term of a classification, slugged.
func SubjectURI(term string) string {
	s := slug(term)
	if s == "" {
		return ""
	}
	return URIPrefix + "subject/" + s
}

// PublisherURI names a publisher by name, because no publisher on this site
// states an identifier for itself anywhere that was measured.
func PublisherURI(name string) string {
	h := nameHash(name)
	if h == "" {
		return ""
	}
	return URIPrefix + "publisher/name/" + h
}

// nameHash is the sha256 of a normalized name, hex encoded.
//
// The normalization is deliberately shallow: unicode case folding, whitespace
// collapsed, and nothing else. It does not reorder Hüllermeier, Eyke into Eyke
// Hüllermeier, because deciding which half of a name is the family name is a
// guess that is wrong often enough across the world's naming conventions that
// an absent join is better than a wrong one. Two spellings of one person are
// two nodes here, and that is the honest answer rather than a defect.
func nameHash(name string) string {
	n := normalizeName(name)
	if n == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(n))
	return hex.EncodeToString(sum[:])
}

func normalizeName(s string) string {
	return strings.ToLower(collapse(strings.TrimSpace(s)))
}

// slug is a lowercased, hyphenated form of a term, for the one uri space where
// the value is prose rather than an identifier.
func slug(s string) string {
	var b strings.Builder
	last := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			last = false
		case !last && b.Len() > 0:
			b.WriteByte('-')
			last = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// urlSegment keeps a uri segment free of the characters that would need
// escaping in every serialization this graph is written in.
func urlSegment(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ReadGraph decodes a graph written by an earlier run.
//
// Only the json form round trips, because it is the only one of the ten that
// holds every field of every node and edge. The rdf forms drop the properties
// that have no standard term and the two xml forms number their nodes, so
// reading either back would silently return a smaller graph than was written.
func ReadGraph(r io.Reader) (*Graph, error) {
	g := &Graph{}
	if err := json.NewDecoder(r).Decode(g); err != nil {
		return nil, err
	}
	g.index()
	return g, nil
}
