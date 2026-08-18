package spr

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The walk.
//
// Four rungs, tried in order, first one that answers wins, and the one that
// answered is written into the envelope. A rung that answers with an empty
// value counts as not answering, with exactly one exception:
// citation_fulltext_world_readable is present and empty on both article
// captures, so present-and-empty is itself the recorded fact.
//
// The order of the fields below follows the record rather than the ladder,
// because the question a reader has is almost always "where did this field come
// from" and not "what does rung 2 answer".

// ErrNotAWork is returned for a page that is not one of the four work types.
// It is a plain answer rather than a failure: a journal home page is a real
// page and this is simply not the extractor for it.
var ErrNotAWork = errors.New("this page is not an article, chapter, protocol or reference work entry")

// workPaths maps the four url prefixes to the type name the record carries.
var workPaths = map[string]string{
	"/article/":            "article",
	"/chapter/":            "chapter",
	"/protocol/":           "protocol",
	"/referenceworkentry/": "entry",

	// The site canonicalizes a reference work entry to /rwe/ while still
	// serving and linking /referenceworkentry/. Both are real addresses for the
	// same page, so both are recognized.
	"/rwe/": "entry",
}

// WorkType returns the content type a url addresses, or the empty string.
//
// A work's own subpages live under its path and are not the work. That has to
// be decided here, on the url, because it cannot be decided on the page: a
// /metrics, /figures/N or /tables/N page carries the parent article's entire
// bibliographic head, all 66 meta names, so extracting one as a work succeeds
// and quietly produces an article record with no body, no sections and no
// references. The address is the only thing that says which page this is.
func WorkType(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if subpageOf(u.Path) {
		return ""
	}
	for prefix, name := range workPaths {
		if strings.HasPrefix(u.Path, prefix) {
			return name
		}
	}
	return ""
}

// subpageOf reports whether a path addresses one of a work's subpages rather
// than the work itself.
func subpageOf(path string) bool {
	path = strings.TrimSuffix(path, "/")
	if strings.HasSuffix(path, "/metrics") {
		return true
	}
	return satelliteRe.MatchString(path)
}

// extractor holds the four rungs for one page plus the envelope being built.
type extractor struct {
	doc  *html.Node
	meta *Meta
	ld   *linkData
	reg  *regions
	w    *Work
}

// ExtractWork reads a work record out of a fetched work page.
//
// A restricted page is extracted rather than refused. The metadata on a
// paywalled chapter is complete and only the body is missing, so the record is
// built, body is named in Envelope.Missed with the page's own reason, and the
// caller gets everything that was actually there. Refusing would throw away
// good data because one field was withheld.
func ExtractWork(resp *Response) (*Work, error) {
	if resp == nil {
		return nil, errors.New("no response to extract from")
	}
	kind := WorkType(resp.URL)
	if kind == "" {
		kind = WorkType(resp.Final)
	}
	if kind == "" {
		return nil, ErrNotAWork
	}
	if resp.Status == StatusChallenged {
		return nil, errors.New("the edge served a client challenge, so there is no page to extract")
	}

	doc, err := parseDoc(resp.Body)
	if err != nil {
		return nil, err
	}

	x := &extractor{
		doc:  doc,
		meta: ParseMeta(doc),
		ld:   parseLinkData(doc),
		reg:  parseRegions(doc),
		w:    &Work{Type: kind, URL: resp.URL},
	}
	x.w.Envelope = Envelope{
		Tier:      "html",
		URLs:      []string{resp.URL},
		Fetched:   resp.Fetched,
		Status:    resp.Status,
		Redirects: resp.Redirects,
		Bytes:     len(resp.Body),
	}
	x.run()
	return x.w, nil
}

// rung is one candidate answer for a field: the level, the exact source, and
// what that source said.
type rung struct {
	level  Level
	source string
	value  string
}

