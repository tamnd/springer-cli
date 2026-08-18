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
//
// Record says which record the row belongs to, and it exists because the same
// field name means different things on different pages. Title on a work comes
// from citation_title on rung 1, and title on a journal comes from an analytics
// payload on rung 3, because a journal page ships no bibliographic meta tags at
// all. One table with no record column would have to pick one of those and be
// wrong about the other.
type Field struct {
	Record string `json:"record"`
	Name   string `json:"name"`
	Rung   Level  `json:"rung"`
	Source string `json:"source"`
	Why    string `json:"why"`
}

// Qualified is the row's full name, journal.title, which is what tells two rows
// with the same field name apart.
func (f Field) Qualified() string { return f.Record + "." + f.Name }

// Fields is the table, in the order spr extraction prints it.
var Fields = []Field{
	{"work", "doi", LevelHighwire, "citation_doi", "the work's permanent identifier, and the only one that survives a url change"},
	{"work", "type", LevelHighwire, "citation_article_type + the url path", "the path says article, chapter, protocol or entry when the tag is absent"},
	{"work", "title", LevelHighwire, "citation_title", "corroborated by dc.title and jsonld:headline"},
	{"work", "abstract", LevelHighwire, "dc.description", "corroborated by jsonld:description, and both are present and empty on an entry"},
	{"work", "language", LevelHighwire, "citation_language", "stated once, in one vocabulary"},
	{"work", "keywords", LevelLinkedData, "keywords", "the authors' own terms, and no meta vocabulary carries them"},
	{"work", "subjects", LevelHighwire, "dc.subject", "Springer's classification, a different list from keywords and kept apart"},
	{"work", "authors", LevelLinkedData, "author[]", "the only source binding orcid, affiliation and email to the right person"},
	{"work", "corresponding", LevelHighwire, "citation_author_email", "the marked corresponding addresses, as printed"},
	{"work", "container_title", LevelHighwire, "citation_journal_title or citation_inbook_title", "which of the two is present is itself the content type signal"},
	{"work", "issn", LevelHighwire, "citation_issn + jsonld:isPartOf.issn", "a journal has a print and an electronic issn and the page gives both"},
	{"work", "isbn", LevelHighwire, "citation_isbn", "on chapters, protocols and entries, never on an article"},
	{"work", "volume", LevelHighwire, "citation_volume", "corroborated by prism.volume"},
	{"work", "issue", LevelHighwire, "citation_issue", "corroborated by prism.number"},
	{"work", "pages", LevelHighwire, "citation_firstpage + citation_lastpage", "derived only when both ends parse, never guessed from one"},
	{"work", "published", LevelHighwire, "citation_publication_date", "when the issue is dated, which is not when it went online"},
	{"work", "online", LevelHighwire, "citation_online_date", "when it went online, which is not the cover date"},
	{"work", "cover_date", LevelHighwire, "citation_cover_date", "what the cover says, which is neither of the other two"},
	{"work", "modified", LevelLinkedData, "dateModified", "no meta vocabulary states it"},
	{"work", "license", LevelLinkedData, "license", "the licence url, present only where one was granted"},
	{"work", "copyright", LevelHighwire, "dc.copyright", "corroborated by prism.copyright"},
	{"work", "access", LevelHighwire, "access + jsonld:isAccessibleForFree", "two independent declarations, and they agreed on all five work captures"},
	{"work", "pdf_url", LevelHighwire, "citation_pdf_url", "the publisher's own pointer, which is not always the url you would build"},
	{"work", "api_url", LevelHighwire, "citation_springer_api_url", "the publisher's api pointer, with the api key stripped before it is stored"},
	{"work", "sections", LevelRegion, "section[data-title]", "the section tree, and there are no id attributes on this page to hang it on"},
	{"work", "figures", LevelRegion, "[data-test=figure] and its bottom-caption", "the label and the prose are two elements, and the first of them prints Fig. 1 rather than a caption"},
	{"work", "tables", LevelRegion, "[data-test=inline-table]", "the caption and the link, because the page announces its tables and publishes zero table elements"},
	{"work", "references", LevelHighwire, "citation_reference", "122 on the open access capture, and zero on a chapter, a protocol or an entry"},
	{"work", "ref_links", LevelSelector, ".c-article-references__links a", "the resolver links, 68 of 122, each naming its kind in data-track-action"},
	{"work", "equations", LevelSelector, ".c-article-equation", "a count, and nothing above rung 4 states it"},
	{"work", "footnotes", LevelSelector, ".c-article-footnote--listed__item", "the listed notes, 24 on the open access capture"},
	{"work", "rails", LevelRegion, "section[data-title=Inline Recommendations]", "the recommender strip, pulled out so the section tree does not carry an advert"},

	// The containers. Every row below is rung 3 or rung 4 and that is not a
	// shortcut taken here, it is what the pages are: a journal home page ships
	// 8 meta names, a book page 8, a series page 9 and a volumes page 8, and
	// none of those heads carry a bibliographic vocabulary. The ladder that
	// arbitrates a work record has nothing to arbitrate here.
	//
	// Where a page ships an analytics payload that parses, the payload outranks
	// a printed region and is recorded as rung 3, because it is the publisher's
	// own machine readable statement rather than a rendered label read back out
	// of its own prose. Every page on this site ships two of them: an
	// assignment, window.dataLayer = [{...}], which is strict JSON and parses,
	// and a push, window.dataLayer.push({...}), which is JavaScript with single
	// quotes and does not. The split is by form and not by page.

	{"journal", "springer_id", LevelRegion, "datalayer.Journal Id", "the journal number in the url, publisher scoped and meaningless outside Springer"},
	{"journal", "title", LevelRegion, "datalayer.Journal Title", "the head carries no citation_journal_title on a journal's own page"},
	{"journal", "electronic_issn", LevelRegion, "[data-test=springer-electronic-issn] dd", "read from the dd, since the region's whole text glues the label to the number"},
	{"journal", "print_issn", LevelRegion, "[data-test=springer-print-issn] dd", "a second number for the same journal, and neither of the two is canonical"},
	{"journal", "publishing_model", LevelRegion, "datalayer.Publishing Model", "the one field that makes the analytics payload worth parsing, and nothing else states it"},
	{"journal", "subjects", LevelRegion, "datalayer.content.category.snt", "Springer's classification of the journal, and the page prints no subject list anywhere"},
	{"journal", "editors", LevelRegion, "[data-test=journal-editor-links]", "one dl per role, and the role travels with the name rather than being flattened to editor"},
	{"journal", "metrics", LevelRegion, "[data-test=impact-factor-value] and its three siblings", "label and value are separate elements, so they are paired by name and not by position"},
	{"journal", "indexed_in", LevelSelector, "[data-test=about-this-journal] .c-list-description__item", "matched on its printed English label, which is the weakest match this tool makes"},
	{"journal", "about", LevelRegion, "[data-test=darwin-journal-homepage-promo-text]", "the real prose, and not the journal information block, which is the issns and the indexing list"},

	{"book", "doi", LevelHighwire, "doi", "one of the three meta names on a book page that says anything bibliographic"},
	{"book", "isbn_electronic", LevelSelector, "the bibliographic table, eBook ISBN", "the edition the doi resolves to, and the book's identity on this site"},
	{"book", "isbn_print", LevelRegion, "datalayer.content.book.pisbn", "a fourth number under a fourth name, equal to the softcover on the measured capture"},
	{"book", "product_type", LevelRegion, "datalayer.content.book.bookProductType", "Springer's own classification, Monograph on the measured capture, and no region states it"},
	{"book", "series", LevelRegion, "datalayer.content.book.seriesId + [data-test=series-link]", "the id comes from the payload and the name from the printed line, and neither alone is enough"},
	{"book", "edition", LevelSelector, "the bibliographic table, Edition Number", "nothing above rung 4 states an edition, a page extent, a publisher or a series issn"},
	{"book", "published", LevelRegion, "[data-test=electronic_isbn_publication_date]", "three editions with three dates a year apart, so they are three fields and not one"},
	{"book", "chapters", LevelRegion, "[data-test=chapter]", "the page range is read from inside each row, not zipped from a flat list of the same length"},
	{"book", "offers", LevelRegion, "[data-test=buy-box-mobile] .buying-option", "the kind comes from the order form's hidden type field, since the printed label is localized prose"},
	{"book", "access", LevelHighwire, "access + datalayer.hasAccess", "two declarations that agreed on the measured capture, and a disagreement is recorded rather than resolved"},

	{"series", "series_id", LevelRegion, "datalayer.Book Series Id", "arrives as a JSON number here and as a string on a journal, so the reader accepts both"},
	{"series", "title", LevelRegion, "datalayer.Book Series Title", "the thinnest payload on the site states this and the id and nothing else"},
	{"series", "editors", LevelRegion, "[data-test=editor-links-1]", "the same dl per role shape a journal uses, with Series Editor in place of Editor-in-Chief"},
	{"series", "latest_titles", LevelRegion, "[data-test=latest-titles] .c-card", "five books out of many thousands, which is why the record names them latest rather than titles"},
	{"series", "indexed_in", LevelRegion, "[data-test=series-abstract-and-index-services-list]", "the same claim a journal makes about itself, in the same words"},

	{"volumes", "journal", LevelRegion, "datalayer.Journal Id", "the page states which journal it belongs to, so the index needs no second fetch to say"},
	{"volumes", "volumes", LevelRegion, "[data-test=volumes-and-issues] .app-vol-and-issues-item", "472 KB with no JSON-LD at all, and the whole back catalogue for one request"},
	{"volumes", "volume.year", LevelRegion, "the volume heading's own time element", "read off the printed span, because a volume can run January to August and span no whole year"},
	{"volumes", "issue.date", LevelRegion, "time[datetime] inside the issue row", "a month, recorded as a month, rather than a day this tool would have invented"},

	// The subpages. These are the one place in the table where a subsidiary
	// page identifies its subject better than the container pages above it: a
	// /metrics, /figures/N or /tables/N page carries the parent article's
	// entire bibliographic head, all 66 meta names in Highwire, Dublin Core and
	// PRISM, exactly as the article page does. So the work being counted or
	// illustrated is named at rung 1 here while a journal's own home page
	// cannot manage rung 1 for its own title.
	//
	// Everything the subpage adds on top of that inherited head is rung 3 or
	// rung 4, and several rows are rung 4 for a reason worth stating: the
	// numbers on a metrics page are drawn as a picture. An accesses count and a
	// citation count are the same element with the same class, told apart only
	// by the heading of the section they sit under, and the cohort comparison
	// is one English sentence with no markup inside it at all.

	{"metrics", "doi", LevelHighwire, "citation_doi", "the subpage carries the parent article's whole head, so the work counted is named at rung 1"},
	{"metrics", "title", LevelHighwire, "citation_title", "stated in the head and again in the printed From line, and the head is the one that does not move"},
	{"metrics", "article_url", LevelSelector, ".c-article-metrics__title a[href]", "the address is printed once, in the link back to the article, and no vocabulary states it"},
	{"metrics", "updated", LevelSelector, ".c-article-metrics__updated", "a daily count with no date cannot be compared with anything, including a later reading of this page"},
	{"metrics", "accesses", LevelSelector, ".app-article-metrics-count under the Accesses heading", "the count carries no data-test of its own and is told from the citation count by the section heading"},
	{"metrics", "citations", LevelRegion, "[data-test=citation-count]", "the same class as the accesses count, and the only one of the two that names itself"},
	{"metrics", "citations_source", LevelSelector, "the section's own prose, provided by X", "Springer says 1,906 where Crossref says 1,553, so a count with nobody's name on it is not a fact"},
	{"metrics", "altmetric", LevelRegion, "[data-test=altmetric-score] img[alt] and the badge src", "stated twice on the same page, in an alt text and in a query parameter, and they agreed"},
	{"metrics", "altmetric_breakdown", LevelRegion, "[data-test=metrics-counts] li", "the kind comes from the class suffix, because the printed noun is tweeters here and Mendeley there"},
	{"metrics", "altmetric_cohorts", LevelSelector, "the donut caption's prose", "two comparisons in one sentence, all journals and this journal, and the sizes are what make them non comparable"},
	{"metrics", "mentions", LevelRegion, "[data-test=metrics-mentions] .c-card-metrics", "the named coverage only, five cards against a breakdown counting 1,334, so the two are separate fields"},

	{"figure", "article_title", LevelHighwire, "citation_title", "the parent article's head travels with its figure, so the work is named at rung 1"},
	{"figure", "label", LevelRegion, "[data-test=top-caption]", "Fig. 1 as the publisher prints it, and the number is taken from the url rather than parsed back out"},
	{"figure", "caption", LevelRegion, "[data-test=bottom-caption]", "printed below the image here and above it on the article page, which is why both are read by region"},
	{"figure", "image", LevelRegion, "[data-test=figure] img", "the full rendition at 1177 wide, where the article page carries the same asset at 685"},
	{"figure", "refs", LevelRegion, "[data-test=citation-ref]", "the link text is only the year and the whole reference sits in the title attribute, so both are kept"},

	{"table", "article_title", LevelHighwire, "citation_title", "the same inherited head a figure page carries, and the same rung 1"},
	{"table", "label", LevelSelector, ".c-article-satellite-title", "the tables page names one region in total, so its heading is read by class and split on the label"},
	{"table", "caption", LevelSelector, ".c-article-satellite-title", "label and caption are printed as one string here where a figure gives them two elements"},
	{"table", "rows", LevelSelector, ".c-article-table-container table", "the article page has zero table elements in 718 KB, so the body is published here and nowhere else"},
}

// FieldsNamed returns the table rows matching a name.
//
// It takes either the bare name, title, or the qualified one, journal.title.
// The bare form can match more than one row, and that is not an ambiguity to be
// resolved here: two records answering the same field from two different rungs
// is exactly the thing worth seeing side by side.
func FieldsNamed(name string) []Field {
	var out []Field
	for _, f := range Fields {
		if f.Name == name || f.Qualified() == name {
			out = append(out, f)
		}
	}
	return out
}

// FieldsFor returns every row belonging to one record.
func FieldsFor(record string) []Field {
	var out []Field
	for _, f := range Fields {
		if f.Record == record {
			out = append(out, f)
		}
	}
	return out
}

// Records are the record names in the table, in the order they appear.
func Records() []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range Fields {
		if !seen[f.Record] {
			seen[f.Record] = true
			out = append(out, f.Record)
		}
	}
	return out
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
	sort.Slice(out, func(i, j int) bool { return out[i].Qualified() < out[j].Qualified() })
	return out
}
