package spr

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Rung 2. The schema.org block.
//
// It is parsed into a typed document rather than reached into by path
// expression. A path expression is quick to write and gives you no signal when
// the shape changes: mainEntity.author[0].affiliation[0].address.name returns
// nothing whether the page dropped affiliations or renamed the wrapper, and the
// two are very different pieces of news. Typed fields make the shape explicit
// and put the accommodations for real variation, of which there are three, in
// named places.
//
// The three accommodations, all measured:
//
//   - The wrapper varies. An article ships {"@type": "WebPage", "mainEntity":
//     {"@type": "ScholarlyArticle", ...}}. A chapter, a protocol and a
//     reference work entry ship the ScholarlyArticle at top level with no
//     wrapper at all.
//   - @type is sometimes a string and sometimes a list. isPartOf on the
//     article is ["Periodical", "PublicationVolume"].
//   - keywords is sometimes a list of 21 strings and sometimes one comma
//     joined string. The protocol capture ships "NCBI, GenBank, DNA sequence,
//     BLAST, SRA" as a single value.
//
// A book page carries three blocks rather than one, and a volumes and issues
// page carries none. Both are handled by returning every block and letting the
// caller ask for the one it wants.

// flexStrings is a JSON value that is either a string or a list of strings.
type flexStrings struct {
	values []string
	single bool
}

func (f *flexStrings) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*f = flexStrings{values: []string{one}, single: true}
		return nil
	}
	var many []json.RawMessage
	if err := json.Unmarshal(b, &many); err != nil {
		return nil // an unexpected shape is not a parse failure for the page
	}
	out := make([]string, 0, len(many))
	for _, raw := range many {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			out = append(out, s)
			continue
		}
		// A list of objects, each of which may name itself. The image list on
		// an article is a list of urls, but the pattern shows up elsewhere.
		var o struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := json.Unmarshal(raw, &o); err == nil {
			if o.Name != "" {
				out = append(out, o.Name)
			} else if o.URL != "" {
				out = append(out, o.URL)
			}
		}
	}
	*f = flexStrings{values: out}
	return nil
}

// List returns the values as they arrived.
func (f flexStrings) List() []string { return f.values }

// First returns the first value, or the empty string.
func (f flexStrings) First() string {
	if len(f.values) == 0 {
		return ""
	}
	return f.values[0]
}