func hw(name, value string) rung  { return rung{LevelHighwire, name, value} }
func ld(name, value string) rung  { return rung{LevelLinkedData, name, value} }
func rgn(name, value string) rung { return rung{LevelRegion, name, value} }

// pick walks the rungs in order, records the one that answered, and returns
// its value. Nothing is recorded when no rung answers, because a field the page
// never carried is absent rather than missed. A caller that expected the field
// says so itself with miss.
func (x *extractor) pick(field string, rungs ...rung) string {
	for _, r := range rungs {
		if strings.TrimSpace(r.value) != "" {
			x.w.Envelope.via(field, r.level, r.source)
			return strings.TrimSpace(r.value)
		}
	}
	return ""
}

func (x *extractor) run() {
	e := x.ld.work()

	x.identity(e)
	x.core(e)
	x.container(e)
	x.dates(e)
	x.people(e)
	x.access(e)
	x.rights(e)
	x.body()
	x.references()
	x.payloads()

	x.w.Envelope.Unread = x.reg.unread()
	x.w.Envelope.sortMissed()
}

func (x *extractor) identity(e *ldEntity) {
	w := x.w
	w.DOI = x.pick("doi", hw("citation_doi", x.meta.First("citation_doi")),
		hw("prism.doi", x.meta.First("prism.doi")),
		hw("DOI", x.meta.First("DOI")))
	if w.DOI == "" {
		w.Envelope.miss("doi", "no vocabulary on this page declared a doi, which no work page has done before")
	}
	w.ArticleType = x.pick("article_type", hw("citation_article_type", x.meta.First("citation_article_type")))

	w.PDFURL = x.pick("pdf_url", hw("citation_pdf_url", x.meta.First("citation_pdf_url")))
	w.FulltextURL = x.pick("fulltext_url", hw("citation_fulltext_html_url", x.meta.First("citation_fulltext_html_url")))
	w.AbstractURL = x.pick("abstract_url", hw("citation_abstract_html_url", x.meta.First("citation_abstract_html_url")))

	if api := x.pick("api_url", hw("citation_springer_api_url", x.meta.First("citation_springer_api_url"))); api != "" {
		w.APIURL = stripAPIKey(api)
	}
	if w.URL != "" {
		w.MetricsURL = strings.TrimSuffix(w.URL, "/") + "/metrics"
	}
	if e != nil && w.ArticleType == "" {
		w.ArticleType = x.pick("article_type", ld("genre", e.Genre.First()))
	}
}

func (x *extractor) core(e *ldEntity) {
	w := x.w
	w.Title = x.pick("title",
		hw("citation_title", x.meta.First("citation_title")),
		hw("dc.title", x.meta.First("dc.title")),
		ld("headline", e.title()),
		rgn("article-title", x.reg.text("article-title")))
	if w.Title == "" {
		w.Envelope.miss("title", "no vocabulary and no region on this page carried a title")
	}

	w.Abstract = x.pick("abstract",
		hw("dc.description", x.meta.First("dc.description")),
		ld("description", ldString(e, func(e *ldEntity) string { return e.Description })))
	if w.Abstract == "" {
		// Declared and empty is a different fact from never declared, and on a
		// reference work entry it is the one that happens: dc.description is
		// present with an empty value and so is the schema.org description.
		why := "neither dc.description nor the schema.org description was present"
		if x.meta.Empty("dc.description") {
			why = "dc.description was present and empty, and so was the schema.org description"
		}
		w.Envelope.miss("abstract", why)
	}

	w.Language = x.pick("language",
		hw("citation_language", x.meta.First("citation_language")),
		hw("dc.language", x.meta.First("dc.language")))

	// Keywords are the authors' own terms and only JSON-LD carries them. They
	// arrive as a list of 21 on an article and as one comma joined string on a
	// chapter, and Split handles both without inventing terms in the list case.
	if e != nil {
		if kw := e.Keywords.Split(); len(kw) > 0 {
			w.Keywords = kw
			w.Envelope.via("keywords", LevelLinkedData, "keywords")
		} else if len(e.Keywords.List()) > 0 {
			w.Envelope.miss("keywords", "the page declared keywords and the value was empty")
		}
	}

	if s := x.meta.All("dc.subject"); len(s) > 0 {
		w.Subjects = s
		w.Envelope.via("subjects", LevelHighwire, "dc.subject")
	}

	w.SectionLabel = x.pick("section_label", hw("prism.section", x.meta.First("prism.section")))
}

