package spr

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// /search.rss, the primary search path.
//
// It answers the same query engine the html search does, pages to the end of
// the result set, and carries the full abstract rather than the truncated card
// version. It is also the path that kept answering when the html surface
// started serving challenges, which is why it is primary and the html pass is
// enrichment.
//
// Four things about this feed are worth knowing before reading the code, and
// all four were measured rather than assumed.
//
// It has no xml declaration. The first bytes are <rss version="2.0"> with no
// <?xml version="1.0" encoding="UTF-8"?> in front of them. Anything that sniffs
// for <?xml to decide whether it has xml will reject the primary search path of
// this site.
//
// The description is not a string. It holds <p>, <i>, <InternalRef> and
// <CitationRef> elements, so binding it to a Go string field silently returns
// empty on 19 of 20 items and returns the floating words "Graphical abstract"
// on the twentieth. It parses, it does not error, and it looks like Springer
// omitted the abstracts. The abstract is the single most valuable thing this
// feed carries, so it gets a deliberate text walk and a test.
//
// One link in twenty is malformed. guid 10.1038/s44334-024-00011-y arrives with
// link https://link.springer.comhttps://www.nature.com/articles/s44334-024-00011-y,
// which is the feed's own base concatenated onto an already absolute Nature
// url. The guid is a clean doi either way, which is the reason the doi and not
// the link is this record's identifier.
//
// An empty feed is empty in two different ways. Page 200 of a 557 result query
// returns 186 bytes with nothing between <link> and </channel>. Page 29 returns
// 190 bytes with the four characters null sitting there instead. Both are 200
// ok. So paging terminates on the item count and never on the response size or
// the status code.

// ErrNotAFeed is returned for a body that is not an rss feed at all, which is
// what an html error page delivered under a .rss url looks like.
var ErrNotAFeed = errors.New("this response is not an rss feed")

// Feed is one page of the search feed.
type Feed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Items       []SearchResult `json:"items,omitempty"`
	Envelope    Envelope       `json:"envelope"`
}

// rssFeed, rssChannel and rssItem are the wire shape, kept separate from the
// record so that the record is not built out of the feed's field names.
type rssFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Description string    `xml:"description"`
		Items       []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string   `xml:"title"`
	Desc    richText `xml:"description"`
	Link    string   `xml:"link"`
	PubDate string   `xml:"pubDate"`
	GUID    string   `xml:"guid"`
}

// ParseFeed reads one page of the search feed.
func ParseFeed(resp *Response) (*Feed, error) {
	if resp == nil {
		return nil, errors.New("no response to parse")
	}
	if resp.Status == StatusChallenged {
		return nil, errors.New("the edge served a client challenge, so there is no feed to parse")
	}

	var wire rssFeed
	if err := xml.Unmarshal(resp.Body, &wire); err != nil {
		return nil, ErrNotAFeed
	}

	f := &Feed{
		Title:       strings.TrimSpace(wire.Channel.Title),
		Description: strings.TrimSpace(wire.Channel.Description),
		Envelope: Envelope{
			Tier:      "rss",
			URLs:      []string{resp.URL},
			Fetched:   resp.Fetched,
			Status:    resp.Status,
			Redirects: resp.Redirects,
			Bytes:     len(resp.Body),
		},
	}

	// The channel is the only place a not-a-feed shows up as a parse that
	// succeeded. encoding/xml accepts any document with no <channel> in it and
	// hands back a zero value, so an html error page under a .rss url arrives
	// here looking like a feed with no items.
	if f.Title == "" && len(wire.Channel.Items) == 0 && !looksLikeRSS(resp.Body) {
		return nil, ErrNotAFeed
	}

	var missing int
	for i, it := range wire.Channel.Items {
		r := SearchResult{
			Position: i + 1,
			Via:      ViaRSS,
			DOI:      strings.TrimSpace(it.GUID),
			Title:    collapse(it.Title),
			Abstract: it.Desc.Text,
			URL:      feedLink(it.Link),
		}
		if r.URL == "" && r.DOI != "" {
			r.URL = Base + "/" + r.DOI
		}
		if d, err := ParseRSSDate(strings.TrimSpace(it.PubDate)); err == nil {
			r.Published = &d
		} else if strings.TrimSpace(it.PubDate) != "" {
			f.Envelope.miss("published", "the feed sent a pubDate of "+strings.TrimSpace(it.PubDate)+" and no measured layout reads it")
		}
		if r.Abstract == "" {
			missing++
		}
		f.Items = append(f.Items, r)
	}

	if len(f.Items) > 0 {
		f.Envelope.via("results", LevelRegion, "rss:item")
		f.Envelope.via("doi", LevelRegion, "rss:guid")
		f.Envelope.via("title", LevelRegion, "rss:title")
		f.Envelope.via("url", LevelRegion, "rss:link")
		if missing < len(f.Items) {
			f.Envelope.via("abstract", LevelRegion, "rss:description")
		}
	}
	// Some works have no abstract, and the feed says so by sending an empty
	// description rather than by omitting the element. 2 of the 17 items on the
	// last page of the stored query are like this. Naming the count is what
	// separates that from the whole field having broken, which is exactly the
	// failure a string binding produces here.
	if missing > 0 {
		f.Envelope.miss("abstract", fmt.Sprintf("%d of %d items in this page of the feed carried an empty description, so those works have no abstract in the feed", missing, len(f.Items)))
	}
	f.Envelope.sortMissed()
	return f, nil
}

