package spr

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The capture ledger.
//
// testdata/capture.txt records, per capture, which fields the extractor set,
// which it named as missed, how many regions it left unread, and the three
// site specific signals. TestCaptureLedger runs the extractor over all nine
// captures and compares. The comparison has three outcomes and they are
// deliberately not the same:
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

// ledgerEntry is one capture's line in the ledger, parsed back.
type ledgerEntry struct {
	name    string
	fields  []string
	missed  []string
	unread  int
	vocabs  string
	jsonld  int
	agreed  string
	metas   int
	regions int
}

// report runs the extractor over one capture and describes what came out.
func report(t *testing.T, c capture) ledgerEntry {
	t.Helper()
	resp := load(t, c)
	e := ledgerEntry{name: c.file}

	doc, err := parseDoc(resp.Body)
	if err != nil {
		t.Fatalf("%s: %v", c.file, err)
	}
	meta := ParseMeta(doc)
	ld := parseLinkData(doc)
	reg := parseRegions(doc)

	e.metas = len(meta.Names())
	e.regions = len(reg.names())
	e.jsonld = ld.count()

	var vocabs []string
	for _, v := range meta.Vocabularies() {
		vocabs = append(vocabs, string(v))
	}
	e.vocabs = strings.Join(vocabs, "+")
	if e.vocabs == "" {
		e.vocabs = "none"
	}

	// The access declaration is stated twice on a work page, once in Highwire
	// and once in schema.org. They agreed on all nine captures, which is the
	// reason a disagreement is worth watching for.
	e.agreed = accessAgreement(meta, ld)

	if c.kind == "" {
		e.unread = -1
		return e
	}

	w, err := ExtractWork(resp)
	if err != nil {
		t.Fatalf("%s: %v", c.file, err)
	}
	if w.Type != c.kind {
		t.Errorf("%s: extracted as %q, want %q", c.file, w.Type, c.kind)
	}
	for f := range w.Envelope.Via {
		e.fields = append(e.fields, f)
	}
	sort.Strings(e.fields)
	for _, m := range w.Envelope.Missed {
		e.missed = append(e.missed, m.Field)
	}
	sort.Strings(e.missed)
	e.unread = len(w.Envelope.Unread)
	return e
}

// accessAgreement compares the two independent access declarations.
func accessAgreement(m *Meta, ld *linkData) string {
	raw := m.First("access")
	work := ld.work()
	var free *bool
	if work != nil {
		free = work.IsAccessibleForFree
	}
	switch {
	case raw == "" && free == nil:
		return "neither"
	case raw == "" || free == nil:
		return "one only"
	case strings.EqualFold(raw, "yes") == *free:
		return "agree"
	default:
		return "DISAGREE"
	}
}

func (e ledgerEntry) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", e.name)
	fmt.Fprintf(&b, "  meta names   %d\n", e.metas)
	fmt.Fprintf(&b, "  json-ld      %d\n", e.jsonld)
	fmt.Fprintf(&b, "  vocabularies %s\n", e.vocabs)
	fmt.Fprintf(&b, "  data-test    %d\n", e.regions)
	fmt.Fprintf(&b, "  access       %s\n", e.agreed)
	if e.unread < 0 {
		fmt.Fprintf(&b, "  extracted    no, this milestone reads work pages only\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  unread       %d\n", e.unread)
	fmt.Fprintf(&b, "  set          %s\n", strings.Join(e.fields, " "))
	if len(e.missed) > 0 {
		fmt.Fprintf(&b, "  missed       %s\n", strings.Join(e.missed, " "))
	}
	return b.String()
}

func TestCaptureLedger(t *testing.T) {
	var got strings.Builder
	got.WriteString(ledgerHeader)
	entries := make([]ledgerEntry, 0, len(captures))
	for _, c := range captures {
		e := report(t, c)
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
	wantEntries := parseLedger(string(want))
	for _, e := range entries {
		w, ok := wantEntries[e.name]
		if !ok {
			t.Errorf("%s: not in the ledger", e.name)
			continue
		}
		gained, lost := diff(w.fields, e.fields)
		if len(lost) > 0 {
			t.Errorf("%s: stopped setting %s, which is a regression", e.name, strings.Join(lost, " "))
		}
		if len(gained) > 0 {
			t.Errorf("%s: now sets %s, which is an improvement; rerun with -update to record it", e.name, strings.Join(gained, " "))
		}
		gainedMiss, lostMiss := diff(w.missed, e.missed)
		if len(gainedMiss) > 0 {
			t.Errorf("%s: now misses %s", e.name, strings.Join(gainedMiss, " "))
		}
		if len(lostMiss) > 0 {
			t.Errorf("%s: no longer misses %s; rerun with -update to record it", e.name, strings.Join(lostMiss, " "))
		}
		if w.unread != e.unread {
			t.Logf("drift: %s unread regions went from %d to %d", e.name, w.unread, e.unread)
		}
		if w.vocabs != e.vocabs {
			t.Errorf("%s: vocabularies went from %s to %s", e.name, w.vocabs, e.vocabs)
		}
		if w.agreed != e.agreed {
			t.Errorf("%s: the two access declarations went from %q to %q", e.name, w.agreed, e.agreed)
		}
		if w.jsonld != e.jsonld {
			t.Errorf("%s: json-ld blocks went from %d to %d", e.name, w.jsonld, e.jsonld)
		}
	}
	t.Errorf("the ledger and the extractor disagree; rerun with -update once the differences above are understood")
}

// parseLedger reads back what String wrote. It is deliberately a small hand
// parser rather than a serialization format, because the ledger's first job is
// to be read by a person in a pull request diff.
func parseLedger(s string) map[string]ledgerEntry {
	out := map[string]ledgerEntry{}
	var cur *ledgerEntry
	for _, line := range strings.Split(s, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			if cur != nil {
				out[cur.name] = *cur
			}
			cur = &ledgerEntry{name: strings.TrimSpace(line), unread: -1}
			continue
		}
		if cur == nil {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "set":
			cur.fields = strings.Fields(value)
		case "missed":
			cur.missed = strings.Fields(value)
		case "unread":
			_, _ = fmt.Sscanf(value, "%d", &cur.unread)
		case "vocabularies":
			cur.vocabs = value
		case "access":
			cur.agreed = value
		case "json-ld":
			_, _ = fmt.Sscanf(value, "%d", &cur.jsonld)
		case "meta":
			_, _ = fmt.Sscanf(value, "names %d", &cur.metas)
		case "data-test":
			_, _ = fmt.Sscanf(value, "%d", &cur.regions)
		}
	}
	if cur != nil {
		out[cur.name] = *cur
	}
	return out
}

// diff returns what is in b and not a, and what is in a and not b.
func diff(a, b []string) (gained, lost []string) {
	in := func(xs []string, s string) bool {
		for _, x := range xs {
			if x == s {
				return true
			}
		}
		return false
	}
	for _, s := range b {
		if !in(a, s) {
			gained = append(gained, s)
		}
	}
	for _, s := range a {
		if !in(b, s) {
			lost = append(lost, s)
		}
	}
	return gained, lost
}

const ledgerHeader = `# The capture ledger.
#
# Nine real pages, fetched 2026-08-18, extracted by the current code. Fewer
# fields set or more missed is a regression and fails. More fields set is an
# improvement and also fails, until this file is updated, so that an
# improvement is always a reviewed change. A change in unread regions is drift
# and is reported without failing, because Springer shipping a new component is
# news about the site rather than a bug in this tool.
#
# Rewrite with: go test ./spr -run TestCaptureLedger -update
`