func (x *extractor) container(e *ldEntity) {
	w := x.w

	w.ContainerTitle = x.pick("container_title",
		hw("citation_journal_title", x.meta.First("citation_journal_title")),
		hw("citation_inbook_title", x.meta.First("citation_inbook_title")),
		hw("prism.publicationName", x.meta.First("prism.publicationName")),
		ld("isPartOf.name", ldPartName(e)))
	w.ContainerAbbrev = x.pick("container_abbrev", hw("citation_journal_abbrev", x.meta.First("citation_journal_abbrev")))

	// Both issns, deduplicated, in the order the higher rung supplied. Highwire
	// gave one on the measured article and JSON-LD gave two, and a caller
	// matching on either needs both.
	issn := x.meta.All("citation_issn")
	issn = append(issn, x.meta.All("prism.issn")...)
	src := "citation_issn"
	if e != nil && e.IsPartOf != nil {
		if more := e.IsPartOf.ISSN.List(); len(more) > 0 {
			issn = append(issn, more...)
			if len(x.meta.All("citation_issn")) == 0 {
				src = "isPartOf.issn"
			} else {
				src = "citation_issn + jsonld:isPartOf.issn"
			}
		}
	}
	if issn = dedupe(issn); len(issn) > 0 {
		w.ISSN = issn
		if strings.Contains(src, "jsonld") || src == "isPartOf.issn" {
			w.Envelope.via("issn", LevelHighwire, src)
		} else {
			w.Envelope.via("issn", LevelHighwire, src)
		}
	}

	w.ISBN = x.pick("isbn", hw("citation_isbn", x.meta.First("citation_isbn")), ld("isPartOf.isbn", ldPartISBN(e)))
	w.Volume = x.pick("volume",
		hw("citation_volume", x.meta.First("citation_volume")),
		hw("prism.volume", x.meta.First("prism.volume")))
	w.Issue = x.pick("issue",
		hw("citation_issue", x.meta.First("citation_issue")),
		hw("prism.number", x.meta.First("prism.number")))
	w.FirstPage = x.pick("first_page",
		hw("citation_firstpage", x.meta.First("citation_firstpage")),
		hw("prism.startingPage", x.meta.First("prism.startingPage")),
		ld("pageStart", ldString(e, func(e *ldEntity) string { return e.PageStart })))
	w.LastPage = x.pick("last_page",
		hw("citation_lastpage", x.meta.First("citation_lastpage")),
		hw("prism.endingPage", x.meta.First("prism.endingPage")),
		ld("pageEnd", ldString(e, func(e *ldEntity) string { return e.PageEnd })))

	// Pages is derived and only when both ends parse. A range of xiv to xxii is
	// a real range and it is not arithmetic, so it produces no count rather
	// than a wrong one.
	if first, err1 := strconv.Atoi(w.FirstPage); err1 == nil {
		if last, err2 := strconv.Atoi(w.LastPage); err2 == nil && last >= first {
			w.Pages = last - first + 1
			w.Envelope.via("pages", LevelHighwire, "citation_firstpage + citation_lastpage")
		}
	}

	w.Publisher = x.pick("publisher",
		hw("citation_publisher", x.meta.First("citation_publisher")),
		hw("dc.publisher", x.meta.First("dc.publisher")),
		ld("publisher.name", ldPublisher(e)))

	if title := x.meta.First("citation_series_title"); title != "" {
		w.Series = &Ref{Kind: "series", Name: title}
		w.Envelope.via("series", LevelHighwire, "citation_series_title")
	}
	if title := x.meta.First("citation_conference_title"); title != "" {
		w.Conference = &Ref{Kind: "conference", Name: title}
		w.Envelope.via("conference", LevelHighwire, "citation_conference_title")
	}
	if w.ContainerTitle != "" {
		w.Container = containerRef(w)
	}
}