// looksLikeRSS reports whether the body opens as this feed does.
//
// Written against the bytes rather than against a parser because the feed ships
// no xml declaration, so the usual sniff finds nothing.
func looksLikeRSS(body []byte) bool {
	head := body
	if len(head) > 256 {
		head = head[:256]
	}
	return strings.Contains(string(head), "<rss")
}

// feedLink repairs the one link shape this feed gets wrong.
//
// The feed prefixes its own base onto urls that are already absolute, which
// produces https://link.springer.comhttps://www.nature.com/articles/... for
// works hosted on a Springer Nature site other than link.springer.com. The
// repair is to take the inner url, because a url with two schemes in it has
// exactly one plausible reading and this is it.
func feedLink(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if rest := strings.TrimPrefix(href, Base); rest != href {
		if strings.HasPrefix(rest, "http://") || strings.HasPrefix(rest, "https://") {
			return rest
		}
	}
	return absolute(href)
}

// richText is a description element read as text rather than as a string.
//
// Go's xml string binding takes the direct chardata of an element and drops
// everything inside child elements. This walks the whole subtree, keeps the
// text of the inline elements where the sentence needs it, and separates
// paragraphs with a blank line so that a two paragraph abstract does not come
// back with the last word of one run into the first word of the next.
type richText struct {
	Text string
}

func (r *richText) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var paras []string
	var cur strings.Builder
	flush := func() {
		if s := collapse(cur.String()); s != "" {
			paras = append(paras, s)
		}
		cur.Reset()
	}

	depth := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.CharData:
			cur.Write(t)
		case xml.StartElement:
			// A block element ends the paragraph being built. Everything else,
			// <i> and <InternalRef> and <CitationRef> among them, is part of
			// the sentence it sits in and its text runs on.
			if isBlockElement(t.Name.Local) {
				flush()
			}
			depth++
		case xml.EndElement:
			if depth == 0 {
				flush()
				r.Text = strings.Join(paras, "\n\n")
				return nil
			}
			depth--
			if isBlockElement(t.Name.Local) {
				flush()
			}
		}
	}
}

// isBlockElement reports whether this element ends the paragraph it is in.
//
// The list is short because the feed's own vocabulary is short. Measured across
// 557 items: p, br, div, and the heading levels, against inline i, em, b, sup,
// sub, InternalRef, CitationRef and Emphasis.
func isBlockElement(name string) bool {
	switch strings.ToLower(name) {
	case "p", "br", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}
