package spr

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

// The capture ledger.
//
// A parser that quietly reads two fields fewer than it did last month looks
// exactly like a parser that is fine. The ledger is what tells them apart: for
// every capture it records which fields the extractor set, which it named as
// missed, how many regions it left unread, and the four site specific signals
// that move before a field empties.
//
// This file holds the reading and the comparison. TestCaptureLedger runs it
// over the stored captures and spr verify runs it over the page cache or over
// the live site, which is the whole reason it is here rather than in a _test.go
// file: a check that only ever runs against bytes frozen in this repository can
// never tell you the site changed.

// The ledger is embedded so that spr verify carries the recorded reading with
// it. It stays under testdata because the test that rewrites it lives next to
// it, and a file with two readers in two places is a file that goes stale in
// one of them.
//
//go:embed testdata/capture.txt
var ledgerFile string

// Capture is one page the ledger is built from.
type Capture struct {
	// File is the name under testdata/captures, without the .gz.
	File string

	// URL is the address the page was fetched from, which is the requested url
	// and not the effective one.
	URL string

	// Record is which extractor reads this page: work, journal, book, series,
	// volumes, metrics, figure, table or search. It is a separate column from
	// Kind because a work page has both, and every other page has a record and
	// no work type at all.
	Record string

	// Kind is the work type the record should carry, on a work page only.
	Kind string
}

// Captures are the fourteen pages the ledger covers, nine record types between
// them.
//
// The feeds, the sitemaps and the open index answers are deliberately not here.
// The ledger reads meta names, json-ld blocks, data-test regions and the
// analytics payload, and a feed has none of those, so running an html reader
// over one to produce four zeroes would be a row that looks like evidence and
// is not.
var Captures = []Capture{
	{"article_oa.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3", "work", "article"},
	{"article_subscription.html", "https://link.springer.com/article/10.1007/s10994-024-06594-z", "work", "article"},
	{"chapter.html", "https://link.springer.com/chapter/10.1007/978-3-031-28170-9_6", "work", "chapter"},
	{"protocol.html", "https://link.springer.com/protocol/10.1007/978-1-0716-2067-0_1", "work", "protocol"},
	{"referenceworkentry.html", "https://link.springer.com/referenceworkentry/10.1007/978-3-642-27737-5_100-2", "work", "entry"},
	{"book.html", "https://link.springer.com/book/10.1007/978-3-031-28170-9", "book", ""},
	{"journal.html", "https://link.springer.com/journal/10994", "journal", ""},
	{"series.html", "https://link.springer.com/series/558", "series", ""},
	{"volumes.html", "https://link.springer.com/journal/10994/volumes-and-issues", "volumes", ""},
	{"metrics.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3/metrics", "metrics", ""},
	{"metrics_subscription.html", "https://link.springer.com/article/10.1007/s10994-024-06594-z/metrics", "metrics", ""},
	{"figure.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3/figures/1", "figure", ""},
	{"table.html", "https://link.springer.com/article/10.1007/s10994-021-05946-3/tables/1", "table", ""},
	{"search.html", SearchQueryURL, "search", ""},
}

// SearchQuery is the query every search capture was taken with, html and rss
// alike, in the same minute. It is one string so that the two paths cannot
// drift apart in the testdata the way they did on the live site.
const SearchQuery = "query=aleatoric+uncertainty&content-type=Article&date=custom&dateFrom=2020&dateTo=2024&sortBy=relevance"

// SearchQueryURL and SearchFeedURL are that query on each of the two paths.
const (
	SearchQueryURL = "https://link.springer.com/search?" + SearchQuery
	SearchFeedURL  = "https://link.springer.com/search.rss?" + SearchQuery
)

// CaptureNamed returns the capture with this file name, without the extension
// if that is how it was asked for, so that spr verify --capture article_oa and
// --capture article_oa.html both work.
func CaptureNamed(name string) (Capture, bool) {
	for _, c := range Captures {
		if c.File == name || strings.TrimSuffix(c.File, ".html") == name {
			return c, true
		}
	}
	return Capture{}, false
}

// LedgerEntry is one capture's recorded reading.
type LedgerEntry struct {
	Name   string `json:"name"`
	Record string `json:"record"`

	Metas   int `json:"meta_names"`
	JSONLD  int `json:"json_ld"`
	Regions int `json:"data_test"`
	Unread  int `json:"unread"`

	Vocabularies string `json:"vocabularies"`
	DataLayer    string `json:"datalayer"`
	Access       string `json:"access"`

	Fields []string `json:"set"`
	Missed []string `json:"missed,omitempty"`
}

