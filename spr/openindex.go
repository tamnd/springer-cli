package spr

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The open indexes, and the one plumbing layer the three of them share.
//
// link.springer.com publishes what a work cites and never publishes what cites
// it. There is no page anywhere on the site that answers "who cited this", and
// no amount of parsing will produce one. That direction exists only in the open
// indexes, which is the whole reason this file exists.
//
// The three hosts are not interchangeable. Crossref is the registration agency
// and holds what publishers deposited, so its reference list is the publisher's
// own. OpenAlex is a derived index and holds both citation directions, an
// inverted abstract and institution ids. The Springer Nature API is the
// publisher's own and needs a key. They disagree about the same work, on
// purpose, and this package keeps their answers apart rather than merging them
// into one number that belongs to nobody.

const (
	// CrossrefHost is the DOI registration agency's public API. No key, and a
	// contact address doubles the request budget.
	CrossrefHost = "api.crossref.org"

	// OpenAlexHost is the open index that replaced Microsoft Academic Graph.
	// No key, and a contact address moves the request into the polite pool
	// without changing any number the host reports.
	OpenAlexHost = "api.openalex.org"

	// SpringerAPIHost is the publisher's own API, which needs a key and
	// answers 401 without one.
	SpringerAPIHost = "api.springernature.com"
)

// Pool is which rate limit pool a request landed in, as the host named it.
//
// It is read and never inferred, because inferring it was measured to be
// wrong. Crossref sends X-Api-Pool on every response and the value moves with
// the request: no mailto answers public-single at 5 requests a second, a mailto
// answers polite-single at 10, and that same mailto against /works?query=
// answers polite-array at 3. A tool that printed "polite pool" on the strength
// of its own configuration would be right about itself and wrong about two of
// those three.
//
// OpenAlex sends no such header. Asked is what this client requested, so the
// debug line can say the honest thing there instead, which is that a contact
// address was sent and the host did not say what it did with it.
type Pool struct {
	// Name is X-Api-Pool verbatim, empty when the host does not send one.
	Name string

	// Asked is "polite" when a contact address went out with the request and
	// "anonymous" when none was configured.
	Asked string

	Seen time.Time
}

// Polite reports whether the host confirmed a polite pool. A host that names no
// pool never confirms one, however politely it was asked.
func (p Pool) Polite() bool { return strings.HasPrefix(p.Name, "polite") }

// String is the one line --debug prints.
func (p Pool) String() string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Asked == "polite":
		return "polite requested, host names no pool"
	default:
		return "anonymous, no contact address configured"
	}
}

// KindJSON responses are decoded here, and the two 404 bodies measured are not
// JSON at all: Crossref answers "Resource not found." as text/plain and
// OpenAlex answers a default HTML error page. Both are 404s, so Classify
// catches them on the status code before the content type is ever consulted,
// and neither one reaches a decoder.

// NoRecord is a 404 from an index, which is an answer rather than a failure.
//
// A DOI that Crossref has never seen is a fact about the DOI and worth
// reporting as one. It is a distinct type so that a caller merging three
// backends can carry on with the two that answered instead of failing the run.
type NoRecord struct {
	Host string
	URL  string
	Code int

	// Says is the host's own words when it used few enough of them. Crossref
	// says "Resource not found." in five plain text bytes over twenty, and
	// OpenAlex answers a Flask error page, which is not worth quoting.
	Says string
}

func (e *NoRecord) Error() string {
	if e.Says != "" {
		return fmt.Sprintf("%s has no record at %s: %s", e.Host, e.URL, e.Says)
	}
	return fmt.Sprintf("%s has no record at %s (%d)", e.Host, e.URL, e.Code)
}