// containerRef points at the journal or the book this work sits in. The id is
// the journal number where the page states one, because that is what a journal
// url is built from, and a Ref with no id is still a useful name.
func containerRef(w *Work) *Ref {
	kind := "book"
	if w.Type == "article" {
		kind = "journal"
	}
	return &Ref{Kind: kind, Name: w.ContainerTitle, ID: w.ISBN}
}

func (x *extractor) dates(e *ldEntity) {
	w := x.w
	w.Published = x.date("published",
		hw("citation_publication_date", x.meta.First("citation_publication_date")),
		hw("prism.publicationDate", x.meta.First("prism.publicationDate")),
		ld("datePublished", ldString(e, func(e *ldEntity) string { return e.DatePublished })))
	w.Online = x.date("online", hw("citation_online_date", x.meta.First("citation_online_date")))
	w.CoverDate = x.date("cover_date", hw("citation_cover_date", x.meta.First("citation_cover_date")))
	w.Modified = x.date("modified", ld("dateModified", ldString(e, func(e *ldEntity) string { return e.DateModified })))
}

// date picks a rung and parses it. A value that is present and does not parse
// is a miss with the string in the reason, not a silent absence, because the
// string is the evidence for whatever the fix turns out to be.
func (x *extractor) date(field string, rungs ...rung) *Date {
	raw := x.pick(field, rungs...)
	if raw == "" {
		return nil
	}
	d, err := ParseDate(raw)
	if err != nil {
		delete(x.w.Envelope.Via, field)
		x.w.Envelope.miss(field, "the page stated "+strconv.Quote(raw)+" and it is not a date in any form this site is known to print")
		return nil
	}
	return &d
}

func (x *extractor) people(e *ldEntity) {
	w := x.w

	// Authors come from rung 2 on purpose. Highwire emits citation_author,
	// citation_author_institution and citation_author_email as three parallel
	// arrays whose alignment is positional, and that alignment breaks the
	// moment one author has two affiliations. JSON-LD binds the three to the
	// right person. Where JSON-LD is absent the parallel arrays are used and
	// the weaker provenance is written into the envelope rather than left in a
	// comment nobody reads.
	if e != nil && len(e.Author) > 0 {
		w.Authors = people(e.Author)
		w.Envelope.via("authors", LevelLinkedData, "author[]")
	} else if names := x.meta.All("citation_author"); len(names) > 0 {
		w.Authors = parallelAuthors(names, x.meta.All("citation_author_institution"), x.meta.All("citation_author_email"))
		w.Envelope.via("authors", LevelHighwire, "citation_author (positional, no json-ld on this page)")
	} else {
		w.Envelope.miss("authors", "neither the schema.org block nor citation_author named anybody")
	}
	if e != nil && len(e.Editor) > 0 {
		w.Editors = people(e.Editor)
		w.Envelope.via("editors", LevelLinkedData, "editor[]")
	}
	if em := x.meta.All("citation_author_email"); len(em) > 0 {
		w.Corresponding = dedupe(em)
		w.Envelope.via("corresponding", LevelHighwire, "citation_author_email")
	}
}

