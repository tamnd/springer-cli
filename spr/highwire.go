package spr

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Rung 1. Three bibliographic vocabularies in one head.
//
// The open access article capture carries 66 distinct meta names: 40 Highwire
// citation_* names, 14 Dublin Core, 12 PRISM. Between them they state the doi,
// the title, every author with institution and email, the journal title and
// abbreviation, both issns, volume, issue, first and last page, four separate
// dates, language, publisher, the pdf url, the api url, the article type, the
// section within the issue, copyright, rights, the licence, five subjects, the
// access declaration, and 122 references.
//
// That is most of a work record from one rung and one request, which is why
// this file is the shortest of the four and does the most work.
//
// The three vocabularies mostly say the same things. That is a reason to read
// all three and notice when they stop agreeing, not a reason to read one.

// Vocabulary names a family of meta names.
type Vocabulary string

const (
	VocabHighwire    Vocabulary = "highwire"
	VocabDublinCore  Vocabulary = "dc"
	VocabPRISM       Vocabulary = "prism"
	VocabUnclassifed Vocabulary = "other"
)

// Meta is the head of a document, read once and asked many times.
//
// It keeps every value for every name rather than the last one, because
// citation_author, citation_reference and dc.subject are repeated names whose
// repetition is the data. It also keeps document order, so that the parallel
// Highwire author arrays can be lined up positionally when JSON-LD is missing
// and there is nothing better to line them up by.
type Meta struct {
	order  []string
	values map[string][]string
}

// ParseMeta reads every named meta tag in the document.
//
// A tag that is present with an empty value is recorded as present with an
// empty value, and is not the same thing as a tag that is absent. Springer
// ships citation_fulltext_world_readable present and empty on both article
// captures and does not ship it at all on a chapter, a protocol or an entry,
// so the two cases have to stay distinguishable or a tag going away looks the
// same as a tag that was always going to be empty.
func ParseMeta(root *html.Node) *Meta {
	m := &Meta{values: map[string][]string{}}
	walk(root, func(n *html.Node) bool {
		if n.DataAtom != atom.Meta {
			return true
		}
		name := attr(n, "name")
		if name == "" {
			return true
		}
		if _, seen := m.values[name]; !seen {
			m.order = append(m.order, name)
		}
		m.values[name] = append(m.values[name], attr(n, "content"))
		return true
	})
	return m
}

// Has reports whether the name was declared at all, empty or not.
func (m *Meta) Has(name string) bool {
	_, ok := m.values[name]
	return ok
}

