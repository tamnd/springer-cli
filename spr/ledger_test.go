package spr

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCaptureLedger runs the reading in ledger.go over each of the fourteen
// stored captures and compares it with testdata/capture.txt. The comparison has
// outcomes that are deliberately not the same:
//
//   - Fewer fields set, or more missed. A regression. Fails.
//   - More fields set. An improvement, and it fails until the ledger is
//     updated, so that an improvement is always a deliberate reviewed change
//     rather than something that happened.
//   - Regions unread changed. Drift. Reported, does not fail, because Springer
//     shipping a new component is news about the site and not a bug in this
//     tool.
//
// Run with -update to rewrite the ledger after a change you meant to make.
var update = flag.Bool("update", false, "rewrite testdata/capture.txt from the current extractor")

const ledgerPath = "testdata/capture.txt"

// read runs the extractor over one capture, failing the test rather than
// returning an error, which is the only difference between this and what spr
// verify does with the same function.
func read(t *testing.T, c Capture) LedgerEntry {
	t.Helper()
	e, err := ReadCapture(load(t, c), c)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCaptureLedger(t *testing.T) {
	var got strings.Builder
	got.WriteString(LedgerHeader)
	entries := make([]LedgerEntry, 0, len(Captures))
	for _, c := range Captures {
		e := read(t, c)
		entries = append(entries, e)
		got.WriteString("\n")
		got.WriteString(e.String())
	}

	if *update {
		if err := os.WriteFile(filepath.Clean(ledgerPath), []byte(got.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("ledger rewritten; read the diff before committing it")
		return
	}

	want, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("%v (run go test -run TestCaptureLedger -update to create it)", err)
	}
	if string(want) == got.String() {
		return
	}

	// The ledger moved. Say which way, because a field appearing and a field
	// disappearing are different pieces of news and only one of them is a bug.
	recorded := ParseLedger(string(want))
	failed := false
	for _, e := range entries {
		w, ok := recorded[e.Name]
		if !ok {
			t.Errorf("%s: not in the ledger", e.Name)
			failed = true
			continue
		}
		d := CompareLedger(w, e)
		switch d.Verdict() {
		case VerdictOK:
		case VerdictDrift:
			t.Logf("drift: %s: %s", e.Name, strings.Join(d.Lines(), ", "))
		default:
			failed = true
			for _, line := range d.Lines() {
				t.Errorf("%s: %s", e.Name, line)
			}
		}
	}
	if failed {
		t.Errorf("the ledger and the extractor disagree; rerun with -update once the differences above are understood")
		return
	}

	// Every entry graded ok or drift and the bytes still differ, which means the
	// prose at the top moved or a capture left the table. That is a real
	// difference, and the ledger is a file people read in a diff, so it is not
	// waved through.
	t.Errorf("the ledger text changed without any entry regressing; rerun with -update to record it")
}

// The ledger this binary carries has to be the one on disk, because spr verify
// reads the embedded copy and TestCaptureLedger reads the file, and a check
// that quietly compares against a stale copy of itself is worse than no check.
func TestTheEmbeddedLedgerIsTheFileOnDisk(t *testing.T) {
	onDisk, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != ledgerFile {
		t.Error("the embedded ledger and testdata/capture.txt differ, which cannot happen from a clean build")
	}
	if len(Ledger()) != len(Captures) {
		t.Errorf("the ledger holds %d entries for %d captures", len(Ledger()), len(Captures))
	}
}

// A verdict is graded on what changed rather than on how much, and a reading
// that both gained and lost a field is a regression rather than a draw.
func TestAVerdictWeighsALossOverAGain(t *testing.T) {
	want := LedgerEntry{Fields: []string{"title", "doi"}, Unread: 4}
	got := LedgerEntry{Fields: []string{"title", "abstract"}, Unread: 4}

	d := CompareLedger(want, got)
	if v := d.Verdict(); v != VerdictRegression {
		t.Errorf("verdict = %q, want %q", v, VerdictRegression)
	}
	if len(d.Lost) != 1 || d.Lost[0] != "doi" {
		t.Errorf("lost = %v, want [doi]", d.Lost)
	}
	if len(d.Gained) != 1 || d.Gained[0] != "abstract" {
		t.Errorf("gained = %v, want [abstract]", d.Gained)
	}

	// Unread on its own is drift and never a failure.
	drift := CompareLedger(LedgerEntry{Unread: 4}, LedgerEntry{Unread: 5})
	if v := drift.Verdict(); v != VerdictDrift {
		t.Errorf("verdict = %q, want %q", v, VerdictDrift)
	}

	// A vocabulary disappearing outranks a field arriving, because a page that
	// dropped Dublin Core is about to empty a field nobody is watching.
	moved := CompareLedger(
		LedgerEntry{Vocabularies: "highwire+dc+prism", Fields: []string{"title"}},
		LedgerEntry{Vocabularies: "highwire+prism", Fields: []string{"title", "abstract"}},
	)
	if v := moved.Verdict(); v != VerdictChanged {
		t.Errorf("verdict = %q, want %q", v, VerdictChanged)
	}
}

// The vocabularies agree on every capture, which is the finding, so the check
// reports the agreements rather than printing an empty list that looks the same
// whether it ran or not.
func TestVocabularyReadingReportsAgreements(t *testing.T) {
	c, ok := CaptureNamed("article_oa")
	if !ok {
		t.Fatal("article_oa is not in the capture table")
	}
	rows, err := ReadVocabularies(load(t, c))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 5 {
		t.Fatalf("%d facts are stated by more than one vocabulary, want several", len(rows))
	}
	var access bool
	for _, r := range rows {
		if !r.Agree {
			t.Errorf("%s disagrees across vocabularies: %v", r.Fact, r.Claims)
		}
		if len(r.Claims) < 2 {
			t.Errorf("%s was reported with %d claim, which is not a cross check", r.Fact, len(r.Claims))
		}
		if r.Fact == "access" {
			access = true
		}
	}
	if !access {
		t.Error("the two access declarations were not compared, which is the one people care about")
	}
}

// Naming a capture works with or without the extension, because somebody
// passing --capture article_oa should not have to know it is stored as html.
func TestCapturesAreNamedWithOrWithoutTheExtension(t *testing.T) {
	a, ok := CaptureNamed("article_oa")
	if !ok {
		t.Fatal("article_oa did not resolve")
	}
	b, ok := CaptureNamed("article_oa.html")
	if !ok {
		t.Fatal("article_oa.html did not resolve")
	}
	if a != b {
		t.Errorf("the two spellings resolved to %v and %v", a, b)
	}
	if _, ok := CaptureNamed("no_such_page"); ok {
		t.Error("a name that is not in the table resolved anyway")
	}
}