func people(in []ldPerson) []Author {
	out := make([]Author, 0, len(in))
	for i, p := range in {
		a := Author{Name: p.Name, Email: p.Email, Position: i}
		if id, err := ParseORCID(p.URL); err == nil {
			a.ORCID = string(id)
		}
		for _, org := range p.Affiliation {
			af := Affiliation{Name: org.Name}
			if org.Address != nil {
				af.Address = org.Address.Name
			}
			a.Affiliations = append(a.Affiliations, af)
		}
		out = append(out, a)
	}
	return out
}

// parallelAuthors is the fallback, and it only lines the arrays up when they
// are the same length. Unequal lengths mean the positional assumption has
// already failed, and attaching the wrong institution to a person is worse than
// attaching none.
func parallelAuthors(names, institutions, emails []string) []Author {
	out := make([]Author, 0, len(names))
	aligned := len(institutions) == len(names)
	mails := len(emails) == len(names)
	for i, n := range names {
		a := Author{Name: n, Position: i}
		if aligned && institutions[i] != "" {
			a.Affiliations = []Affiliation{{Name: institutions[i]}}
		}
		if mails {
			a.Email = emails[i]
		}
		out = append(out, a)
	}
	return out
}

func (x *extractor) access(e *ldEntity) {
	w := x.w

	// Two independent declarations of the same fact, in two vocabularies. They
	// agreed on all nine captures, which is exactly why a disagreement is worth
	// recording rather than resolving.
	raw := x.meta.First("access")
	var free *bool
	switch {
	case raw != "":
		v := strings.EqualFold(raw, "yes")
		free = &v
		w.Access.Raw = raw
		w.Envelope.via("access", LevelHighwire, "access + jsonld:isAccessibleForFree")
	case e != nil && e.IsAccessibleForFree != nil:
		v := *e.IsAccessibleForFree
		free = &v
		w.Envelope.via("access", LevelLinkedData, "isAccessibleForFree")
	default:
		w.Envelope.miss("access", "the page declared neither meta access nor isAccessibleForFree")
	}
	if free != nil && e != nil && e.IsAccessibleForFree != nil && *free != *e.IsAccessibleForFree {
		w.Envelope.miss("access", "meta access says "+raw+" and isAccessibleForFree says "+
			strconv.FormatBool(*e.IsAccessibleForFree)+"; the page is stating two different things and this tool does not pick")
	}
	w.Access.Free = free

	// Present and empty is the recorded fact here, not a missing field. The tag
	// is present and empty on both article captures, including the open access
	// one, and is not shipped at all on a chapter, a protocol or an entry. A
	// plain string would make the day it disappears from an article look like
	// the day it was always going to be empty.
	if x.meta.Has("citation_fulltext_world_readable") {
		v := x.meta.First("citation_fulltext_world_readable")
		w.Access.WorldReadable = &v
		w.Envelope.via("access.world_readable", LevelHighwire, "citation_fulltext_world_readable (present, empty)")
	}

	if s := accessStatement(x.doc); s != "" {
		w.Access.Statement = s
		w.Envelope.via("access.statement", LevelSelector, ".c-notes__text")
	}
}

func (x *extractor) rights(e *ldEntity) {
	w := x.w
	w.License = x.pick("license", ld("license", ldString(e, func(e *ldEntity) string { return e.License })))
	w.Copyright = x.pick("copyright",
		hw("dc.copyright", x.meta.First("dc.copyright")),
		hw("prism.copyright", x.meta.First("prism.copyright")),
		rgn("copyright", x.reg.text("copyright")))
	w.Rights = x.pick("rights", hw("dc.rights", x.meta.First("dc.rights")))
	w.RightsAgent = x.pick("rights_agent",
		hw("dc.rightsAgent", x.meta.First("dc.rightsAgent")),
		hw("prism.rightsAgent", x.meta.First("prism.rightsAgent")))
}

