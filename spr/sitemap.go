package spr

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The sitemaps, which are the only complete enumeration of what this site
// publishes. Twelve maps in total: one index of dated shards, and eight static
// maps that between them name every journal, series and collection.
//
// Four things about them decide this file, and all four were measured.
//
// There are three shapes behind two file extensions. sitemap-index.xml and
// sitemap-subjects-index.xml are indexes of other sitemaps, the shards and the
// imprint maps are urlsets, and sitemap_shops.txt is one url per line of plain
// text sitting at a .txt path among .xml siblings. Two of the xml maps start
// with an xml declaration and two start straight in on the root element, so
// neither the extension nor the first byte is enough on its own. The form is
// sniffed from the first element name.
//
// The date in a shard's file name is a bucket and not a publication date. The
// index holds 66 shards named 2020-01-01, roughly 330,000 works, because the
// first of January is where everything known only to its year is filed. The
// entries inside sitemap_2020-01-01_1.xml carry 173 distinct lastmod values
// running from 2020-01-23 to 2026-08-17 and not one of them is 2020-01-01. So
// the field here is Bucket, it is parsed from the file name, and there is no
// path in this package by which it becomes a published date.
//
// lastmod is on the day shards and nowhere else. 8,106 of the index's 10,408
// children carry one, always ten characters, always identical to the bucket in
// the file name. All 2,252 month shards and all 50 year shards carry none. The
// element restates the bucket where the bucket is a day and is absent exactly
// where the site knows only a month or a year, which is worth recording in the
// envelope rather than treating as a hole.
//
// A shard that does not exist answers 200 with 108 bytes of empty urlset. Same
// trap as the empty search feed: neither the status nor the size says anything,
// so enumeration ends on the entry count.

const (
	// IndexPath is the root index of dated shards.
	IndexPath = "/sitemap-index.xml"

	// SubjectsIndexPath is the other index, and it is an index at a name that
	// reads like a list. Ten children, named with url encoded subject phrases
	// rather than ids, one of them literally sitemap-subjects-FIRST.xml.
	SubjectsIndexPath = "/sitemap-subjects-index.xml"

	// ShardCeiling is the largest shard measured, in urls. The sitemap protocol
	// allows 50,000 and this site fills to 5,000, so a bound computed from it is
	// an upper bound on a full shard and not a guess.
	ShardCeiling = 5000

	// ShardBytes is a full shard on the wire, from sitemap_2020-01-01_1.xml at
	// 572,445 bytes. It is used for one thing, the size half of the bill that
	// --all prints, and it is a measurement rather than a limit.
	ShardBytes = 572445
)

// ErrNotASitemap is a body that is not any of the three shapes a sitemap
// arrives in. An html error page under a .xml url is what this looks like.
var ErrNotASitemap = errors.New("this response is not a sitemap")

// Form is which of the three shapes a fetched sitemap turned out to be.
type Form string

const (
	// FormIndex is a sitemapindex: a list of other sitemaps.
	FormIndex Form = "index"

	// FormURLSet is a urlset: a list of pages.
	FormURLSet Form = "urlset"

	// FormText is one url per line, which is how sitemap_shops.txt is served.
	FormText Form = "text"
)

