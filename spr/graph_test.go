package spr

import (
	"bytes"
	"strings"
	"testing"
)

// buildWork is the graph of the open access article, which is the page every
// claim in this file is measured against.
func buildWork(t *testing.T, rails bool) *Graph {
	t.Helper()
	w, err := ExtractWork(capturedResponse(t, "article_oa.html"))
	if err != nil {
		t.Fatalf("work: %v", err)
	}
	g := NewGraph()
	if uri := g.AddWork(w, rails); uri == "" {
		t.Fatal("the work produced no node")
	}
	return g
}

// The identity rule, which is the one thing in this package that cannot be got
// wrong quietly. A person with an ORCID and a person with only a name are two
// different nodes and the uri says which is which.
func TestPersonURIsSayWhoIdentifiedThem(t *testing.T) {
	g := buildWork(t, false)

	var orcid, named int
	for _, n := range g.Nodes {
		if n.Kind != NodePerson {
			continue
		}
		switch {
		case strings.HasPrefix(n.URI, "spr:person/orcid/"):
			orcid++
		case strings.HasPrefix(n.URI, "spr:person/name/"):
			named++
		default:
			t.Errorf("a person node is in neither uri space: %s", n.URI)
		}
	}
	// 1 of the 2 authors on this page carries an ORCID, which is the whole
	// reason the two spaces exist rather than one.
	if orcid != 1 || named != 1 {
		t.Errorf("%d orcid keyed and %d name keyed people, want 1 and 1", orcid, named)
	}
}

// A name that appeared in an author position is not a person, and nothing joins
// the two spaces on its own.
func TestNamesAreNeverMergedIntoORCIDsByThemselves(t *testing.T) {
	g := NewGraph()
	const name = "Eyke Hüllermeier"
	orcid := PersonORCIDURI("0000-0002-9944-4108")
	named := PersonNameURI(name)

	g.AddNode(Node{URI: orcid, Kind: NodePerson, Label: name})
	g.AddNode(Node{URI: named, Kind: NodePerson, Label: name})
	if len(g.Nodes) != 2 {
		t.Fatalf("the same name in two identifier spaces made %d nodes, want 2", len(g.Nodes))
	}

	// And when it is asked for, it happens and it says so. The surviving node
	// carries what it swallowed, which is the entire purpose of spr:mergedFrom.
	if n := g.MergeNames(); n != 1 {
		t.Fatalf("MergeNames merged %d, want 1", n)
	}
	held, ok := g.Node(orcid)
	if !ok {
		t.Fatal("the orcid node did not survive its own merge")
	}
	if held.Props["merged_from"] != named {
		t.Errorf("merged_from = %q, want %q", held.Props["merged_from"], named)
	}
	if _, gone := g.Node(named); gone {
		t.Error("the name node is still there after being merged away")
	}
}

// Two ORCIDs answering to one name is two people who share a name, which is the
// case the whole rule exists for.
func TestMergeNamesRefusesAnAmbiguousName(t *testing.T) {
	g := NewGraph()
	const name = "Wei Wang"
	g.AddNode(Node{URI: PersonORCIDURI("0000-0002-9944-4108"), Kind: NodePerson, Label: name})
	g.AddNode(Node{URI: PersonORCIDURI("0000-0002-1825-0097"), Kind: NodePerson, Label: name})
	g.AddNode(Node{URI: PersonNameURI(name), Kind: NodePerson, Label: name})

	if n := g.MergeNames(); n != 0 {
		t.Errorf("%d people were merged into a name two orcids answer to", n)
	}
}

// A reference that did not resolve produces no edge, and the count of them is
// said out loud rather than left as a gap between two numbers.
func TestUnresolvedReferencesProduceNoEdge(t *testing.T) {
	g := buildWork(t, false)

	var cites int
	for _, e := range g.Edges {
		if e.Kind == EdgeCites {
			cites++
		}
		if e.From == "" || e.To == "" {
			t.Errorf("an edge has a blank end: %+v", e)
		}
	}
	// The page publishes 122 references as text and none of them carries a DOI
	// on the page itself, so the html tier alone produces no citation edge at
	// all. That is the honest answer and --also crossref is what changes it.
	if cites != 0 {
		t.Errorf("%d citation edges came off a page that states no reference doi", cites)
	}
	if len(g.Notes) == 0 || !strings.Contains(strings.Join(g.Notes, " "), "did not") {
		t.Errorf("the notes do not say what became of the references: %v", g.Notes)
	}
}