// ReadCapture runs the right extractor over one page and describes what came
// out, in the shape the ledger records.
func ReadCapture(resp *Response, c Capture) (LedgerEntry, error) {
	e := LedgerEntry{Name: c.File, Record: c.Record}

	doc, err := parseDoc(resp.Body)
	if err != nil {
		return e, fmt.Errorf("%s: %w", c.File, err)
	}
	meta := ParseMeta(doc)
	ld := parseLinkData(doc)
	reg := parseRegions(doc)

	e.Metas = len(meta.Names())
	e.Regions = len(reg.names())
	e.JSONLD = ld.count()

	var vocabs []string
	for _, v := range meta.Vocabularies() {
		vocabs = append(vocabs, string(v))
	}
	e.Vocabularies = strings.Join(vocabs, "+")
	if e.Vocabularies == "" {
		e.Vocabularies = "none"
	}

	// The access declaration is stated twice on a work page, once in Highwire
	// and once in schema.org. They agreed on all fourteen captures, which is the
	// reason a disagreement is worth watching for.
	e.Access = accessAgreement(meta, ld)

	// The analytics payload, which every page on this site ships twice. The
	// assignment form is strict JSON and parses everywhere. The push form is
	// JavaScript with single quotes and parses nowhere, and it is the one
	// carrying the data-test attribute. The split is by form and not by page
	// type, which is worth a column of its own precisely because it is easy to
	// assume otherwise.
	dl := parseDataLayer(doc)
	e.DataLayer = fmt.Sprintf("%d assigned, %d pushed, %d broken", len(dl.entries), len(dl.pushes), dl.broken)

	env, err := extractEnvelope(resp, c)
	if err != nil {
		return e, fmt.Errorf("%s: %w", c.File, err)
	}
	for f := range env.Via {
		e.Fields = append(e.Fields, f)
	}
	sort.Strings(e.Fields)
	for _, m := range env.Missed {
		e.Missed = append(e.Missed, m.Field)
	}
	sort.Strings(e.Missed)
	e.Unread = len(env.Unread)
	return e, nil
}

// extractEnvelope runs the extractor this capture is for and returns its
// envelope, which is the only part of nine different record types the ledger
// compares.
func extractEnvelope(resp *Response, c Capture) (Envelope, error) {
	switch c.Record {
	case "work":
		w, err := ExtractWork(resp)
		if err != nil {
			return Envelope{}, err
		}
		if w.Type != c.Kind {
			return Envelope{}, fmt.Errorf("extracted as %q, want %q", w.Type, c.Kind)
		}
		return w.Envelope, nil
	case "journal":
		j, err := ExtractJournal(resp)
		if err != nil {
			return Envelope{}, err
		}
		return j.Envelope, nil
	case "book":
		b, err := ExtractBook(resp)
		if err != nil {
			return Envelope{}, err
		}
		return b.Envelope, nil
	case "series":
		s, err := ExtractSeries(resp)
		if err != nil {
			return Envelope{}, err
		}
		return s.Envelope, nil
	case "volumes":
		v, err := ExtractVolumes(resp)
		if err != nil {
			return Envelope{}, err
		}
		return v.Envelope, nil
	case "metrics":
		m, err := ExtractMetrics(resp)
		if err != nil {
			return Envelope{}, err
		}
		return m.Envelope, nil
	case "figure":
		f, err := ExtractFigure(resp)
		if err != nil {
			return Envelope{}, err
		}
		return f.Envelope, nil
	case "table":
		t, err := ExtractTable(resp)
		if err != nil {
			return Envelope{}, err
		}
		return t.Envelope, nil
	case "search":
		s, err := ExtractSearch(resp)
		if err != nil {
			return Envelope{}, err
		}
		return s.Envelope, nil
	}
	return Envelope{}, fmt.Errorf("no extractor is registered for record %q", c.Record)
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

// String writes the entry the way the ledger file holds it.
func (e LedgerEntry) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", e.Name)
	fmt.Fprintf(&b, "  record       %s\n", e.Record)
	fmt.Fprintf(&b, "  meta names   %d\n", e.Metas)
	fmt.Fprintf(&b, "  json-ld      %d\n", e.JSONLD)
	fmt.Fprintf(&b, "  vocabularies %s\n", e.Vocabularies)
	fmt.Fprintf(&b, "  data-test    %d\n", e.Regions)
	fmt.Fprintf(&b, "  datalayer    %s\n", e.DataLayer)
	fmt.Fprintf(&b, "  access       %s\n", e.Access)
	fmt.Fprintf(&b, "  unread       %d\n", e.Unread)
	fmt.Fprintf(&b, "  set          %s\n", strings.Join(e.Fields, " "))
	if len(e.Missed) > 0 {
		fmt.Fprintf(&b, "  missed       %s\n", strings.Join(e.Missed, " "))
	}
	return b.String()
}