// Split returns the values with a single comma joined string broken up, which
// is the shape keywords arrives in on a chapter, a protocol and an entry. A
// value that arrived as a list is returned untouched, because a keyword is
// allowed to contain a comma and splitting a list that did not need it would
// invent terms nobody wrote.
func (f flexStrings) Split() []string {
	if !f.single || len(f.values) != 1 {
		return f.values
	}
	parts := strings.Split(f.values[0], ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ldEntity is a schema.org node. The fields are the ones the captures carry.
type ldEntity struct {
	Type        flexStrings `json:"@type"`
	Headline    string      `json:"headline"`
	Name        string      `json:"name"`
	Description string      `json:"description"`

	// Genre is a list on a chapter, ["Economics and Finance", "Economics and
	// Finance (R0)"], and a string elsewhere. It is Springer's own subject
	// coding and the parenthesised entry is the coarse bucket.
	Genre flexStrings `json:"genre"`

	DatePublished string `json:"datePublished"`
	DateModified  string `json:"dateModified"`

	PageStart string `json:"pageStart"`
	PageEnd   string `json:"pageEnd"`

	License string      `json:"license"`
	SameAs  flexStrings `json:"sameAs"`

	Keywords flexStrings `json:"keywords"`
	Image    flexStrings `json:"image"`

	// ISBN and CopyrightYear appear on a book block and nowhere else. The isbn
	// there is the electronic edition, which is the one the doi resolves to, and
	// not the hardcover the page prices beside it.
	ISBN          flexStrings `json:"isbn"`
	CopyrightYear string      `json:"copyrightYear"`
	NumberOfPages int         `json:"numberOfPages"`

	IsAccessibleForFree *bool `json:"isAccessibleForFree"`

	Author    []ldPerson `json:"author"`
	Editor    []ldPerson `json:"editor"`
	Publisher *ldOrg     `json:"publisher"`
	IsPartOf  *ldPart    `json:"isPartOf"`

	MainEntity *ldEntity `json:"mainEntity"`
}

type ldPerson struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Email       string  `json:"email"`
	Affiliation []ldOrg `json:"affiliation"`
}

type ldOrg struct {
	Name    string     `json:"name"`
	Address *ldAddress `json:"address"`
}

type ldAddress struct {
	Name string `json:"name"`
}

type ldPart struct {
	Name         string      `json:"name"`
	ISSN         flexStrings `json:"issn"`
	ISBN         flexStrings `json:"isbn"`
	VolumeNumber string      `json:"volumeNumber"`
	IssueNumber  string      `json:"issueNumber"`
	Type         flexStrings `json:"@type"`
}

// is reports whether the entity declares this schema.org type, which may have
// arrived as a bare string or inside a list.
func (e *ldEntity) is(want string) bool {
	if e == nil {
		return false
	}
	for _, t := range e.Type.List() {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

// title returns the entity's own name under whichever of the two keys it used.
// Springer writes headline on works and name on containers.
func (e *ldEntity) title() string {
	if e == nil {
		return ""
	}
	if e.Headline != "" {
		return e.Headline
	}
	return e.Name
}

// linkData is every JSON-LD block on one page, in document order, with the
// bytes of each kept beside the parse.
type linkData struct {
	blocks []*ldEntity
	raw    []json.RawMessage
	broken int
}

// ParseLinkData reads every application/ld+json block in the document.
//
// A block that does not parse is counted and skipped rather than failing the
// page. The count reaches the envelope, because a block that stopped parsing is
// news even when nothing needed it.
func parseLinkData(root *html.Node) *linkData {
	ld := &linkData{}
	walk(root, func(n *html.Node) bool {
		if n.DataAtom != atom.Script || !strings.Contains(attr(n, "type"), "ld+json") {
			return true
		}
		var buf strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				buf.WriteString(c.Data)
			}
		}
		var e ldEntity
		if err := json.Unmarshal([]byte(buf.String()), &e); err != nil {
			ld.broken++
			return true
		}
		ld.blocks = append(ld.blocks, &e)
		ld.raw = append(ld.raw, json.RawMessage(buf.String()))
		return true
	})
	return ld
}

// workTypes are the schema.org types a Springer work page declares for itself.
var workTypes = []string{"ScholarlyArticle", "Article", "Chapter"}

// work returns the block describing the work, unwrapping the mainEntity form
// used on articles and taking the top level form used on chapters, protocols
// and entries. It returns nil on a page that carries no work block, which is
// every container page and is a normal outcome rather than a failure.
func (ld *linkData) work() *ldEntity {
	if ld == nil {
		return nil
	}
	for _, b := range ld.blocks {
		if b.MainEntity != nil {
			return b.MainEntity
		}
	}
	for _, b := range ld.blocks {
		for _, t := range workTypes {
			if b.is(t) {
				return b
			}
		}
	}
	return nil
}

// bookTypes are the schema.org types a container page declares for itself.
// Product is in the list because a book page ships three blocks, one Book and
// two identical Products, and a page that ever drops the Book block still
// states its isbn in the Product.
var bookTypes = []string{"Book", "PublicationVolume", "Periodical"}

// book returns the block describing the container, preferring a real Book over
// the Product offers that follow it. It returns nil on a journal or a series
// page, whose only JSON-LD describes the web page rather than the thing, which
// is a normal outcome and not a failure.
func (ld *linkData) book() *ldEntity {
	if ld == nil {
		return nil
	}
	for _, b := range ld.blocks {
		for _, t := range bookTypes {
			if b.is(t) {
				return b
			}
		}
	}
	for _, b := range ld.blocks {
		if b.is("Product") && b.ISBN.First() != "" {
			return b
		}
	}
	return nil
}

// count is how many blocks the page carried, which the capture ledger tracks.
func (ld *linkData) count() int {
	if ld == nil {
		return 0
	}
	return len(ld.blocks)
}