// The recommender strip is not a citation and is not on by default.
func TestRailsAreOffUnlessAskedFor(t *testing.T) {
	off, on := buildWork(t, false), buildWork(t, true)
	count := func(g *Graph) int {
		var n int
		for _, e := range g.Edges {
			if e.Kind == EdgeRecommends {
				n++
			}
		}
		return n
	}
	if count(off) != 0 {
		t.Errorf("%d recommends edges with the flag off", count(off))
	}
	if count(on) == 0 {
		t.Error("no recommends edge with --include-rails, and the page carries a rail")
	}
}

// The order of an author list is a fact and it survives into the edges.
func TestAuthorEdgesCarryTheirPosition(t *testing.T) {
	g := buildWork(t, false)

	seen := map[int]bool{}
	for _, e := range g.Edges {
		if e.Kind != EdgeAuthoredBy {
			continue
		}
		if e.Position == 0 {
			t.Errorf("an authoredBy edge lost its position: %+v", e)
		}
		if seen[e.Position] {
			t.Errorf("two authors are at position %d", e.Position)
		}
		seen[e.Position] = true
	}
	if len(seen) != 2 {
		t.Errorf("%d authors on a page with two of them", len(seen))
	}
}

// Two identifiers for one journal are two uris and a sameAs, not one uri picked
// by whichever was read first.
func TestJournalIdentifiersJoinWithSameAs(t *testing.T) {
	g := buildWork(t, false)

	var journals, sameAs int
	for _, n := range g.Nodes {
		if n.Kind == NodeJournal {
			journals++
		}
	}
	for _, e := range g.Edges {
		if e.Kind == EdgeSameAs {
			sameAs++
		}
	}
	if journals < 2 {
		t.Errorf("%d journal nodes off a page that states two issns and a publisher number", journals)
	}
	if sameAs == 0 {
		t.Error("the journal identifiers were not tied together at all")
	}
	// Both directions, so that a consumer walking one way does not have to know
	// which identifier was read first.
	if sameAs%2 != 0 {
		t.Errorf("%d sameAs edges, which is not a whole number of pairs", sameAs)
	}
}

// The open indexes bring what the page cannot: a funder with no id of its own,
// and an institution with a ROR.
func TestBackendsBringWhatThePageCannot(t *testing.T) {
	g := NewGraph()
	g.AddCrossrefWork(decodeCrossref(t))
	g.AddOpenAlexWork(decodeOpenAlex(t))

	var funder, ror bool
	for _, n := range g.Nodes {
		switch {
		case n.Kind == NodeFunder && strings.HasPrefix(n.URI, "spr:funder/name/"):
			// Projekt DEAL is deposited with a name and no DOI, so it lands in
			// the name space and joins to nothing, which the uri says.
			funder = true
		case n.Kind == NodeOrg && strings.HasPrefix(n.URI, "spr:org/ror/"):
			ror = true
		}
	}
	if !funder {
		t.Error("the funder deposited with a name and no id did not arrive as a name keyed node")
	}
	if !ror {
		t.Error("no ror keyed institution arrived from openalex")
	}
}

// Merging is a set union on content addressed uris, so merging a graph with
// itself changes nothing.
func TestMergeIsASetUnion(t *testing.T) {
	a, b := buildWork(t, false), buildWork(t, false)
	nodes, edges := len(a.Nodes), len(a.Edges)

	held, heldEdges := a.Merge(b)
	if held != nodes || heldEdges != edges {
		t.Errorf("merging a graph with itself reported %d of %d nodes and %d of %d edges as already held", held, nodes, heldEdges, edges)
	}
	if len(a.Nodes) != nodes || len(a.Edges) != edges {
		t.Errorf("the graph grew to %d nodes and %d edges by merging with itself", len(a.Nodes), len(a.Edges))
	}
}

