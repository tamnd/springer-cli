package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/springer-cli/spr"
)

// A walk that would spend an afternoon of somebody else's bandwidth says so
// first. --dry-run prints the bill and fetches nothing, and the bill is per
// depth because one total cannot tell a work from a book.
func TestGraphBillsPerDepth(t *testing.T) {
	out, err := run(t, "graph", "10.1007/978-3-031-28170-9", "--depth", "1", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	for _, want := range []string{"seed", "depth 0", "depth 1", "requests", "pace", "estimate", "tier"} {
		if !strings.Contains(out, want) {
			t.Errorf("the bill has no %q line:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "html only") {
		t.Errorf("the bill does not say which tier this walk reads:\n%s", out)
	}
}

// The threshold is the same twenty the rest of this tool uses, and above it the
// command stops rather than starting a crawl nobody agreed to.
func TestGraphStopsAboveTheThreshold(t *testing.T) {
	out, err := run(t, "graph", "10.1007/978-3-031-28170-9", "--depth", "1")
	if err == nil {
		t.Fatal("a walk of forty pages started without --yes")
	}
	if !strings.Contains(out, "--yes") {
		t.Errorf("the refusal does not say what to pass:\n%s", out)
	}
	if !strings.Contains(out, "requests") {
		t.Errorf("the refusal does not bill what it refused:\n%s", out)
	}
}

// A seed and its container is two requests, which is under the threshold and
// runs without being asked twice.
func TestGraphDoesNotAskAboutASmallWalk(t *testing.T) {
	out, err := run(t, "graph", "10.1007/s10994-021-05946-3", "--depth", "1", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if strings.Contains(out, "--yes") {
		t.Errorf("a two request walk was billed as though it needed permission:\n%s", out)
	}
}

// The flags that only mean something next to another flag say so rather than
// being ignored.
func TestGraphRefusesFlagsThatContradict(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"graph", "10994", "--projection", "coauthor"}, "--format gexf"},
		{[]string{"graph", "10994", "--dir", "/tmp/x"}, "--format csv"},
		{[]string{"graph", "10994", "--format", "rdfa"}, "unknown graph format"},
		{[]string{"graph", "10994", "--depth", "-1"}, "--depth"},
		{[]string{"graph"}, "a seed"},
	}
	for _, c := range cases {
		out, err := run(t, c.args...)
		if err == nil {
			t.Errorf("%v was accepted", c.args)
			continue
		}
		if !strings.Contains(out+err.Error(), c.want) {
			t.Errorf("%v: the error does not mention %q: %v", c.args, c.want, err)
		}
	}
}

// Merging two graphs is a job with nothing to fetch in it, so it does not need
// a seed and it never merges on anything but the uri.
func TestGraphMergesFilesWithoutASeed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "held.json")
	writeGraphFile(t, path)

	out, err := run(t, "graph", "--merge", path, "--merge", path)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var held spr.Graph
	if err := json.Unmarshal([]byte(jsonPart(out)), &held); err != nil {
		t.Fatalf("the merged graph is not json: %v\n%s", err, out)
	}
	// The same file twice is the same two nodes, because a merge is a set union
	// on content addressed uris.
	if len(held.Nodes) != 2 {
		t.Errorf("merging one file into itself made %d nodes, want 2", len(held.Nodes))
	}
	if !strings.Contains(out, "were already held") {
		t.Errorf("the merge did not say how many nodes joined:\n%s", out)
	}
}

// --format csv --dir writes the pair Neo4j imports, with its header syntax,
// rather than two tables down one pipe.
func TestGraphWritesTheCSVPair(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "held.json")
	writeGraphFile(t, path)

	outDir := filepath.Join(dir, "out")
	if _, err := run(t, "graph", "--merge", path, "--format", "csv", "--dir", outDir); err != nil {
		t.Fatalf("csv: %v", err)
	}
	nodes, err := os.ReadFile(filepath.Join(outDir, "nodes.csv"))
	if err != nil {
		t.Fatalf("nodes.csv: %v", err)
	}
	edges, err := os.ReadFile(filepath.Join(outDir, "edges.csv"))
	if err != nil {
		t.Fatalf("edges.csv: %v", err)
	}
	if !strings.HasPrefix(string(nodes), "uri:ID,:LABEL") {
		t.Errorf("nodes.csv does not carry the import header: %q", firstLine(string(nodes)))
	}
	if !strings.HasPrefix(string(edges), ":START_ID,:END_ID,:TYPE") {
		t.Errorf("edges.csv does not carry the import header: %q", firstLine(string(edges)))
	}
}

// Help is where somebody finds out that identity is not guessed here, so the
// two sentences that say it have to be in the help rather than only in a doc.
func TestGraphHelpSaysHowIdentityWorks(t *testing.T) {
	out, err := run(t, "graph", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ORCID", "--merge-names", "--resume", "--depth"} {
		if !strings.Contains(out, want) {
			t.Errorf("the help does not mention %q", want)
		}
	}
	// Every format is offered, because a format that exists and is not listed
	// is a format nobody finds.
	for _, f := range spr.GraphFormats {
		if !strings.Contains(out, string(f)) {
			t.Errorf("the help does not offer --format %s", f)
		}
	}
}

// writeGraphFile puts a small graph on disk in the one format that round trips.
func writeGraphFile(t *testing.T, path string) {
	t.Helper()
	g := spr.NewGraph()
	work := spr.WorkURI("10.1007/s10994-021-05946-3")
	person := spr.PersonORCIDURI("0000-0002-9944-4108")
	g.AddNode(spr.Node{URI: work, Kind: spr.NodeWork, Label: "Aleatoric and epistemic uncertainty in machine learning", Tier: "html"})
	g.AddNode(spr.Node{URI: person, Kind: spr.NodePerson, Label: "Eyke Hüllermeier", Tier: "html"})
	g.AddEdge(spr.Edge{From: work, To: person, Kind: spr.EdgeAuthoredBy, Position: 1, Tier: "html"})

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := spr.WriteGraph(f, g, spr.GraphWriteOptions{Format: spr.FormatGraphJSON}); err != nil {
		t.Fatal(err)
	}
}

// jsonPart returns the object in a run's output, which shares a buffer with the
// notes the command writes to stderr.
func jsonPart(out string) string {
	i := strings.Index(out, "{")
	j := strings.LastIndex(out, "}")
	if i < 0 || j < i {
		return ""
	}
	return out[i : j+1]
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