// Ledger returns the recorded reading, keyed on the capture file name.
func Ledger() map[string]LedgerEntry { return ParseLedger(ledgerFile) }

// ParseLedger reads back what String wrote. It is deliberately a small hand
// parser rather than a serialization format, because the ledger's first job is
// to be read by a person in a pull request diff.
func ParseLedger(s string) map[string]LedgerEntry {
	out := map[string]LedgerEntry{}
	var cur *LedgerEntry
	for _, line := range strings.Split(s, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			if cur != nil {
				out[cur.Name] = *cur
			}
			cur = &LedgerEntry{Name: strings.TrimSpace(line), Unread: -1}
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
			cur.Fields = strings.Fields(value)
		case "missed":
			cur.Missed = strings.Fields(value)
		case "unread":
			_, _ = fmt.Sscanf(value, "%d", &cur.Unread)
		case "vocabularies":
			cur.Vocabularies = value
		case "access":
			cur.Access = value
		case "json-ld":
			_, _ = fmt.Sscanf(value, "%d", &cur.JSONLD)
		case "meta":
			_, _ = fmt.Sscanf(value, "names %d", &cur.Metas)
		case "data-test":
			_, _ = fmt.Sscanf(value, "%d", &cur.Regions)
		case "datalayer":
			cur.DataLayer = value
		case "record":
			cur.Record = value
		}
	}
	if cur != nil {
		out[cur.Name] = *cur
	}
	return out
}

// Verdict is what a comparison against the ledger came to.
//
// The four are separate because they are four different pieces of news and
// only two of them are anybody's fault. A regression is a bug in this tool. An
// improvement is a change somebody meant to make and has not recorded yet. A
// signal moving is the site restating a fact differently, which needs a person.
// Drift is Springer shipping a component, which is news about the site.
type Verdict string

const (
	VerdictOK          Verdict = "ok"
	VerdictRegression  Verdict = "regression"
	VerdictImprovement Verdict = "improvement"
	VerdictChanged     Verdict = "changed"
	VerdictDrift       Verdict = "drift"
)

// LedgerDiff is how one fresh reading differs from what was recorded.
type LedgerDiff struct {
	Name string `json:"name"`

	Lost   []string `json:"lost,omitempty"`
	Gained []string `json:"gained,omitempty"`

	NowMissed      []string `json:"now_missed,omitempty"`
	NoLongerMissed []string `json:"no_longer_missed,omitempty"`

	// Moved is the signals that changed: the vocabularies a page carries, the
	// agreement between the two access declarations, the json-ld block count
	// and the analytics payload.
	Moved []SignalMove `json:"moved,omitempty"`

	UnreadFrom int `json:"unread_from"`
	UnreadTo   int `json:"unread_to"`
}

// SignalMove is one of the four site signals reading differently than recorded.
type SignalMove struct {
	What string `json:"what"`
	From string `json:"from"`
	To   string `json:"to"`
}

// CompareLedger says how a fresh reading differs from the recorded one.
func CompareLedger(want, got LedgerEntry) LedgerDiff {
	d := LedgerDiff{Name: got.Name, UnreadFrom: want.Unread, UnreadTo: got.Unread}
	d.Gained, d.Lost = diffStrings(want.Fields, got.Fields)
	d.NowMissed, d.NoLongerMissed = diffStrings(want.Missed, got.Missed)

	move := func(what, from, to string) {
		if from != to {
			d.Moved = append(d.Moved, SignalMove{What: what, From: from, To: to})
		}
	}
	move("vocabularies", want.Vocabularies, got.Vocabularies)
	move("the two access declarations", want.Access, got.Access)
	move("json-ld blocks", fmt.Sprint(want.JSONLD), fmt.Sprint(got.JSONLD))
	move("the analytics payload", want.DataLayer, got.DataLayer)
	return d
}

// Verdict grades one diff. Regression wins over improvement, because a reading
// that both gained and lost a field is a reading that lost a field.
func (d LedgerDiff) Verdict() Verdict {
	switch {
	case len(d.Lost) > 0 || len(d.NowMissed) > 0:
		return VerdictRegression
	case len(d.Moved) > 0:
		return VerdictChanged
	case len(d.Gained) > 0 || len(d.NoLongerMissed) > 0:
		return VerdictImprovement
	case d.UnreadFrom != d.UnreadTo:
		return VerdictDrift
	default:
		return VerdictOK
	}
}