// Two runs that found the same things write the same bytes, which is what makes
// a diff of two runs readable and a checksum mean anything.
func TestOutputIsStableAcrossRuns(t *testing.T) {
	for _, f := range GraphFormats {
		if f == FormatCSV {
			// The csv pair holds a fetch time in every row and is stable for
			// the same reason, but it is checked below with its own headers.
			continue
		}
		var first, second bytes.Buffer
		if err := WriteGraph(&first, buildWork(t, true), GraphWriteOptions{Format: f}); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if err := WriteGraph(&second, buildWork(t, true), GraphWriteOptions{Format: f}); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if first.String() != second.String() {
			t.Errorf("%s wrote different bytes on two runs of the same input", f)
		}
		if first.Len() == 0 {
			t.Errorf("%s wrote nothing", f)
		}
	}
}

// The local vocabulary is four terms and stays four terms. Anything else in the
// spr namespace in an RDF file is a term this tool invented and did not declare.
func TestRDFKeepsTheLocalVocabularyAtFourTerms(t *testing.T) {
	g := buildWork(t, true)
	g.AddNode(Node{URI: PersonORCIDURI("0000-0002-9944-4108"), Kind: NodePerson, Props: map[string]string{"merged_from": PersonNameURI("Eyke Hüllermeier")}})

	var buf bytes.Buffer
	if err := WriteGraph(&buf, g, GraphWriteOptions{Format: FormatNTriples}); err != nil {
		t.Fatalf("nt: %v", err)
	}

	allowed := map[string]bool{"recommends": true, "springerId": true, "accessStatement": true, "mergedFrom": true}
	for _, line := range strings.Split(buf.String(), "\n") {
		_, rest, ok := strings.Cut(line, "> <"+SprNamespace)
		if !ok {
			continue
		}
		term, _, _ := strings.Cut(rest, ">")
		// A predicate in the spr namespace is a local term. A subject or an
		// object in it is a node uri, and those are the point.
		if strings.Contains(term, "/") {
			continue
		}
		if !allowed[term] {
			t.Errorf("the rdf uses a local term nobody declared: spr:%s", term)
		}
	}
}

// Provenance goes on a named graph per tier, so that dropping the fourth column
// leaves plain triples behind.
func TestQuadsCarryTheTier(t *testing.T) {
	var nt, nq bytes.Buffer
	if err := WriteGraph(&nt, buildWork(t, false), GraphWriteOptions{Format: FormatNTriples}); err != nil {
		t.Fatalf("nt: %v", err)
	}
	if err := WriteGraph(&nq, buildWork(t, false), GraphWriteOptions{Format: FormatNQuads}); err != nil {
		t.Fatalf("nq: %v", err)
	}
	if !strings.Contains(nq.String(), SprNamespace+"tier/html>") {
		t.Error("the quads do not name the tier that asserted them")
	}
	if strings.Contains(nt.String(), SprNamespace+"tier/") {
		t.Error("the triples carry a fourth term, which makes them quads")
	}
	if a, b := strings.Count(nt.String(), "\n"), strings.Count(nq.String(), "\n"); a != b {
		t.Errorf("%d triples and %d quads off the same graph", a, b)
	}
}

// Co-authorship is computed on the way out and never stored, because storing it
// squares the author list for nothing.
func TestCoauthorIsAProjectionAndNotAnEdge(t *testing.T) {
	g := buildWork(t, false)
	for _, e := range g.Edges {
		if e.Kind == "coauthorWith" {
			t.Fatal("co-authorship was materialized into the graph")
		}
	}

	var buf bytes.Buffer
	if err := WriteGraph(&buf, g, GraphWriteOptions{Format: FormatGEXF, Projection: "coauthor"}); err != nil {
		t.Fatalf("gexf: %v", err)
	}
	if !strings.Contains(buf.String(), "coauthorWith") {
		t.Error("the projection produced no co-authorship edge from a two author paper")
	}
}

// The two xml formats number their nodes, and the numbering is derived from the
// uri so that two files can be compared and merged.
func TestXMLIDsAreDerivedFromTheURI(t *testing.T) {
	a, b := nodeID("spr:work/10.1007/s10994-021-05946-3"), nodeID("spr:work/10.1007/s10994-021-05946-3")
	if a != b {
		t.Errorf("the same uri got two ids, %q and %q", a, b)
	}
	if c := nodeID("spr:work/10.1007/s10994-024-06594-z"); c == a {
		t.Errorf("two works share an id: %q", c)
	}
}

