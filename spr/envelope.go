package spr

import (
	"encoding/json"
	"sort"
	"time"
)

// Envelope is what turns a record you have to trust into a record you can
// audit. Every record carries one.
//
// The load bearing field is Missed. Absent means absent everywhere in this
// tool, so a field missing from a record with no Missed entry means the
// extractor read the right place and there was nothing there. A field missing
// with a Missed entry means something stopped it, and the reason is written
// out. Without that distinction every empty field is the same shrug.
//
// Unread is the other half of the same honesty. It names every region on the
// page the extractor did not read, which is how the tool says out loud what it
// is leaving on the table rather than letting the record look complete.
type Envelope struct {
	// Tier is which surface produced this record: html, api, crossref,
	// openalex. Everything in this milestone is html.
	Tier string `json:"tier"`

	// URLs is a list because a deep record is built from several responses and
	// each field's provenance has to stay traceable to the one that made it.
	// The urls are the requested ones, never the effective ones, because the
	// effective url carries a per request uuid and is not an identifier.
	URLs []string `json:"urls"`

	Fetched time.Time `json:"fetched"`

	// Status is the classification, not the HTTP code. A challenge, a
	// paywalled article and an html page served from a pdf url all answer 200.
	Status Status `json:"status"`

	Redirects int `json:"redirects"`
	Bytes     int `json:"bytes"`

	// Via maps a field name to the rung and the exact tag or region that
	// answered it, written as "highwire:citation_title" or "region:data-title".
	Via map[string]string `json:"via,omitempty"`

	Missed []Miss   `json:"missed,omitempty"`
	Unread []string `json:"unread,omitempty"`

	// Extra carries payloads that were found and deliberately not read. The
	// article's analytics blob is here verbatim, because it does not parse as
	// JSON and a lenient parser that recovers most of it most of the time is a
	// permanent source of quiet wrongness. Carrying the bytes and reading
	// nothing out of them is the honest handling.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Miss is a field that was looked for and did not arrive, with the reason.
type Miss struct {
	Field string `json:"field"`
	Why   string `json:"why"`
}

// via records which rung and which source answered a field.
func (e *Envelope) via(field string, l Level, source string) {
	if e.Via == nil {
		e.Via = map[string]string{}
	}
	e.Via[field] = l.String() + ":" + source
}

// miss records a field that was looked for and did not arrive.
func (e *Envelope) miss(field, why string) {
	e.Missed = append(e.Missed, Miss{Field: field, Why: why})
}

// extra stores a payload that was found and not read.
func (e *Envelope) extra(name string, raw []byte) {
	if e.Extra == nil {
		e.Extra = map[string]json.RawMessage{}
	}
	// The payload is stored as a JSON string when it is not JSON, so that a
	// record always parses even when the thing it carries did not.
	if json.Valid(raw) {
		e.Extra[name] = json.RawMessage(raw)
		return
	}
	quoted, err := json.Marshal(string(raw))
	if err != nil {
		return
	}
	e.Extra[name] = json.RawMessage(quoted)
}

// sortMissed puts the missed list in a stable order so that two runs of the
// same page produce the same bytes and a diff means something changed.
func (e *Envelope) sortMissed() {
	sort.Slice(e.Missed, func(i, j int) bool { return e.Missed[i].Field < e.Missed[j].Field })
}