// Lines describes the diff for a person, one finding per line, in the order
// the verdict weighs them.
func (d LedgerDiff) Lines() []string {
	var out []string
	if len(d.Lost) > 0 {
		out = append(out, "stopped setting "+strings.Join(d.Lost, " "))
	}
	if len(d.NowMissed) > 0 {
		out = append(out, "now misses "+strings.Join(d.NowMissed, " "))
	}
	for _, m := range d.Moved {
		out = append(out, fmt.Sprintf("%s went from %s to %s", m.What, m.From, m.To))
	}
	if len(d.Gained) > 0 {
		out = append(out, "now sets "+strings.Join(d.Gained, " "))
	}
	if len(d.NoLongerMissed) > 0 {
		out = append(out, "no longer misses "+strings.Join(d.NoLongerMissed, " "))
	}
	if d.UnreadFrom != d.UnreadTo {
		out = append(out, fmt.Sprintf("unread regions went from %d to %d", d.UnreadFrom, d.UnreadTo))
	}
	return out
}

// diffStrings returns what is in b and not a, and what is in a and not b.
func diffStrings(a, b []string) (gained, lost []string) {
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

// VocabReading is what every vocabulary on one page claims about one fact.
type VocabReading struct {
	Fact   string            `json:"fact"`
	Claims map[string]string `json:"claims"`
	Agree  bool              `json:"agree"`
}

// ReadVocabularies cross-checks every fact that more than one vocabulary
// declares, and reports the agreements as well as the divergences.
//
// The agreements are the point. They agreed on all fourteen captures, and a
// report that printed only divergences would be an empty page that looks
// identical whether the check ran or not.
func ReadVocabularies(resp *Response) ([]VocabReading, error) {
	doc, err := parseDoc(resp.Body)
	if err != nil {
		return nil, err
	}
	meta := ParseMeta(doc)
	ld := parseLinkData(doc)

	diverged := map[string]bool{}
	for _, d := range meta.CrossCheck() {
		diverged[d.Fact] = true
	}

	var out []VocabReading
	for _, c := range crossChecks {
		claims := map[string]string{}
		for v, name := range c.names {
			if got := meta.First(name); got != "" {
				claims[string(v)+":"+name] = got
			}
		}
		if len(claims) < 2 {
			continue
		}
		out = append(out, VocabReading{Fact: c.fact, Claims: claims, Agree: !diverged[c.fact]})
	}

	// The access declaration is the one fact stated in two different formats
	// rather than in two meta names, so it is checked here rather than in the
	// meta cross check, and it is the one people care about most.
	raw := meta.First("access")
	work := ld.work()
	if raw != "" && work != nil && work.IsAccessibleForFree != nil {
		out = append(out, VocabReading{
			Fact: "access",
			Claims: map[string]string{
				"highwire:access":            raw,
				"jsonld:isAccessibleForFree": fmt.Sprint(*work.IsAccessibleForFree),
			},
			Agree: accessAgreement(meta, ld) == "agree",
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Fact < out[j].Fact })
	return out, nil
}

// LedgerHeader is the prose at the top of the ledger file, rewritten with it.
const LedgerHeader = `# The capture ledger.
#
# Fourteen real pages across nine record types, fetched 2026-08-18, extracted by the current code.
#
# Fewer fields set or more missed is a regression and fails. More fields set is an improvement
# and also fails, until this file is updated, so that an improvement is always a reviewed change.
# A change in unread regions is drift and is reported without failing, because Springer shipping
# a new component is news about the site rather than a bug in this tool.
#
# The datalayer line counts the two analytics forms separately. Assigned is window.dataLayer =
# [{...}], which is strict JSON and parses on every page. Pushed is window.dataLayer.push({...}),
# which is javascript and parses on none of them, and is carried to the envelope unread. Both
# numbers are non zero on every capture here except the search page, which ships three pushed
# blocks and no assigned one, so it is the only page on this site whose whole analytics payload
# is unreadable. Everywhere else the readable and unreadable split is by form and not by page.
#
# This file is read twice: by TestCaptureLedger against the stored captures, and by spr verify
# against the page cache or the live site. The first says the extractor did not change and the
# second says the site did not.
#
# Rewrite with: go test ./spr -run TestCaptureLedger -update
`