func (x *extractor) body() {
	w := x.w

	// The section tree comes from data-title and document order, and from
	// nothing else, because there are no id attributes on Springer's article
	// sections. Position is the handle. No anchor is synthesized, because one
	// would look stable and would not be.
	nodes := x.reg.sectionNodes()
	for i, n := range nodes {
		w.Sections = append(w.Sections, Section{
			Number:   sectionNumber(n),
			Title:    attr(n, "data-title"),
			Depth:    depth(n, isSection),
			Position: i,

			// Own text and not all the text under the node, so that a parent
			// section does not repeat every word of the child sections nested
			// inside it. Concatenating the tree would make the body several
			// times its own length and would put the same sentence in four
			// records at four depths.
			Text: ownText(n),
		})
	}
	if len(w.Sections) > 0 {
		w.Envelope.via("sections", LevelRegion, "section[data-title]")

		// BodyText is the readable article in one string, which is what a
		// reader who wants to search or diff the prose actually wants. It is
		// the section texts joined rather than a second walk of the tree, so
		// there is exactly one answer to what the body of this page says.
		//
		// A restricted page is left without one on purpose. Its sections are
		// the author information and the rights notice, and joining those into
		// a field called body_text would hand somebody the back matter under
		// the name of the article they did not get.
		restricted := w.Access.Free != nil && !*w.Access.Free
		var parts []string
		for _, s := range w.Sections {
			if s.Text != "" {
				parts = append(parts, s.Text)
			}
		}
		if len(parts) > 0 && !restricted {
			w.BodyText = strings.Join(parts, "\n\n")
			w.Envelope.via("body_text", LevelRegion, "section[data-title]")
		}
	}

	// The recommender strip carries a data-title and is not a section of the
	// paper, so it comes out here and goes to rails.
	for _, n := range x.reg.railNodes() {
		for _, a := range findTag(n, atom.A) {
			href := attr(a, "href")
			if href == "" {
				continue
			}
			w.Rails = append(w.Rails, Ref{Kind: "work", Name: text(a), URL: trimQuery(href), ID: doiFromPath(href)})
		}
	}
	if len(w.Rails) > 0 {
		w.Envelope.via("rails", LevelRegion, "section[data-title="+railTitle+"]")
	}

	// The two caption elements on a figure are not a caption and a subtitle.
	// c-article-section__figure-caption holds "Fig. 1", which is the label, and
	// the prose lives in the description below the image under the same
	// bottom-caption region name the figure subpage uses. Reading the first as
	// the caption gave every figure a caption equal to its own label, which
	// looked like data and was not.
	for _, n := range x.reg.all("figure") {
		f := Figure{Label: attr(n, "data-title")}
		if c := firstClass(n, "c-article-section__figure-caption"); c != nil {
			if f.Label == "" {
				f.Label = text(c)
			}
			if id := attr(c, "id"); id != "" {
				f.Anchor = "#" + id
			}
		}
		f.Caption = text(x.reg.firstIn(n, "bottom-caption"))
		for _, a := range findTag(n, atom.A) {
			if href := attr(a, "href"); strings.Contains(href, "/figures/") {
				f.PageURL = absolute(href)
				break
			}
		}
		for _, img := range findTag(n, atom.Img) {
			if src := attr(img, "src"); src != "" {
				f.Image = absolute(src)
				break
			}
		}
		w.Figures = append(w.Figures, f)
	}
	if len(w.Figures) > 0 {
		w.Envelope.via("figures", LevelRegion, "[data-test=figure]")
	}

	// The tables the article announces. The caption element carries the article
	// anchor as its own id, which is the one place on this page where the two
	// are stated together, so it is read from there rather than from the link.
	for _, n := range x.reg.all("inline-table") {
		var t Table
		if c := x.reg.firstIn(n, "table-caption"); c != nil {
			t.Label, t.Caption = splitLabel(text(c))
			if id := attr(c, "id"); id != "" {
				t.Anchor = "#" + id
			}
		}
		if a := x.reg.firstIn(n, "table-link"); a != nil {
			t.PageURL = absolute(attr(a, "href"))
		}
		if t.Label == "" && t.Caption == "" && t.PageURL == "" {
			continue
		}
		w.Tables = append(w.Tables, t)
	}
	if len(w.Tables) > 0 {
		w.Envelope.via("tables", LevelRegion, "[data-test=inline-table]")
	}

	if n := equationCount(x.doc); n > 0 {
		w.Equations = n
		w.Envelope.via("equations", LevelSelector, ".c-article-equation")
	}
	if fn := footnotes(x.doc); len(fn) > 0 {
		w.Footnotes = fn
		w.Envelope.via("footnotes", LevelSelector, ".c-article-footnote--listed__item")
	}

	// The body itself. On a restricted work the metadata is complete and the
	// body is a preview, so the record is everything that was there and body is
	// named as missing with the page's own reason. This tool does not then go
	// looking at /content/pdf/, it does not look for a mirror, and there is no
	// flag that would.
	if w.Access.Free != nil && !*w.Access.Free {
		why := "the page states access=No and isAccessibleForFree=false; the body is a preview"
		if w.Access.Statement != "" {
			why = "the page says " + strconv.Quote(w.Access.Statement)
		}
		w.Envelope.miss("body", why)
	}
}

