package spr

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Writing a graph out in the shape the next tool along wants it.
//
// Ten formats look like a lot for one data structure and they are not
// interchangeable in practice. Gephi will not read N-Triples, rapper will not
// read GEXF, Neo4j wants two csv files with particular header syntax, and a
// person eyeballing twenty nodes wants Graphviz. The graph itself is the same
// in all of them, and everything below is a projection of the same node and
// edge lists with nothing added and nothing inferred.

// GraphFormat is one serialization.
type GraphFormat string

const (
	FormatGraphJSON  GraphFormat = "json"
	FormatGraphJSONL GraphFormat = "jsonl"
	FormatNTriples   GraphFormat = "nt"
	FormatNQuads     GraphFormat = "nq"
	FormatTurtle     GraphFormat = "ttl"
	FormatJSONLD     GraphFormat = "jsonld"
	FormatGraphML    GraphFormat = "graphml"
	FormatGEXF       GraphFormat = "gexf"
	FormatDOT        GraphFormat = "dot"
	FormatCSV        GraphFormat = "csv"
)

// GraphFormats is every format, in the order the reference table lists them.
var GraphFormats = []GraphFormat{
	FormatGraphJSON, FormatGraphJSONL, FormatNTriples, FormatNQuads,
	FormatTurtle, FormatJSONLD, FormatGraphML, FormatGEXF, FormatDOT, FormatCSV,
}