// Sitemap is one fetched map, in whichever form it arrived.
//
// Index and list are one type rather than two because which one you have is not
// knowable until the bytes are read. sitemap-subjects-index.xml is an index,
// sitemap-collections.xml is a list, and the only difference in their addresses
// is a word in the middle of the file name.
type Sitemap struct {
	URL  string `json:"url"`
	Form Form   `json:"form"`

	// Children are the other sitemaps this one points at, on an index.
	Children []Child `json:"children,omitempty"`

	// Entries are the pages this one lists, on a urlset or a text map.
	Entries []Entry `json:"entries,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Child is one line of an index: another sitemap, and what its file name says.
type Child struct {
	URL  string `json:"url"`
	Name string `json:"name"`

	// Bucket is the date in the file name, at the precision the name states it.
	// It is where a record is filed and not when it was published.
	Bucket Bucket `json:"bucket"`

	// Shard is the trailing number in the name, so sitemap_2020-01-01_7.xml is
	// shard 7 of the 66 that bucket has.
	Shard int `json:"shard,omitempty"`

	// LastMod is the sitemap protocol's element, present on day shards only,
	// and identical to Bucket wherever it is present.
	LastMod string `json:"lastmod,omitempty"`
}

// Dated reports whether this child's name carried a bucket. The children of the
// subjects index do not: their names are subject phrases.
func (c Child) Dated() bool { return !c.Bucket.Zero() }

// Entry is one page a sitemap lists.
type Entry struct {
	URL string `json:"url"`

	// Kind is what the first path segment says this url is: article, chapter,
	// protocol, entry, book, referencework, journal, series, collection, brand,
	// partner or shop. It is the url's own word and it costs no request, which
	// is what makes --kind a filter rather than a crawl.
	Kind string `json:"kind,omitempty"`

	// LastMod is carried as the ten characters the map printed. It is a
	// modification stamp on a page and not a date on a record, so it is not a
	// Date and it does not go anywhere near one.
	LastMod string `json:"lastmod,omitempty"`
}

// Bucket is the date in a shard's file name.
//
// It has a precision because the name has one: sitemap_1850_1.xml states a
// year, sitemap_1900-01_1.xml states a month, sitemap_2020-01-01_1.xml states a
// day, and the 66 shards at that last name are the proof that a stated day can
// still mean nothing more than a year. A window filter therefore compares
// against the span a bucket covers rather than against a single instant.
type Bucket struct {
	// Raw is the date exactly as the file name spelled it.
	Raw string `json:"raw,omitempty"`

	// Precision is how much of it the name stated.
	Precision Precision `json:"precision,omitempty"`

	// Start is the first instant the bucket covers. It is a span boundary and
	// not a claim about any record in the shard.
	Start time.Time `json:"-"`
}

// Zero reports whether the name carried no bucket at all.
func (b Bucket) Zero() bool { return b.Start.IsZero() }

// String prints the bucket the way the file name spelled it.
func (b Bucket) String() string { return b.Raw }

// End is the first instant after the span this bucket covers.
func (b Bucket) End() time.Time {
	switch b.Precision {
	case PrecisionYear:
		return b.Start.AddDate(1, 0, 0)
	case PrecisionMonth:
		return b.Start.AddDate(0, 1, 0)
	default:
		return b.Start.AddDate(0, 0, 1)
	}
}

// InWindow reports whether this bucket's span overlaps [since, until], with a
// zero time meaning that end is open.
//
// The comparison is against the span and not against Start, because a year
// bucket covers a year: --since 1850-06-01 has to keep sitemap_1850_1.xml,
// whose records are filed under a year that has not been narrowed down and
// half of which is after that date.
//
// A child with no bucket is in no window. It is not dated, so nothing can be
// said about whether it belongs in a dated range, and quietly including it
// would put undated shards in every window a caller ever asks for.
func (b Bucket) InWindow(since, until time.Time) bool {
	if since.IsZero() && until.IsZero() {
		return true
	}
	if b.Zero() {
		return false
	}
	if !since.IsZero() && !b.End().After(since) {
		return false
	}
	if !until.IsZero() && b.Start.After(until) {
		return false
	}
	return true
}

// childName is the four name shapes, which are one pattern with two optional
// halves. Counted across the whole index: 7,304 day shards, 1,988 month shards,
// 802 day shards past the ninth, 264 month shards past the ninth, 50 year
// shards, and nothing that failed to match.
var childName = regexp.MustCompile(`^sitemap_(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?_(\d+)\.xml$`)

// ParseChildName reads the bucket and the shard number out of a child sitemap's
// file name. A name in none of the four shapes returns false rather than a
// guess.
func ParseChildName(name string) (Bucket, int, bool) {
	m := childName.FindStringSubmatch(name)
	if m == nil {
		return Bucket{}, 0, false
	}
	year, _ := strconv.Atoi(m[1])
	month, day := 1, 1
	precision := PrecisionYear
	raw := m[1]
	if m[2] != "" {
		month, _ = strconv.Atoi(m[2])
		precision = PrecisionMonth
		raw += "-" + m[2]
	}
	if m[3] != "" {
		day, _ = strconv.Atoi(m[3])
		precision = PrecisionDay
		raw += "-" + m[3]
	}
	shard, _ := strconv.Atoi(m[4])
	return Bucket{
		Raw:       raw,
		Precision: precision,
		Start:     time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC),
	}, shard, true
}

// entryPaths maps a first path segment to the kind of thing that lives under
// it. It is a slice and not a map because /referenceworkentry/ has to be tried
// before /referencework/, which a map's iteration order would decide by coin
// toss.
//
// Both spellings of a reference work entry are here for the same reason they
// are in WorkType: the site canonicalizes to /rwe/ and keeps serving and
// linking the long one. The shards use /rwe/, 1,249 of them in one shard alone.
var entryPaths = []struct{ prefix, kind string }{
	{"/article/", "article"},
	{"/chapter/", "chapter"},
	{"/protocol/", "protocol"},
	{"/referenceworkentry/", "entry"},
	{"/rwe/", "entry"},
	{"/referencework/", "referencework"},
	{"/book/", "book"},
	{"/journal/", "journal"},
	{"/series/", "series"},
	{"/collections/", "collection"},
	{"/brands/", "brand"},
	{"/partners/", "partner"},
	{"/shop/", "shop"},
}

// EntryKinds are the kinds --kind accepts, in the order they are worth reading.
var EntryKinds = []string{
	"article", "chapter", "protocol", "entry", "referencework", "book",
	"journal", "series", "collection", "brand", "partner", "shop",
}

// kindAliases are the spellings a person types for a kind whose real name is
// something else. The url segment is not the word anybody says out loud.
var kindAliases = map[string]string{
	"rwe":                "entry",
	"referenceworkentry": "entry",
	"collections":        "collection",
	"brands":             "brand",
	"partners":           "partner",
	"shops":              "shop",
	"books":              "book",
	"articles":           "article",
	"chapters":           "chapter",
	"protocols":          "protocol",
	"journals":           "journal",
}

// NormalizeKind turns what a caller typed into the kind name records carry, and
// says whether it is one this tool knows.
func NormalizeKind(s string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(s))
	if alias, ok := kindAliases[k]; ok {
		k = alias
	}
	for _, known := range EntryKinds {
		if k == known {
			return k, true
		}
	}
	return k, false
}

// EntryKind says what a sitemap url addresses, from the url alone, or the empty
// string for a path this tool has no name for.
func EntryKind(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	path := u.Path
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	for _, p := range entryPaths {
		if strings.HasPrefix(path, p.prefix) {
			return p.kind
		}
	}
	return ""
}

// The wire shapes, kept separate from the records so that the records are not
// built out of the protocol's field names.
type xmlIndex struct {
	Sitemaps []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"sitemap"`
}

type xmlURLSet struct {
	URLs []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
		// The alternates on the brands map. They are counted and not read: 66
		// of them across 22 of its 79 urls, and every one is the same page in
		// another language.
		Links []struct {
			HrefLang string `xml:"hreflang,attr"`
		} `xml:"link"`
	} `xml:"url"`
}