func (x *extractor) references() {
	w := x.w

	raw := x.meta.All("citation_reference")
	items := referenceItems(x.doc)

	// citation_reference is an article feature. It carried 122 entries on the
	// open access article and 69 on the subscription one, and zero on every
	// chapter, protocol and entry measured. Those types do print a reference
	// list, they just print it in the body and declare nothing in the head, so
	// where the head says nothing the printed list is read at rung 4 and the
	// weaker provenance goes in the envelope where somebody will see it.
	if len(raw) == 0 {
		if len(items) == 0 {
			if x.w.Type != "article" {
				w.Envelope.miss("references", "citation_reference is an article feature and this "+x.w.Type+" printed no reference list either")
			}
			return
		}
		links := 0
		for i, it := range items {
			r := Reference{Position: i, Text: it.text, Links: it.links}
			if len(it.links) > 0 {
				links++
			}
			w.References = append(w.References, r)
		}
		w.ReferencesCount = len(w.References)
		w.ReferenceLinks = links
		w.Envelope.via("references", LevelSelector, ".c-article-references__item (the head declared none)")
		if links > 0 {
			w.Envelope.via("ref_links", LevelSelector, ".c-article-references__links a")
		}
		return
	}

	w.Envelope.via("references", LevelHighwire, "citation_reference")

	// The printed list and the meta tags are two different sources and they are
	// paired only when their counts match, 122 against 122 on the open access
	// capture. A mismatch means one of the two moved, and pairing them anyway
	// would hang somebody else's resolver links off a reference, which is a
	// wrong answer that looks entirely right.
	paired := len(items) == len(raw)
	if !paired && len(items) > 0 {
		w.Envelope.miss("ref_links", "the printed list has "+strconv.Itoa(len(items))+
			" entries and citation_reference has "+strconv.Itoa(len(raw))+
			", so the two cannot be lined up by position and no links were attached")
	}

	links := 0
	for i, s := range raw {
		r := Reference{Position: i, Text: s}
		parseReferencePairs(&r)
		if paired {
			r.Links = items[i].links
			if len(r.Links) > 0 {
				links++
			}
		}
		w.References = append(w.References, r)
	}
	w.ReferencesCount = len(w.References)
	w.ReferenceLinks = links
	if links > 0 {
		w.Envelope.via("ref_links", LevelSelector, ".c-article-references__links a")
	}
}