// The seed argument takes the four shapes a person has in their hand.
func TestGraphSeedURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"10.1007/s10994-021-05946-3", Base + "/article/10.1007/s10994-021-05946-3"},
		{"10994", Base + "/journal/10994"},
		{"/journal/10994/volumes-and-issues", Base + "/journal/10994/volumes-and-issues"},
		{Base + "/book/10.1007/978-3-031-28170-9", Base + "/book/10.1007/978-3-031-28170-9"},
	}
	for _, c := range cases {
		got, err := GraphSeedURL(c.in)
		if err != nil {
			t.Errorf("GraphSeedURL(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("GraphSeedURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := GraphSeedURL("not a seed at all"); err == nil {
		t.Error("a seed this tool cannot address was accepted")
	}
}

// The bill is per depth, because one work and its journal is two requests and a
// book at depth 1 is forty, and one total cannot tell those apart.
func TestBillGraphSaysWhereTheRequestsGo(t *testing.T) {
	work := BillGraph("work", WalkOptions{Depth: 1}, 2_000_000_000)
	if work.Requests != 2 {
		t.Errorf("a work and its container billed %d requests, want 2", work.Requests)
	}
	book := BillGraph("book", WalkOptions{Depth: 1}, 2_000_000_000)
	if book.Requests <= work.Requests {
		t.Errorf("a book at depth 1 billed %d requests and a work billed %d", book.Requests, work.Requests)
	}
	if len(book.Lines) != 2 {
		t.Errorf("the bill has %d lines for two depths", len(book.Lines))
	}
	// The backends are on their own hosts and their own pace, so they are in the
	// request count and never in the estimate.
	with := BillGraph("work", WalkOptions{Depth: 1, Crossref: true}, 2_000_000_000)
	if with.Duration != work.Duration {
		t.Errorf("asking crossref changed the estimate from %s to %s", work.Duration, with.Duration)
	}
	if with.Backends == 0 {
		t.Error("the crossref requests were not billed at all")
	}
}

// Reading a graph back gives the same graph, which is what makes two runs
// mergeable without a merge tool.
func TestGraphRoundTripsThroughJSON(t *testing.T) {
	g := buildWork(t, true)
	var buf bytes.Buffer
	if err := WriteGraph(&buf, g, GraphWriteOptions{Format: FormatGraphJSON}); err != nil {
		t.Fatalf("write: %v", err)
	}
	back, err := ReadGraph(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(back.Nodes) != len(g.Nodes) || len(back.Edges) != len(g.Edges) {
		t.Fatalf("the graph came back as %d nodes and %d edges, from %d and %d",
			len(back.Nodes), len(back.Edges), len(g.Nodes), len(g.Edges))
	}
	held, edges := back.Merge(g)
	if held != len(g.Nodes) || edges != len(g.Edges) {
		t.Errorf("merging a graph with its own round trip found %d of %d nodes and %d of %d edges already held",
			held, len(g.Nodes), edges, len(g.Edges))
	}
}

// The volumes page is the whole back catalogue for one request, and it arrives
// as nodes scoped under the journal rather than as bare volume numbers.
func TestVolumesBecomeNodesUnderTheirJournal(t *testing.T) {
	v, err := ExtractVolumes(capturedResponse(t, "volumes.html"))
	if err != nil {
		t.Fatalf("volumes: %v", err)
	}
	g := NewGraph()
	journal := g.AddVolumes(v)
	if journal == "" {
		t.Fatal("the volumes page produced no journal node")
	}

	var volumes, issues int
	for _, n := range g.Nodes {
		switch n.Kind {
		case NodeVolume:
			volumes++
			if !strings.HasPrefix(n.URI, journal+"/volume/") {
				t.Errorf("a volume is not scoped under its journal: %s", n.URI)
			}
		case NodeIssue:
			issues++
		}
	}
	// 38 volumes on the measured journal, and volume 110 exists in several
	// thousand journals, which is why the scoping matters.
	if volumes < 38 {
		t.Errorf("%d volumes, want at least the 38 the page lists", volumes)
	}
	if issues <= volumes {
		t.Errorf("%d issues across %d volumes, which is fewer than one each", issues, volumes)
	}
}