// Empty reports whether the name was declared and every value it carried was
// empty. This is the present-and-empty case, recorded as a fact of its own.
func (m *Meta) Empty(name string) bool {
	vs, ok := m.values[name]
	if !ok {
		return false
	}
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// First returns the first non empty value for a name, or the empty string.
func (m *Meta) First(names ...string) string {
	for _, name := range names {
		for _, v := range m.values[name] {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// All returns every value for a name, in document order, with the empty ones
// dropped. A caller that needs to know an empty one was there asks Empty.
func (m *Meta) All(name string) []string {
	var out []string
	for _, v := range m.values[name] {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Names returns every declared name in document order.
func (m *Meta) Names() []string { return append([]string(nil), m.order...) }

// Vocabularies returns which of the three families this page carries, in
// ladder order. A work page has all three, a container page has none of them,
// and a work page that has lost one is drift worth a report before the fields
// that depended on it silently empty.
func (m *Meta) Vocabularies() []Vocabulary {
	seen := map[Vocabulary]bool{}
	for _, n := range m.order {
		seen[vocabOf(n)] = true
	}
	var out []Vocabulary
	for _, v := range []Vocabulary{VocabHighwire, VocabDublinCore, VocabPRISM} {
		if seen[v] {
			out = append(out, v)
		}
	}
	return out
}

func vocabOf(name string) Vocabulary {
	switch {
	case strings.HasPrefix(name, "citation_"), name == "access", name == "DOI":
		return VocabHighwire
	case strings.HasPrefix(name, "dc."):
		return VocabDublinCore
	case strings.HasPrefix(name, "prism."):
		return VocabPRISM
	}
	return VocabUnclassifed
}

// Divergence is one fact that two vocabularies state differently.
//
// It is not an error and this package never resolves one. A page saying two
// different things about one fact is a decision somebody made, and the useful
// response is to put both claims in front of a person rather than to pick.
type Divergence struct {
	Fact   string            `json:"fact"`
	Claims map[string]string `json:"claims"`
}

// crossChecks are the facts more than one vocabulary declares. Each entry is
// the fact name, the meta name each vocabulary uses for it, and how to put two
// claims into the same shape before they are compared.
var crossChecks = []struct {
	fact  string
	names map[Vocabulary]string
	norm  func(string) string
}{
	{fact: "title", names: map[Vocabulary]string{VocabHighwire: "citation_title", VocabDublinCore: "dc.title"}},

	// prism.doi carries the doi: scheme and citation_doi does not, on every
	// work capture. That is one fact written two correct ways, so the scheme
	// comes off before the comparison. Reporting it would put a divergence on
	// every page and teach everybody to ignore the list.
	{fact: "doi", names: map[Vocabulary]string{VocabHighwire: "citation_doi", VocabPRISM: "prism.doi"}, norm: dropDOIScheme},
	{fact: "journal", names: map[Vocabulary]string{VocabHighwire: "citation_journal_title", VocabPRISM: "prism.publicationName"}},
	{fact: "issn", names: map[Vocabulary]string{VocabHighwire: "citation_issn", VocabPRISM: "prism.issn"}},
	{fact: "volume", names: map[Vocabulary]string{VocabHighwire: "citation_volume", VocabPRISM: "prism.volume"}},
	{fact: "issue", names: map[Vocabulary]string{VocabHighwire: "citation_issue", VocabPRISM: "prism.number"}},
	{fact: "first_page", names: map[Vocabulary]string{VocabHighwire: "citation_firstpage", VocabPRISM: "prism.startingPage"}},
	{fact: "last_page", names: map[Vocabulary]string{VocabHighwire: "citation_lastpage", VocabPRISM: "prism.endingPage"}},
	{fact: "language", names: map[Vocabulary]string{VocabHighwire: "citation_language", VocabDublinCore: "dc.language"}},
	{fact: "copyright", names: map[Vocabulary]string{VocabDublinCore: "dc.copyright", VocabPRISM: "prism.copyright"}},
	{fact: "rights_agent", names: map[Vocabulary]string{VocabDublinCore: "dc.rightsAgent", VocabPRISM: "prism.rightsAgent"}},
}

// CrossCheck compares every fact that more than one vocabulary declares and
// returns the ones where the declarations disagree.
//
// A fact only one vocabulary states is not checked and is not a disagreement.
// Comparison is case insensitive and ignores surrounding whitespace, because a
// vocabulary that title cases where another sentence cases is a formatting
// difference and reporting it would bury the real ones.
func (m *Meta) CrossCheck() []Divergence {
	var out []Divergence
	for _, c := range crossChecks {
		claims := map[string]string{}
		var norm []string
		for v, name := range c.names {
			got := m.First(name)
			if got == "" {
				continue
			}
			claims[string(v)+":"+name] = got
			cmp := got
			if c.norm != nil {
				cmp = c.norm(cmp)
			}
			norm = append(norm, strings.ToLower(cmp))
		}
		if len(norm) < 2 {
			continue
		}
		agreed := true
		for _, n := range norm[1:] {
			if n != norm[0] {
				agreed = false
			}
		}
		if !agreed {
			out = append(out, Divergence{Fact: c.fact, Claims: claims})
		}
	}
	return out
}

// dropDOIScheme removes the doi: prefix that PRISM writes and Highwire does
// not. It is a scheme on an identifier and not part of the identifier.
func dropDOIScheme(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "doi:")
}

// stripAPIKey blanks the api_key parameter on a url.
//
// It started as one line of hygiene for the citation_springer_api_url meta tag,
// which ships with the parameter present and empty, so on that path it changes
// nothing. It is now the single chokepoint every url in this package passes
// through before it is cached, printed or recorded, because the Springer Nature
// API takes its key in the query string and there must be no path from a
// configured key to a file on disk or a line on a terminal.
//
// The parameter is blanked and not removed. api_key= says a credential was
// there and was taken out, and removing it would say there never was one.
func stripAPIKey(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if !q.Has("api_key") {
		return raw
	}
	q.Set("api_key", "")
	u.RawQuery = q.Encode()
	return u.String()
}