// ParseGraphFormat reads a --format value.
func ParseGraphFormat(s string) (GraphFormat, error) {
	f := GraphFormat(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range GraphFormats {
		if k == f {
			return k, nil
		}
	}
	names := make([]string, len(GraphFormats))
	for i, k := range GraphFormats {
		names[i] = string(k)
	}
	return "", fmt.Errorf("unknown graph format %q, want one of %s", s, strings.Join(names, ", "))
}

// GraphWriteOptions is everything the serializers need beyond the graph.
type GraphWriteOptions struct {
	Format GraphFormat

	// Projection is coauthor or empty. It is computed on the way out and is
	// never stored, because materializing co-authorship would square the author
	// list for no information gained.
	Projection string

	// Dir sends the csv pair to two files in a directory. With it empty both go
	// to the writer one after the other, each behind a line naming it, which is
	// enough for awk and honest about there being two tables.
	Dir string
}

// WriteGraph writes a graph in one format.
func WriteGraph(w io.Writer, g *Graph, opt GraphWriteOptions) error {
	if g == nil {
		return fmt.Errorf("no graph to write")
	}
	// Every format goes out sorted, so that two runs that found the same things
	// produce the same bytes and a diff of them is readable.
	g.Sort()

	switch opt.Format {
	case "", FormatGraphJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	case FormatGraphJSONL:
		return writeJSONL(w, g)
	case FormatNTriples:
		return writeTriples(w, g, false)
	case FormatNQuads:
		return writeTriples(w, g, true)
	case FormatTurtle:
		return writeTurtle(w, g)
	case FormatJSONLD:
		return writeJSONLD(w, g)
	case FormatGraphML:
		return writeGraphML(w, g)
	case FormatGEXF:
		return writeGEXF(w, g, opt.Projection)
	case FormatDOT:
		return writeDOT(w, g)
	case FormatCSV:
		return writeCSV(w, g, opt.Dir)
	}
	return fmt.Errorf("unknown graph format %q", opt.Format)
}

// The vocabulary.
//
// RDF output does not invent a term where a standard one exists, and the whole
// mapping is in the three tables below rather than spread through the writers.
// Four local terms is the budget and the budget is kept: spr:recommends,
// spr:springerId, spr:accessStatement and spr:mergedFrom. A vocabulary that is
// mostly local is a vocabulary nobody else can read.

// SprNamespace is what the spr prefix expands to.
const SprNamespace = "https://springer-cli.tamnd.com/ns/"

// prefixes are emitted inline by Turtle and JSON-LD so that a file is self
// describing and does not depend on a hosted context document that may not
// exist in five years.
var prefixes = [][2]string{
	{"spr", SprNamespace},
	{"schema", "http://schema.org/"},
	{"dcterms", "http://purl.org/dc/terms/"},
	{"prism", "http://prismstandard.org/namespaces/basic/2.0/"},
	{"bibo", "http://purl.org/ontology/bibo/"},
	{"fabio", "http://purl.org/spar/fabio/"},
	{"cito", "http://purl.org/spar/cito/"},
	{"frapo", "http://purl.org/cerif/frapo/"},
	{"owl", "http://www.w3.org/2002/07/owl#"},
	{"rdf", "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
	{"rdfs", "http://www.w3.org/2000/01/rdf-schema#"},
}

// nodeClasses is the rdf:type of each kind. A work is two classes because the
// two vocabularies say different useful things about it and neither subsumes
// the other.
var nodeClasses = map[NodeKind][]string{
	NodeWork:       {"schema:ScholarlyArticle", "fabio:Expression"},
	NodeJournal:    {"schema:Periodical", "fabio:Journal"},
	NodeBook:       {"schema:Book", "fabio:Book"},
	NodeSeries:     {"fabio:BookSeries"},
	NodeVolume:     {"fabio:JournalVolume"},
	NodeIssue:      {"fabio:JournalIssue"},
	NodePerson:     {"schema:Person"},
	NodeOrg:        {"schema:Organization"},
	NodeFunder:     {"frapo:FundingAgency"},
	NodeConference: {"bibo:Conference"},
	NodeSubject:    {"schema:DefinedTerm"},
	NodePublisher:  {"schema:Organization"},
}

// edgePredicates maps each of the fifteen edges to a term.
//
// Four containment edges share dcterms:isPartOf, which is not a loss: the
// object's own rdf:type says whether a work is inside a journal, a volume, an
// issue or a series, so the distinction survives in RDF without four local
// terms being minted to carry what the type already carries.
var edgePredicates = map[EdgeKind]string{
	EdgePartOf:         "dcterms:isPartOf",
	EdgeInVolume:       "dcterms:isPartOf",
	EdgeInIssue:        "dcterms:isPartOf",
	EdgeInSeries:       "dcterms:isPartOf",
	EdgePresentedAt:    "bibo:presentedAt",
	EdgeAuthoredBy:     "dcterms:creator",
	EdgeEditedBy:       "schema:editor",
	EdgeAffiliatedWith: "schema:affiliation",
	EdgeCites:          "cito:cites",
	EdgeCitedBy:        "cito:isCitedBy",
	EdgeFundedBy:       "frapo:isFundedBy",
	EdgeHasSubject:     "dcterms:subject",
	EdgePublishedBy:    "dcterms:publisher",
	EdgeRecommends:     "spr:recommends",
	EdgeSameAs:         "owl:sameAs",
}

// propPredicates maps a node property to a term, and the ones that are absent
// from this map are absent from RDF.
//
// Dropping a property rather than minting spr:oaStatus for it is the deliberate
// half of keeping the local vocabulary at four terms. Everything the graph
// holds is in --format json, and the RDF is the subset that means the same
// thing to somebody who has never seen this tool.
var propPredicates = map[string]string{
	"doi":              "prism:doi",
	"issn":             "prism:issn",
	"electronic_issn":  "prism:issn",
	"print_issn":       "prism:issn",
	"isbn":             "prism:isbn",
	"url":              "schema:url",
	"published":        "schema:datePublished",
	"type":             "dcterms:type",
	"language":         "dcterms:language",
	"name":             "schema:name",
	"orcid":            "schema:identifier",
	"ror":              "schema:identifier",
	"country":          "schema:addressCountry",
	"free":             "schema:isAccessibleForFree",
	"access_statement": "spr:accessStatement",
	"springer_id":      "spr:springerId",
	"merged_from":      MergedFromPredicate,
}

// MergedFromPredicate is the fourth local term. It is written by an explicit name merge
// and by nothing else, so that a graph where two nodes were joined on a name
// says so on the node rather than in a log line nobody kept.
const MergedFromPredicate = "spr:mergedFrom"

// writeJSONL writes one node or edge per line.
func writeJSONL(w io.Writer, g *Graph) error {
	enc := json.NewEncoder(w)
	for _, n := range g.Nodes {
		if err := enc.Encode(struct {
			Rec string `json:"rec"`
			Node
		}{"node", n}); err != nil {
			return err
		}
	}
	for _, e := range g.Edges {
		if err := enc.Encode(struct {
			Rec string `json:"rec"`
			Edge
		}{"edge", e}); err != nil {
			return err
		}
	}
	return nil
}

// writeTriples writes N-Triples, or N-Quads when tiers is set.
//
// Provenance in RDF goes on a named graph per tier rather than on a reification
// of every triple. A consumer that does not care drops the fourth column and
// has plain triples back; a consumer that does can ask which tier asserted an
// edge without parsing anything.
func writeTriples(w io.Writer, g *Graph, quads bool) error {
	bw := &errWriter{w: w}
	for _, n := range g.Nodes {
		gr := graphTerm(n.Tier, quads)
		for _, class := range nodeClasses[n.Kind] {
			bw.line(iri(n.URI), expand("rdf:type"), expand(class), gr)
		}
		if n.Label != "" {
			bw.line(iri(n.URI), expand("rdfs:label"), literal(n.Label), gr)
		}
		for _, k := range sortedStringKeys(n.Props) {
			p, ok := propPredicates[k]
			if !ok {
				continue
			}
			// merged_from points at the uris a name merge folded in, so it is
			// the one property whose object is a node and not a string.
			if k == "merged_from" {
				for _, from := range strings.Fields(n.Props[k]) {
					bw.line(iri(n.URI), expand(p), iri(from), gr)
				}
				continue
			}
			bw.line(iri(n.URI), expand(p), propObject(k, n.Props[k]), gr)
		}
	}
	for _, e := range g.Edges {
		p, ok := edgePredicates[e.Kind]
		if !ok {
			continue
		}
		bw.line(iri(e.From), expand(p), iri(e.To), graphTerm(e.Tier, quads))
	}
	for _, l := range authorLists(g) {
		gr := graphTerm(l.tier, quads)
		bw.line(iri(l.work), expand("bibo:authorList"), blank(l.nodes[0]), gr)
		for i, b := range l.nodes {
			bw.line(blank(b), expand("rdf:first"), iri(l.people[i]), gr)
			if i+1 < len(l.nodes) {
				bw.line(blank(b), expand("rdf:rest"), blank(l.nodes[i+1]), gr)
			} else {
				bw.line(blank(b), expand("rdf:rest"), expand("rdf:nil"), gr)
			}
		}
	}
	return bw.err
}

// writeTurtle writes the same statements grouped by subject.
func writeTurtle(w io.Writer, g *Graph) error {
	bw := &errWriter{w: w}
	for _, p := range prefixes {
		bw.printf("@prefix %s: <%s> .\n", p[0], p[1])
	}
	bw.printf("\n")

	for _, n := range g.Nodes {
		var lines []string
		if classes := nodeClasses[n.Kind]; len(classes) > 0 {
			lines = append(lines, "a "+strings.Join(classes, ", "))
		}
		if n.Label != "" {
			lines = append(lines, "rdfs:label "+literal(n.Label))
		}
		for _, k := range sortedStringKeys(n.Props) {
			p, ok := propPredicates[k]
			if !ok {
				continue
			}
			if k == "merged_from" {
				for _, from := range strings.Fields(n.Props[k]) {
					lines = append(lines, p+" "+turtleIRI(from))
				}
				continue
			}
			lines = append(lines, p+" "+propObject(k, n.Props[k]))
		}
		for _, e := range g.Edges {
			if e.From != n.URI {
				continue
			}
			if p, ok := edgePredicates[e.Kind]; ok {
				lines = append(lines, p+" "+turtleIRI(e.To))
			}
		}
		if len(lines) == 0 {
			continue
		}
		bw.printf("%s\n    %s .\n\n", turtleIRI(n.URI), strings.Join(lines, " ;\n    "))
	}

	// The ordered author lists last, because a Turtle collection reads better on
	// its own than folded into the work's block.
	for _, l := range authorLists(g) {
		people := make([]string, len(l.people))
		for i, p := range l.people {
			people[i] = turtleIRI(p)
		}
		bw.printf("%s bibo:authorList ( %s ) .\n", turtleIRI(l.work), strings.Join(people, " "))
	}
	return bw.err
}

// writeJSONLD writes one document with its context inline.
func writeJSONLD(w io.Writer, g *Graph) error {
	ctx := map[string]any{}
	for _, p := range prefixes {
		ctx[p[0]] = p[1]
	}
	// The two terms that need more than a prefix: an author list is ordered and
	// says so in the context rather than in every document that uses it.
	ctx["authorList"] = map[string]any{"@id": "bibo:authorList", "@container": "@list"}
	ctx["label"] = "rdfs:label"

	out := make([]map[string]any, 0, len(g.Nodes))
	lists := map[string][]string{}
	for _, l := range authorLists(g) {
		lists[l.work] = l.people
	}
	for _, n := range g.Nodes {
		obj := map[string]any{"@id": n.URI}
		if classes := nodeClasses[n.Kind]; len(classes) > 0 {
			obj["@type"] = classes
		}
		if n.Label != "" {
			obj["label"] = n.Label
		}
		for _, k := range sortedStringKeys(n.Props) {
			if p, ok := propPredicates[k]; ok {
				obj[p] = jsonldValue(k, n.Props[k])
			}
		}
		for _, e := range g.Edges {
			if e.From != n.URI {
				continue
			}
			p, ok := edgePredicates[e.Kind]
			if !ok {
				continue
			}
			ref := map[string]any{"@id": e.To}
			switch held := obj[p].(type) {
			case nil:
				obj[p] = ref
			case map[string]any:
				obj[p] = []any{held, ref}
			case []any:
				obj[p] = append(held, ref)
			}
		}
		if people := lists[n.URI]; len(people) > 0 {
			refs := make([]any, len(people))
			for i, p := range people {
				refs[i] = map[string]any{"@id": p}
			}
			obj["authorList"] = refs
		}
		out = append(out, obj)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"@context": ctx, "@graph": out})
}

// writeGraphML writes what Gephi and yEd read.
//
// GraphML numbers nodes per file, which would give the same node two different
// ids in two runs and make the files unmergeable. The id here is derived from
// the uri hash instead, so two runs agree without having seen each other.
func writeGraphML(w io.Writer, g *Graph) error {
	bw := &errWriter{w: w}
	bw.printf("%s", xmlHeader)
	bw.printf("<graphml xmlns=\"http://graphml.graphdrawing.org/xmlns\">\n")
	for _, k := range []string{"uri", "kind", "label", "via", "tier"} {
		bw.printf("  <key id=\"n_%s\" for=\"node\" attr.name=\"%s\" attr.type=\"string\"/>\n", k, k)
	}
	for _, k := range []string{"type", "via", "tier", "sequence", "role"} {
		bw.printf("  <key id=\"e_%s\" for=\"edge\" attr.name=\"%s\" attr.type=\"string\"/>\n", k, k)
	}
	bw.printf("  <key id=\"e_position\" for=\"edge\" attr.name=\"position\" attr.type=\"int\"/>\n")
	bw.printf("  <graph id=\"spr\" edgedefault=\"directed\">\n")
	for _, n := range g.Nodes {
		bw.printf("    <node id=\"%s\">\n", nodeID(n.URI))
		graphMLData(bw, "n_uri", n.URI)
		graphMLData(bw, "n_kind", string(n.Kind))
		graphMLData(bw, "n_label", n.Label)
		graphMLData(bw, "n_via", n.Via)
		graphMLData(bw, "n_tier", n.Tier)
		bw.printf("    </node>\n")
	}
	for i, e := range g.Edges {
		bw.printf("    <edge id=\"e%d\" source=\"%s\" target=\"%s\">\n", i, nodeID(e.From), nodeID(e.To))
		graphMLData(bw, "e_type", string(e.Kind))
		if e.Position > 0 {
			graphMLData(bw, "e_position", strconv.Itoa(e.Position))
		}
		graphMLData(bw, "e_sequence", e.Sequence)
		graphMLData(bw, "e_role", e.Role)
		graphMLData(bw, "e_via", e.Via)
		graphMLData(bw, "e_tier", e.Tier)
		bw.printf("    </edge>\n")
	}
	bw.printf("  </graph>\n</graphml>\n")
	return bw.err
}

func graphMLData(bw *errWriter, key, val string) {
	if val == "" {
		return
	}
	bw.printf("      <data key=\"%s\">%s</data>\n", key, xmlEscape(val))
}

// writeGEXF writes Gephi's own format, and is where the projection happens.
func writeGEXF(w io.Writer, g *Graph, projection string) error {
	nodes, edges := g.Nodes, g.Edges
	weights := map[int]int{}
	switch strings.ToLower(strings.TrimSpace(projection)) {
	case "", "none":
	case "coauthor":
		nodes, edges, weights = coauthorProjection(g)
	default:
		return fmt.Errorf("unknown projection %q, want coauthor", projection)
	}

	bw := &errWriter{w: w}
	bw.printf("%s", xmlHeader)
	bw.printf("<gexf xmlns=\"http://gexf.net/1.3\" version=\"1.3\">\n")
	bw.printf("  <graph mode=\"static\" defaultedgetype=\"directed\">\n")
	bw.printf("    <attributes class=\"node\">\n")
	for i, k := range []string{"uri", "kind", "via", "tier"} {
		bw.printf("      <attribute id=\"%d\" title=\"%s\" type=\"string\"/>\n", i, k)
	}
	bw.printf("    </attributes>\n")
	bw.printf("    <nodes>\n")
	for _, n := range nodes {
		bw.printf("      <node id=\"%s\" label=\"%s\">\n", nodeID(n.URI), xmlEscape(firstNonEmpty(n.Label, n.URI)))
		bw.printf("        <attvalues>\n")
		for i, v := range []string{n.URI, string(n.Kind), n.Via, n.Tier} {
			if v == "" {
				continue
			}
			bw.printf("          <attvalue for=\"%d\" value=\"%s\"/>\n", i, xmlEscape(v))
		}
		bw.printf("        </attvalues>\n      </node>\n")
	}
	bw.printf("    </nodes>\n    <edges>\n")
	for i, e := range edges {
		bw.printf("      <edge id=\"%d\" source=\"%s\" target=\"%s\" label=\"%s\"", i, nodeID(e.From), nodeID(e.To), xmlEscape(string(e.Kind)))
		if wgt, ok := weights[i]; ok {
			bw.printf(" weight=\"%d\"", wgt)
		}
		bw.printf("/>\n")
	}
	bw.printf("    </edges>\n  </graph>\n</gexf>\n")
	return bw.err
}

// coauthorProjection computes co-authorship on the way out.
//
// It is not stored because it is derivable and materializing it multiplies the
// edge count by the square of the author list. Computing it here costs nothing
// and the tools that want it precomputed get it, with the weight being how many
// works two people share.
func coauthorProjection(g *Graph) ([]Node, []Edge, map[int]int) {
	byWork := map[string][]string{}
	for _, e := range g.Edges {
		if e.Kind == EdgeAuthoredBy {
			byWork[e.From] = append(byWork[e.From], e.To)
		}
	}
	shared := map[[2]string]int{}
	for _, people := range byWork {
		sort.Strings(people)
		for i := 0; i < len(people); i++ {
			for j := i + 1; j < len(people); j++ {
				if people[i] == people[j] {
					continue
				}
				shared[[2]string{people[i], people[j]}]++
			}
		}
	}

	keys := make([][2]string, 0, len(shared))
	kept := map[string]bool{}
	for k := range shared {
		keys = append(keys, k)
		kept[k[0]], kept[k[1]] = true, true
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})

	var nodes []Node
	for _, n := range g.Nodes {
		if kept[n.URI] {
			nodes = append(nodes, n)
		}
	}
	edges := make([]Edge, 0, len(keys))
	weights := map[int]int{}
	for i, k := range keys {
		edges = append(edges, Edge{From: k[0], To: k[1], Kind: "coauthorWith", Via: "projection:coauthor"})
		weights[i] = shared[k]
	}
	return nodes, edges, weights
}