// rootElement returns the name of the first element in a body, skipping the xml
// declaration, any comments and any whitespace in front of it.
//
// The index has an xml declaration and three spaces of indentation before its
// root. The brands map has no declaration at all and starts on <urlset. The
// search feed, in the other file, has no declaration either. Nothing here may
// depend on a declaration being present.
func rootElement(body []byte) string {
	s := string(body)
	for {
		// The byte order mark is in that cut set because a server that sends
		// one in front of an xml declaration is not doing anything wrong and it
		// would otherwise be read as the first element's name.
		s = strings.TrimLeft(s, " \t\r\n\uFEFF")
		switch {
		case strings.HasPrefix(s, "<?"):
			end := strings.Index(s, "?>")
			if end < 0 {
				return ""
			}
			s = s[end+2:]
		case strings.HasPrefix(s, "<!--"):
			end := strings.Index(s, "-->")
			if end < 0 {
				return ""
			}
			s = s[end+3:]
		case strings.HasPrefix(s, "<!"):
			end := strings.IndexByte(s, '>')
			if end < 0 {
				return ""
			}
			s = s[end+1:]
		case strings.HasPrefix(s, "<"):
			name := s[1:]
			if i := strings.IndexAny(name, " \t\r\n>/"); i >= 0 {
				name = name[:i]
			}
			if i := strings.IndexByte(name, ':'); i >= 0 {
				name = name[i+1:]
			}
			return strings.ToLower(name)
		default:
			return ""
		}
	}
}