// shortReason returns a body worth quoting in an error, and nothing otherwise.
func shortReason(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" || len(s) > 200 || strings.HasPrefix(s, "<") {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// getJSON fetches raw and decodes it into v, returning the response so the
// caller can stamp an envelope from the same fetch that produced the fields.
func (c *Client) getJSON(ctx context.Context, raw string, v any) (*Response, error) {
	resp, err := c.Get(ctx, raw, KindJSON)
	if err != nil {
		return nil, err
	}
	host := hostOf(resp.URL)
	switch resp.Status {
	case StatusNotFound:
		return resp, &NoRecord{Host: host, URL: resp.URL, Code: resp.Code, Says: shortReason(resp.Body)}
	case StatusWrongKind:
		return resp, fmt.Errorf("%s answered %s rather than json for %s", host, resp.Header.Get("Content-Type"), resp.URL)
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		return resp, fmt.Errorf("%s: decoding %d bytes of json from %s: %w", host, len(resp.Body), resp.URL, err)
	}
	return resp, nil
}

// hostOf names the host of a url for an error message, and returns the url
// itself when it will not parse, because an error message is not the place to
// start failing.
func hostOf(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return raw
}

// readPool records the pool a host named, and what was asked of it.
func (c *Client) readPool(host string, h http.Header) {
	asked := "anonymous"
	if c.Mailto != "" && !strings.EqualFold(host, springerHost) {
		asked = "polite"
	}
	name := h.Get("X-Api-Pool")
	if name == "" && asked == "anonymous" {
		// Nothing was asked and nothing was said, which is every request to
		// link.springer.com and not worth a map entry or a debug line.
		return
	}
	p := Pool{Name: name, Asked: asked, Seen: time.Now()}
	c.rateMu.Lock()
	c.pool[strings.ToLower(host)] = p
	c.rateMu.Unlock()
	c.debugf("pool %s %s", host, p)
}

// Pool reports the pool a host last named, and whether one was recorded.
func (c *Client) Pool(host string) (Pool, bool) {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	p, ok := c.pool[strings.ToLower(host)]
	return p, ok
}

// epochFloor separates a reset that is a wall clock from a reset that is a
// countdown, because the hosts measured use the same header name for both and
// neither says which it means. OpenAlex sent X-RateLimit-Reset: 39286, which is
// eleven hours from now as a countdown and the first of January 1970 as an
// epoch. Anything below this floor is read as a countdown, and the floor is the
// start of 2001, so the ambiguity only comes back if a host starts quoting a
// reset more than thirty years out.
const epochFloor = 978307200

// resetTime turns an X-RateLimit-Reset value into an instant.
func resetTime(v int, now time.Time) time.Time {
	if v >= epochFloor {
		return time.Unix(int64(v), 0)
	}
	return now.Add(time.Duration(v) * time.Second)
}

// headerAny reads the first of several spellings that is present.
//
// The two hosts measured disagree about the hyphen. OpenAlex sends
// X-RateLimit-Limit and Crossref sends X-Rate-Limit-Limit, and http.Header
// canonicalizes the case of each dash separated word but never moves a dash, so
// the two are different keys and both have to be asked for by name.
func headerAny(h http.Header, names ...string) (string, bool) {
	for _, n := range names {
		if v := h.Get(n); v != "" {
			return v, true
		}
	}
	return "", false
}

func headerIntAny(h http.Header, names ...string) (int, bool) {
	v, ok := headerAny(h, names...)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

func headerFloatAny(h http.Header, names ...string) (float64, bool) {
	v, ok := headerAny(h, names...)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// jats strips the JATS markup Crossref stores an abstract in.
//
// The spec expected no abstract from Crossref for most Springer works. On the
// measured article there is one, 1,061 characters of it, and it arrives as
// <jats:title>Abstract</jats:title><jats:p>... inside a JSON string. The tags
// are dropped, the leading "Abstract" heading with them, and the text is
// otherwise untouched. Nothing here tries to be an XML parser: the input is a
// fragment with no root element and a namespace prefix bound nowhere, and a
// real parser rejects it.
func jats(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>' && depth > 0:
			depth--
			if depth == 0 {
				// A tag boundary is a word boundary. Without this, a closing
				// </jats:p> followed by an opening <jats:p> joins two sentences
				// into one word.
				b.WriteByte(' ')
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	out := html.UnescapeString(b.String())
	out = strings.Join(strings.Fields(out), " ")
	return strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(out, "Abstract")), ":")
}

// Enumerating the open indexes is paced the same as everything else.
//
// PaceBucket already gives each host its own bucket, so a run that reads a
// Springer page, a Crossref record and an OpenAlex record waits on three
// independent clocks rather than one queue. Nothing here lowers the floor. The
// hosts allow far more than one request a second, Crossref says so in its own
// headers, and this tool still does not take it: throughput is not the
// constraint on any of these runs, and a client that spends a stated budget to
// its last request is a client with no margin the day the budget changes.
//
// OpenAlex is the one host where the budget binds before the pace does. It
// reported 1,000 requests remaining against a reset eleven hours out, which at
// the default pace is 33 minutes of work and then most of a day of waiting.
// That is why the budget it states is carried on the record rather than only
// logged.