// writeDOT writes Graphviz, which is for looking at rather than for loading.
func writeDOT(w io.Writer, g *Graph) error {
	bw := &errWriter{w: w}
	bw.printf("digraph spr {\n  rankdir=LR;\n  node [shape=box, fontsize=10];\n")
	for _, n := range g.Nodes {
		label := firstNonEmpty(n.Label, n.URI)
		bw.printf("  %q [label=%q, tooltip=%q];\n", n.URI, wrapLabel(label, 32), n.URI)
	}
	for _, e := range g.Edges {
		label := string(e.Kind)
		if e.Position > 0 {
			label = fmt.Sprintf("%s %d", label, e.Position)
		}
		bw.printf("  %q -> %q [label=%q, fontsize=8];\n", e.From, e.To, label)
	}
	bw.printf("}\n")
	return bw.err
}

// writeCSV writes the pair Neo4j imports, with its header syntax.
func writeCSV(w io.Writer, g *Graph, dir string) error {
	nodes := func(out io.Writer) error {
		c := csv.NewWriter(out)
		if err := c.Write([]string{"uri:ID", ":LABEL", "label", "props", "via", "tier", "fetched:datetime"}); err != nil {
			return err
		}
		for _, n := range g.Nodes {
			props := ""
			if len(n.Props) > 0 {
				b, err := json.Marshal(n.Props)
				if err != nil {
					return err
				}
				props = string(b)
			}
			if err := c.Write([]string{n.URI, string(n.Kind), n.Label, props, n.Via, n.Tier, stamp(n.Fetched)}); err != nil {
				return err
			}
		}
		c.Flush()
		return c.Error()
	}
	edges := func(out io.Writer) error {
		c := csv.NewWriter(out)
		if err := c.Write([]string{":START_ID", ":END_ID", ":TYPE", "position:int", "sequence", "role", "via", "tier", "fetched:datetime"}); err != nil {
			return err
		}
		for _, e := range g.Edges {
			pos := ""
			if e.Position > 0 {
				pos = strconv.Itoa(e.Position)
			}
			if err := c.Write([]string{e.From, e.To, string(e.Kind), pos, e.Sequence, e.Role, e.Via, e.Tier, stamp(e.Fetched)}); err != nil {
				return err
			}
		}
		c.Flush()
		return c.Error()
	}

	if dir == "" {
		if _, err := fmt.Fprintln(w, "# nodes.csv"); err != nil {
			return err
		}
		if err := nodes(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "\n# edges.csv"); err != nil {
			return err
		}
		return edges(w)
	}
	if err := writeFileIn(dir, "nodes.csv", nodes); err != nil {
		return err
	}
	return writeFileIn(dir, "edges.csv", edges)
}

