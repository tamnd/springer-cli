package spr

import "sort"

// Level is a rung on the extraction ladder. Four rungs, tried in order, and the
// first one that answers wins.
//
// The ordering is the one decision in this file worth arguing about, because it
// puts a 2005 era meta tag convention above a typed schema.org graph.
//
// Highwire meta tags are what Google Scholar reads. A publisher who breaks them
// stops being indexed, hears about it within days, and fixes it. JSON-LD has no
// equivalent forcing function, and its shape here varies by content type: one
// block on an article wrapped in mainEntity, one block at top level on a
// chapter, three on a book, and none at all on the volumes and issues page. So
// rung 1 is the one with an external party checking it, and that is worth more
// than better structure.
//
// Authors are the deliberate exception and come from rung 2. Highwire emits
// three parallel arrays for name, institution and email, and their alignment is
// positional. It breaks the moment one author has two affiliations, which is
// common, and it breaks silently, which is worse. JSON-LD binds the three to
// the right person.
type Level int

const (
	// LevelNone means no rung answered.
	LevelNone Level = iota

	// LevelHighwire is <meta name="citation_*">, dc.* and prism.*.
	LevelHighwire

	// LevelLinkedData is the schema.org JSON-LD block.
	LevelLinkedData

	// LevelRegion is Springer's own data-test and data-title attributes.
	LevelRegion

	// LevelSelector is CSS class names. Presentational, changes with a
	// redesign, and the last thing to try.
	LevelSelector
)

// String names the rung the way envelope.via writes it.
func (l Level) String() string {
	switch l {
	case LevelHighwire:
		return "highwire"
	case LevelLinkedData:
		return "linkdata"
	case LevelRegion:
		return "region"
	case LevelSelector:
		return "selector"
	default:
		return "none"
	}
}

// Field is one row of the extraction table. Every field this tool fills has a
// row, and the row says which rung is expected to answer, which exact tag or
// region that is, and why it sits there.
//
// The why column is not documentation for its own sake. When a field starts
// coming back empty, the first question is always whether the page moved or the
// extractor was always wrong, and a row that states its reasoning answers that
// without an archaeology session in the git history.
type Field struct {
	Name   string `json:"name"`
	Rung   Level  `json:"rung"`
	Source string `json:"source"`
	Why    string `json:"why"`
}

// Fields is the table, in the order spr extraction prints it.
var Fields = []Field{
	{"doi", LevelHighwire, "citation_doi", "the work's permanent identifier, and the only one that survives a url change"},
	{"type", LevelHighwire, "citation_article_type + the url path", "the path says article, chapter, protocol or entry when the tag is absent"},
	{"title", LevelHighwire, "citation_title", "corroborated by dc.title and jsonld:headline"},
	{"abstract", LevelHighwire, "dc.description", "corroborated by jsonld:description, and both are present and empty on an entry"},
	{"language", LevelHighwire, "citation_language", "stated once, in one vocabulary"},
	{"keywords", LevelLinkedData, "keywords", "the authors' own terms, and no meta vocabulary carries them"},
	{"subjects", LevelHighwire, "dc.subject", "Springer's classification, a different list from keywords and kept apart"},
	{"authors", LevelLinkedData, "author[]", "the only source binding orcid, affiliation and email to the right person"},
	{"corresponding", LevelHighwire, "citation_author_email", "the marked corresponding addresses, as printed"},
	{"container_title", LevelHighwire, "citation_journal_title or citation_inbook_title", "which of the two is present is itself the content type signal"},
	{"issn", LevelHighwire, "citation_issn + jsonld:isPartOf.issn", "a journal has a print and an electronic issn and the page gives both"},
	{"isbn", LevelHighwire, "citation_isbn", "on chapters, protocols and entries, never on an article"},
	{"volume", LevelHighwire, "citation_volume", "corroborated by prism.volume"},
	{"issue", LevelHighwire, "citation_issue", "corroborated by prism.number"},
	{"pages", LevelHighwire, "citation_firstpage + citation_lastpage", "derived only when both ends parse, never guessed from one"},
	{"published", LevelHighwire, "citation_publication_date", "when the issue is dated, which is not when it went online"},
	{"online", LevelHighwire, "citation_online_date", "when it went online, which is not the cover date"},
	{"cover_date", LevelHighwire, "citation_cover_date", "what the cover says, which is neither of the other two"},
	{"modified", LevelLinkedData, "dateModified", "no meta vocabulary states it"},
	{"license", LevelLinkedData, "license", "the licence url, present only where one was granted"},
	{"copyright", LevelHighwire, "dc.copyright", "corroborated by prism.copyright"},
	{"access", LevelHighwire, "access + jsonld:isAccessibleForFree", "two independent declarations, and they agreed on all five work captures"},
	{"pdf_url", LevelHighwire, "citation_pdf_url", "the publisher's own pointer, which is not always the url you would build"},
	{"api_url", LevelHighwire, "citation_springer_api_url", "the publisher's api pointer, with the api key stripped before it is stored"},
	{"sections", LevelRegion, "section[data-title]", "the section tree, and there are no id attributes on this page to hang it on"},
	{"figures", LevelRegion, "[data-test=figure]", "label, caption and description all sit on the article page already"},
	{"references", LevelHighwire, "citation_reference", "122 on the open access capture, and zero on a chapter, a protocol or an entry"},
	{"ref_links", LevelSelector, ".c-article-references__links a", "the resolver links, 68 of 122, each naming its kind in data-track-action"},
	{"equations", LevelSelector, ".c-article-equation", "a count, and nothing above rung 4 states it"},
	{"footnotes", LevelSelector, ".c-article-footnote--listed__item", "the listed notes, 24 on the open access capture"},
	{"rails", LevelRegion, "section[data-title=Inline Recommendations]", "the recommender strip, pulled out so the section tree does not carry an advert"},
}

// FieldByName returns the table row for a field name.
func FieldByName(name string) (Field, bool) {
	for _, f := range Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// FieldsAt returns every field expected to be answered by one rung, which is
// what spr extraction --rung prints and what tells you at a glance how much of
// a record would survive a vocabulary disappearing.
func FieldsAt(l Level) []Field {
	var out []Field
	for _, f := range Fields {
		if f.Rung == l {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