// SniffForm decides which of the three shapes a body is, from its first
// element. A body with no element at all is text, which is what the shops map
// is and what an empty file would also be, so the parse of a text map still has
// to hold up when there is nothing in it.
func SniffForm(body []byte) Form {
	switch rootElement(body) {
	case "sitemapindex":
		return FormIndex
	case "urlset":
		return FormURLSet
	default:
		return FormText
	}
}

// ParseSitemap reads a fetched sitemap in whichever form it arrived in.
func ParseSitemap(resp *Response) (*Sitemap, error) {
	if resp == nil {
		return nil, errors.New("no response to parse")
	}
	if resp.Status == StatusChallenged {
		return nil, fmt.Errorf("%s: the edge served a challenge instead of the sitemap", resp.URL)
	}
	if resp.Status == StatusNotFound {
		return nil, fmt.Errorf("%s: there is no such sitemap", resp.URL)
	}

	s := &Sitemap{
		URL:      resp.URL,
		Form:     SniffForm(resp.Body),
		Envelope: Envelope{Tier: "sitemap"},
	}
	s.Envelope.record(resp)

	switch s.Form {
	case FormIndex:
		if err := s.readIndex(resp.Body); err != nil {
			return nil, err
		}
	case FormURLSet:
		if err := s.readURLSet(resp.Body); err != nil {
			return nil, err
		}
	default:
		s.readText(resp.Body)
	}
	s.Envelope.sortMissed()
	return s, nil
}

func (s *Sitemap) readIndex(body []byte) error {
	var doc xmlIndex
	if err := xml.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("%s: %w: %v", s.URL, ErrNotASitemap, err)
	}
	s.Envelope.carry("children", "sitemap:sitemapindex/sitemap/loc")
	s.Envelope.carry("bucket", "sitemap:the child's file name")

	var undated, noLastMod int
	for _, sm := range doc.Sitemaps {
		loc := strings.TrimSpace(sm.Loc)
		if loc == "" {
			continue
		}
		child := Child{
			URL:     loc,
			Name:    baseName(loc),
			LastMod: strings.TrimSpace(sm.LastMod),
		}
		bucket, shard, ok := ParseChildName(child.Name)
		if ok {
			child.Bucket, child.Shard = bucket, shard
		} else {
			undated++
		}
		if child.LastMod == "" {
			noLastMod++
		}
		s.Children = append(s.Children, child)
	}

	if len(s.Children) == 0 {
		s.Envelope.miss("children", "the index parsed and held no child sitemaps, which is a shape change rather than an empty site")
		return nil
	}
	if noLastMod > 0 {
		s.Envelope.miss("lastmod", fmt.Sprintf("absent on %d of %d children, which is every month and year shard, because the site knows the bucket those are filed under and not a day", noLastMod, len(s.Children)))
	} else {
		s.Envelope.carry("lastmod", "sitemap:sitemapindex/sitemap/lastmod")
	}
	if undated > 0 {
		s.Envelope.miss("bucket", fmt.Sprintf("%d of %d child names carry no date, which is what the subjects index holds and what a fifth name shape in the root index would also look like", undated, len(s.Children)))
	}
	return nil
}