// stamp is a fetch time in the one format every importer here reads, and the
// empty string for a zero time rather than the year 1.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// writeFileIn creates one of the csv pair, making the directory if it is not
// there. The two files are written whole rather than streamed because a half
// written nodes.csv next to a complete edges.csv is worse than an error.
func writeFileIn(dir, name string, fn func(io.Writer) error) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		// The write error is the one worth reporting, so the close that follows
		// it is only here to give the descriptor back.
		_ = f.Close()
		return err
	}
	return f.Close()
}

// authorList is one work's ordered author list, kept as a list rather than as
// fifteen unordered creator triples.
//
// RDF has no ordered property, so the order goes into an rdf:List under
// bibo:authorList. That is the standard answer and it costs no local term,
// where reifying every authoredBy statement to hang a position on it would cost
// one and would be harder to query.
type authorList struct {
	work   string
	people []string
	nodes  []string
	tier   string
}

func authorLists(g *Graph) []authorList {
	byWork := map[string][]Edge{}
	var order []string
	for _, e := range g.Edges {
		if e.Kind != EdgeAuthoredBy || e.Position == 0 {
			continue
		}
		if _, seen := byWork[e.From]; !seen {
			order = append(order, e.From)
		}
		byWork[e.From] = append(byWork[e.From], e)
	}
	sort.Strings(order)

	var out []authorList
	for _, work := range order {
		es := byWork[work]
		sort.SliceStable(es, func(i, j int) bool { return es[i].Position < es[j].Position })
		l := authorList{work: work, tier: es[0].Tier}
		for i, e := range es {
			l.people = append(l.people, e.To)
			// The blank node label is derived from the work so that two runs
			// that read the same page write the same labels, which is what
			// makes cat and sort -u a correct merge.
			l.nodes = append(l.nodes, fmt.Sprintf("l%s%d", nameHash(work)[:12], i))
		}
		out = append(out, l)
	}
	return out
}

