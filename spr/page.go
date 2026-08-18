package spr

import (
	"errors"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The scaffolding every extractor shares.
//
// A page is one fetched document with all four rungs parsed and an envelope
// already stamped from the response. The work extractor builds its own because
// it was written first and carries a Work through the walk; the container
// extractors take one of these and fill their own record.

// page is a parsed document plus the envelope being built for it.
type page struct {
	doc  *html.Node
	meta *Meta
	ld   *linkData
	reg  *regions
	dl   *dataLayer
	env  Envelope
}

// newPage parses a response into a page and stamps the envelope.
func newPage(resp *Response) (*page, error) {
	if resp == nil {
		return nil, errors.New("no response to extract from")
	}
	if resp.Status == StatusChallenged {
		return nil, errors.New("the edge served a client challenge, so there is no page to extract")
	}
	doc, err := parseDoc(resp.Body)
	if err != nil {
		return nil, err
	}
	return &page{
		doc:  doc,
		meta: ParseMeta(doc),
		ld:   parseLinkData(doc),
		reg:  parseRegions(doc),
		dl:   parseDataLayer(doc),
		env: Envelope{
			Tier:      "html",
			URLs:      []string{resp.URL},
			Fetched:   resp.Fetched,
			Status:    resp.Status,
			Redirects: resp.Redirects,
			Bytes:     len(resp.Body),
		},
	}, nil
}

// finish closes the envelope: the unread regions, the payloads carried and not
// read, and a stable order for the misses.
func (p *page) finish() Envelope {
	for name, src := range p.dl.pushes {
		p.env.extra(name, []byte(src))
	}
	if p.dl.broken > 0 {
		p.env.miss("datalayer", strconv.Itoa(p.dl.broken)+" analytics blocks on this page are in the assignment form and did not parse as json")
	}
	if p.ld.broken > 0 {
		p.env.miss("linked_data", strconv.Itoa(p.ld.broken)+" schema.org blocks on this page did not parse as json")
	}
	p.env.Unread = p.reg.unread()
	p.env.sortMissed()
	return p.env
}

// set records a field and its source, and reports whether there was anything to
// record. It is the container equivalent of the ladder's pick, and it is
// simpler because on a container page there is usually exactly one source.
func (p *page) set(field string, l Level, source, value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	p.env.via(field, l, source)
	return v
}

// region reads a named region and records the read, which is the common case on
// every container page.
func (p *page) region(field, name string) string {
	return p.set(field, LevelRegion, "[data-test="+name+"]", p.reg.text(name))
}

// layer reads a path out of the analytics payload and records it.
func (p *page) layer(field string, path ...string) string {
	return p.set(field, LevelRegion, "datalayer."+strings.Join(path, "."), p.dl.str(path...))
}

// The description list pattern.
//
// Springer states most of a container's facts as a <dt> label and a <dd> value
// inside one named region: Electronic ISSN and 1573-0565, Series Editor and
// four names. Reading the region's whole text gives "Electronic ISSN
// 1573-0565", which is the label glued to the value, so these two read the
// halves apart.

// term is the <dt> under a node.
func term(n *html.Node) string {
	for _, d := range findTag(n, atom.Dt) {
		return text(d)
	}
	return ""
}

// detail is the <dd> under a node.
func detail(n *html.Node) string {
	for _, d := range findTag(n, atom.Dd) {
		return text(d)
	}
	return ""
}

// listItems returns the text of every <li> under a node, which is how an editor
// list, an author list and a subject list are all printed.
func listItems(n *html.Node) []string {
	var out []string
	for _, li := range findTag(n, atom.Li) {
		if s := strings.TrimSpace(strings.TrimSuffix(text(li), ",")); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// named turns a list of printed names into authors with their positions.
//
// Nothing is split into family and given here, for the same reason nothing is
// on a work: name splitting is wrong often enough across the world's naming
// conventions that a guess is worse than an absence. An editor read off a
// container page is a name and a position, which is all the page said.
func named(names []string) []Author {
	out := make([]Author, 0, len(names))
	for i, n := range names {
		out = append(out, Author{Name: n, Position: i})
	}
	return out
}

// firstInt reads the first run of digits in a string, which is how "1169
// Accesses", "664 open access articles" and "Copyright: 2027" all give up their
// number. It returns 0 when there is no digit, and every caller treats 0 as
// absent rather than as a count of zero, because none of these labels is ever
// printed with a zero in it.
func firstInt(s string) int {
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			n, _ := strconv.Atoi(s[start:i])
			return n
		}
	}
	if start >= 0 {
		n, _ := strconv.Atoi(s[start:])
		return n
	}
	return 0
}

// afterColon returns what follows the first colon, which is how "Published: 27
// April 2023", "Copyright: 2027" and "Publishing model: Hybrid" are all
// printed. A string with no colon comes back whole rather than empty.
func afterColon(s string) string {
	if _, rest, ok := strings.Cut(s, ":"); ok {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(s)
}
