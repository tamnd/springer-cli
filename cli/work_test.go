package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tamnd/springer-cli/spr"
)

// run executes one command line against the tree and returns what it wrote.
// Nothing here goes to the network: every command exercised in this file
// answers out of a table.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := Root()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestExtractionTable(t *testing.T) {
	out, err := run(t, "extraction")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"doi", "authors", "linkdata", "citation_title", "section[data-title]"} {
		if !strings.Contains(out, want) {
			t.Errorf("the table does not mention %q", want)
		}
	}
	// One row per field and one header row.
	if got, want := strings.Count(strings.TrimSpace(out), "\n")+1, len(spr.Fields)+1; got != want {
		t.Errorf("printed %d lines, want %d", got, want)
	}
}

// Asking about one field by name gets the long form, because the question was
// about that field and there is room to answer it properly.
func TestExtractionOneField(t *testing.T) {
	out, err := run(t, "extraction", "authors")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "rung    2, linkdata") {
		t.Errorf("the rung is not stated as rung 2:\n%s", out)
	}
	if !strings.Contains(out, "orcid, affiliation and email") {
		t.Errorf("the reason is missing:\n%s", out)
	}

	if _, err := run(t, "extraction", "not_a_field"); err == nil {
		t.Error("a field that is not in the table was accepted")
	}
}

func TestExtractionByRung(t *testing.T) {
	out, err := run(t, "extraction", "--rung", "selector")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "highwire") {
		t.Errorf("a rung 1 field came back from a rung 4 filter:\n%s", out)
	}
	if !strings.Contains(out, "equations") {
		t.Errorf("the rung 4 fields are missing:\n%s", out)
	}

	if _, err := run(t, "extraction", "--rung", "jsonld"); err == nil {
		t.Error("a rung name that does not exist was accepted")
	}
}

// Free and the world readable tag are two separate declarations and both are
// printed. Declared and empty is not the same as never declared, and neither is
// the same as declared no, so all three have to read differently.
func TestAccessLine(t *testing.T) {
	yes, no := true, false
	empty := ""

	cases := []struct {
		name string
		in   spr.Access
		want string
	}{
		{"nothing declared", spr.Access{}, ""},
		{"open", spr.Access{Free: &yes, Raw: "Yes", WorldReadable: &empty}, "free to read, access=Yes, world readable declared and empty"},
		{"restricted", spr.Access{Free: &no, Raw: "No", Statement: "This is a preview of subscription content"}, "not free to read, access=No, This is a preview of subscription content"},
	}
	for _, c := range cases {
		if got := accessLine(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWrap(t *testing.T) {
	got := wrap("one two three four five six", 9, "| ")
	want := "one two\n| three\n| four five\n| six"
	if got != want {
		t.Errorf("wrap gave\n%q\nwant\n%q", got, want)
	}
	if wrap("   ", 10, "") != "" {
		t.Error("a string of spaces wrapped to something")
	}
}

// A record printed twice is printed the same way twice, so that a diff between
// two runs means the page changed rather than a map iterated in a new order.
func TestPrintWorkIsStable(t *testing.T) {
	w := &spr.Work{
		DOI:   "10.1007/s10994-021-05946-3",
		Type:  "article",
		Title: "Aleatoric and epistemic uncertainty in machine learning",
		Envelope: spr.Envelope{
			Tier:   "html",
			Status: spr.StatusOK,
			Via: map[string]string{
				"doi":     "highwire:citation_doi",
				"title":   "highwire:citation_title",
				"authors": "linkdata:author[]",
			},
			Missed: []spr.Miss{{Field: "body", Why: "the publisher states access=No"}},
		},
	}

	var first bytes.Buffer
	printWork(&first, w, false, true)
	for i := 0; i < 5; i++ {
		var again bytes.Buffer
		printWork(&again, w, false, true)
		if again.String() != first.String() {
			t.Fatal("two runs of the same record printed different bytes")
		}
	}
	if !strings.Contains(first.String(), "missed     body: the publisher states access=No") {
		t.Errorf("the missed field is not reported:\n%s", first.String())
	}
}