// The term writers.

func expand(curie string) string {
	name, rest, ok := strings.Cut(curie, ":")
	if !ok {
		return "<" + curie + ">"
	}
	for _, p := range prefixes {
		if p[0] == name {
			return "<" + p[1] + rest + ">"
		}
	}
	return "<" + curie + ">"
}

// iri expands a node uri, which is always in the spr namespace.
func iri(uri string) string {
	if rest, ok := strings.CutPrefix(uri, URIPrefix); ok {
		return "<" + SprNamespace + escapeIRI(rest) + ">"
	}
	return "<" + escapeIRI(uri) + ">"
}

// turtleIRI keeps the prefixed form, which is the whole reason to write Turtle.
func turtleIRI(uri string) string {
	if rest, ok := strings.CutPrefix(uri, URIPrefix); ok {
		// A DOI suffix can hold characters a prefixed name cannot, so anything
		// that is not plainly safe goes out in full angle brackets.
		if safeLocalName(rest) {
			return "spr:" + rest
		}
		return "<" + SprNamespace + escapeIRI(rest) + ">"
	}
	return "<" + escapeIRI(uri) + ">"
}

func safeLocalName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_' || r == '/':
		default:
			return false
		}
	}
	return !strings.HasSuffix(s, ".")
}

func escapeIRI(s string) string {
	r := strings.NewReplacer(" ", "%20", "<", "%3C", ">", "%3E", "\"", "%22", "{", "%7B", "}", "%7D", "|", "%7C", "\\", "%5C", "^", "%5E", "`", "%60")
	return r.Replace(s)
}