func (s *Sitemap) readURLSet(body []byte) error {
	var doc xmlURLSet
	if err := xml.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("%s: %w: %v", s.URL, ErrNotASitemap, err)
	}
	s.Envelope.carry("urls", "sitemap:urlset/url/loc")
	s.Envelope.carry("kind", "sitemap:the url's first path segment")

	var noLastMod, unknownKind, alternates int
	for _, u := range doc.URLs {
		loc := strings.TrimSpace(u.Loc)
		if loc == "" {
			continue
		}
		e := Entry{URL: loc, Kind: EntryKind(loc), LastMod: strings.TrimSpace(u.LastMod)}
		if e.LastMod == "" {
			noLastMod++
		}
		if e.Kind == "" {
			unknownKind++
		}
		alternates += len(u.Links)
		s.Entries = append(s.Entries, e)
	}

	// An empty urlset is how a shard that does not exist answers, at 200 ok and
	// 108 bytes. Saying so here is what keeps a run that asked for a shard past
	// the end from reading as a run that found nothing published.
	if len(s.Entries) == 0 {
		s.Envelope.miss("urls", "the map is a well formed urlset with nothing in it, which is what this site serves for a shard that does not exist")
		return nil
	}
	switch {
	case noLastMod == len(s.Entries):
		s.Envelope.miss("lastmod", "no entry in this map carries one, which is normal on a month or year shard")
	case noLastMod > 0:
		s.Envelope.miss("lastmod", fmt.Sprintf("absent on %d of %d entries", noLastMod, len(s.Entries)))
	default:
		s.Envelope.carry("lastmod", "sitemap:urlset/url/lastmod")
	}
	if unknownKind > 0 {
		s.Envelope.miss("kind", fmt.Sprintf("%d of %d urls are under a path this tool has no name for, so they are listed with no kind rather than filed under a wrong one", unknownKind, len(s.Entries)))
	}
	if alternates > 0 {
		s.Envelope.Unread = append(s.Envelope.Unread, fmt.Sprintf("%d xhtml:link alternates, which are these same pages in other languages", alternates))
	}
	return nil
}

// readText reads one url per line, which is the shops map. Blank lines and
// comments are skipped, and anything that is not a url is skipped and counted,
// because a text map has no structure to fail against and the only way it can
// say it changed shape is by holding lines that are not urls.
func (s *Sitemap) readText(body []byte) {
	s.Envelope.carry("urls", "sitemap:one url per line of text/plain")
	var notURLs int
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			notURLs++
			continue
		}
		s.Entries = append(s.Entries, Entry{URL: line, Kind: EntryKind(line)})
	}
	if len(s.Entries) == 0 {
		s.Envelope.miss("urls", "the map holds no urls at all")
	}
	if notURLs > 0 {
		s.Envelope.miss("urls", fmt.Sprintf("%d lines are not urls and were skipped", notURLs))
	}
}

// Select returns the children whose bucket overlaps a window, keeping the
// index's own order, which is newest first.
func (s *Sitemap) Select(since, until time.Time) []Child {
	out := make([]Child, 0, len(s.Children))
	for _, c := range s.Children {
		if c.Bucket.InWindow(since, until) {
			out = append(out, c)
		}
	}
	return out
}

// Span is the oldest and newest bucket in an index, which is the honest way to
// say how far back the enumeration reaches.
func (s *Sitemap) Span() (oldest, newest Bucket) {
	for _, c := range s.Children {
		if !c.Dated() {
			continue
		}
		if oldest.Zero() || c.Bucket.Start.Before(oldest.Start) {
			oldest = c.Bucket
		}
		if newest.Zero() || c.Bucket.Start.After(newest.Start) {
			newest = c.Bucket
		}
	}
	return oldest, newest
}

// Shapes counts the children by the precision of their bucket, which is the one
// summary of an index that says something a count of children does not: how
// much of what this site publishes is dated only to a month or a year.
func (s *Sitemap) Shapes() map[Precision]int {
	out := map[Precision]int{}
	for _, c := range s.Children {
		if c.Dated() {
			out[c.Bucket.Precision]++
		}
	}
	return out
}

// baseName is the file name at the end of a url, which is where a child's
// bucket is written.
func baseName(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.LastIndexByte(raw, '/'); i >= 0 {
		return raw[i+1:]
	}
	return raw
}