// parseReferencePairs reads the semicolon separated Highwire pairs out of a
// reference and fills the structured fields.
//
// 68 of the 122 references on the open access capture are pairs and the other
// 54 are free text. Structured false is a normal outcome, not a failure, and
// the verbatim string is kept either way because the pairs are lossy and the
// string is what Springer published.
func parseReferencePairs(r *Reference) {
	if !strings.Contains(r.Text, "citation_title=") && !strings.Contains(r.Text, "citation_journal_title=") {
		return
	}
	for _, part := range strings.Split(r.Text, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "citation_title":
			r.Title = value
		case "citation_journal_title":
			r.Journal = value
		case "citation_author":
			// The pairs put every author in one value separated by commas,
			// which is as much structure as the source has.
			for _, a := range strings.Split(value, ",") {
				if s := strings.TrimSpace(a); s != "" {
					r.Authors = append(r.Authors, s)
				}
			}
		case "citation_publication_date":
			r.Year = value
		case "citation_volume":
			r.Volume = value
		case "citation_issue":
			r.Issue = value
		case "citation_firstpage":
			r.FirstPage = value
		case "citation_id":
			if r.DOI == "" && strings.HasPrefix(value, "10.") {
				r.DOI = value
			}
		}
	}
	r.Structured = r.Title != "" || r.Journal != ""
}

// payloads carries what was found and deliberately not read.
//
// The article's analytics blob is javascript rather than JSON. The tempting
// move is a lenient parser that recovers most of it most of the time, and a
// parser that half works on a blob the site can change without notice is a
// permanent source of quiet wrongness. The bytes go to the envelope verbatim,
// nothing reads them, and anyone who wants to write that parser can do it
// knowingly.
func (x *extractor) payloads() {
	if n := x.reg.first("springer-link-article-datalayer"); n != nil {
		// scriptText and not text, because the blob is the script source and
		// text drops script elements on the way past. Reading it with text
		// returns an empty string, which would look exactly like the day the
		// site stops shipping the block.
		if raw := scriptText(n); raw != "" {
			x.w.Envelope.extra("datalayer", []byte(raw))
		}
	}
	if x.ld.broken > 0 {
		x.w.Envelope.miss("linked_data", strconv.Itoa(x.ld.broken)+" schema.org blocks on this page did not parse as json")
	}
}

// ldString reads a field off the work entity, tolerating its absence.
func ldString(e *ldEntity, get func(*ldEntity) string) string {
	if e == nil {
		return ""
	}
	return get(e)
}

func ldPartName(e *ldEntity) string {
	if e == nil || e.IsPartOf == nil {
		return ""
	}
	return e.IsPartOf.Name
}

func ldPartISBN(e *ldEntity) string {
	if e == nil || e.IsPartOf == nil {
		return ""
	}
	return e.IsPartOf.ISBN.First()
}

func ldPublisher(e *ldEntity) string {
	if e == nil || e.Publisher == nil {
		return ""
	}
	return e.Publisher.Name
}

// dedupe keeps the first occurrence of each value and the order they arrived in.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// absolute resolves a page relative or protocol relative url against the site.
// Springer writes its image sources as //media.springernature.com/..., which is
// legal and is unusable the moment it leaves a browser.
func absolute(href string) string {
	switch {
	case strings.HasPrefix(href, "//"):
		return "https:" + href
	case strings.HasPrefix(href, "/"):
		return Base + href
	}
	return href
}

// trimQuery drops the query string off a link this tool stores.
//
// The recommender links carry ?fromPaywallRec=false, which is Springer telling
// itself where the click came from. It is not part of the address and keeping
// it would make the same work compare unequal to itself depending on which page
// it was found on.
func trimQuery(href string) string {
	if i := strings.IndexAny(href, "?#"); i >= 0 {
		href = href[:i]
	}
	return absolute(href)
}

// doiFromPath pulls a doi out of a link, which is how a recommender entry gets
// an identifier without a fetch.
func doiFromPath(href string) string {
	i := strings.Index(href, "10.")
	if i < 0 {
		return ""
	}
	d, err := ParseDOI(trimQuery(href[i:]))
	if err != nil {
		return ""
	}
	return string(d)
}