func blank(label string) string { return "_:" + label }

func literal(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t")
	return "\"" + r.Replace(s) + "\""
}

// propObject types the two values where the type is the point. Everything else
// is a plain string, because a graph is not the place to guess at datatypes.
func propObject(key, val string) string {
	switch key {
	case "free":
		return literal(val) + "^^" + expand("http://www.w3.org/2001/XMLSchema#boolean")
	case "published":
		return literal(val) + "^^" + expand("http://www.w3.org/2001/XMLSchema#date")
	}
	return literal(val)
}

func jsonldValue(key, val string) any {
	switch key {
	case "merged_from":
		var out []any
		for _, from := range strings.Fields(val) {
			out = append(out, map[string]any{"@id": from})
		}
		return out
	case "free":
		return val == "true"
	case "published":
		return map[string]any{"@value": val, "@type": "http://www.w3.org/2001/XMLSchema#date"}
	}
	return val
}

// graphTerm is the fourth column of a quad, and the empty string in N-Triples.
// A node or edge with no tier goes in the default graph, which is where a
// projection and a merge note belong.
func graphTerm(tier string, quads bool) string {
	if !quads || tier == "" {
		return ""
	}
	return "<" + SprNamespace + "tier/" + urlSegment(tier) + ">"
}

// nodeID is the id the two xml formats number their nodes with, derived from
// the uri so that two runs agree.
func nodeID(uri string) string {
	h := nameHash(uri)
	if h == "" {
		return "n0"
	}
	return "n" + h[:16]
}

func sortedStringKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return r.Replace(s)
}

const xmlHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"

// wrapLabel breaks a long label over lines so that a Graphviz box stays a box.
func wrapLabel(s string, width int) string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// errWriter keeps the writers above readable by holding the first error instead
// of returning one from every line.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, args ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, args...)
}

func (e *errWriter) line(s, p, o, g string) {
	if g == "" {
		e.printf("%s %s %s .\n", s, p, o)
		return
	}
	e.printf("%s %s %s %s .\n", s, p, o, g)
}
