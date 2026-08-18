package spr

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The journal page.
//
// 8 meta names in the head, none of them bibliographic, and one JSON-LD block
// that describes the web page rather than the journal. So everything here comes
// from a named region or from the analytics payload, and the payload wins where
// both have an answer, because it is the publisher's own machine readable
// statement and a region is a rendered label that has to be read back out of
// its own prose.

// ErrNotAJournal is returned for a page that is not a journal home page.
var ErrNotAJournal = errors.New("this page is not a journal home page")

// JournalPath reports whether a url addresses a journal home page.
//
// The volumes and issues page lives under the same prefix and is a different
// record, so a path with anything after the journal id is not this page.
func JournalPath(raw string) bool {
	id, sub := journalParts(raw)
	return id != "" && sub == ""
}

// journalParts splits a journal url into the journal id and whatever follows.
func journalParts(raw string) (id, sub string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	rest, ok := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/journal/")
	if !ok {
		return "", ""
	}
	id, sub, _ = strings.Cut(rest, "/")
	if _, err := ParseSpringerID(id); err != nil {
		return "", ""
	}
	return id, sub
}

// ExtractJournal reads a journal record out of a fetched journal home page.
func ExtractJournal(resp *Response) (*Journal, error) {
	if resp == nil {
		return nil, errors.New("no response to extract from")
	}
	if !JournalPath(resp.URL) && !JournalPath(resp.Final) {
		return nil, ErrNotAJournal
	}
	p, err := newPage(resp)
	if err != nil {
		return nil, err
	}

	j := &Journal{URL: resp.URL}

	j.SpringerID = p.layer("springer_id", "Journal Id")
	if j.SpringerID == "" {
		if id, _ := journalParts(resp.URL); id != "" {
			j.SpringerID = p.set("springer_id", LevelSelector, "the url path", id)
		}
	}
	j.Title = p.layer("title", "Journal Title")
	if j.Title == "" {
		j.Title = p.region("title", "journal-heading-href")
	}
	if j.Title == "" {
		p.env.miss("title", "neither the analytics payload nor the masthead named the journal")
	}

	// The two issns come out of their own regions, and out of the dd rather
	// than the region's whole text, which reads "Electronic ISSN 1573-0565"
	// with the label glued to the number.
	j.ElectronicISSN = issnOf(p, "electronic_issn", "springer-electronic-issn")
	j.PrintISSN = issnOf(p, "print_issn", "springer-print-issn")

	j.PublisherBrand = p.layer("publisher_brand", "publisherBrand")
	j.Imprint = p.layer("imprint", "imprint")

	// Publishing model is the reason the analytics payload is parsed at all. No
	// meta tag, no schema.org key and no region states it in machine readable
	// form. The masthead does print "Publishing model: Hybrid", which is the
	// same fact in a shorter word, and it is the fallback rather than the
	// source.
	j.PublishingModel = p.layer("publishing_model", "Publishing Model")
	if j.PublishingModel == "" {
		j.PublishingModel = p.set("publishing_model", LevelRegion,
			"[data-test=darwin-publishing-model]", afterColon(p.reg.text("darwin-publishing-model")))
	}
	if j.PublishingModel == "" {
		p.env.miss("publishing_model", "the analytics payload did not state a publishing model and the masthead did not print one")
	}

	if b := p.dl.boolean("continuousArticlePublishing"); b != nil {
		j.ContinuousPublication = b
		p.env.via("continuous_publication", LevelRegion, "datalayer.continuousArticlePublishing")
	}

	// Subjects are Springer's own classification of the journal and only the
	// payload carries them. The page prints no subject list anywhere.
	if s := p.dl.list("content", "category", "snt"); len(s) > 0 {
		j.Subjects = s
		p.env.via("subjects", LevelRegion, "datalayer.content.category.snt")
	}

	j.Editors = editorsOf(p, "journal-editor-links")
	j.Metrics = journalMetrics(p)
	j.IndexedIn = indexedIn(p)

	if n := firstInt(p.reg.text("total-oa-articles")); n > 0 {
		j.OpenAccessArticles = n
		p.env.via("open_access_articles", LevelRegion, "[data-test=total-oa-articles]")
	}

	j.Copyright = p.region("copyright", "copyright-information")
	j.About = p.region("about", "darwin-journal-homepage-promo-text")

	// The volumes are a request this command did not make. The Conn says where
	// they are and that none are held, which is a different statement from an
	// empty list.
	if j.SpringerID != "" {
		j.Volumes = &Conn{URL: Base + "/journal/" + j.SpringerID + "/volumes-and-issues"}
	}

	j.Envelope = p.finish()
	return j, nil
}

// issnOf reads one issn out of its region and validates the check digit.
//
// A number that fails the checksum is kept as printed and named in the
// envelope, because the page is what it is and quietly dropping a malformed
// issn would leave a journal looking as though it had none.
func issnOf(p *page, field, name string) string {
	n := p.reg.first(name)
	if n == nil {
		return ""
	}
	raw := strings.TrimSpace(detail(n))
	if raw == "" {
		return ""
	}
	issn, err := ParseISSN(raw)
	if err != nil {
		p.env.via(field, LevelRegion, "[data-test="+name+"] dd")
		p.env.miss(field, "the page printed "+strconv.Quote(raw)+" and it is not a valid issn: "+err.Error())
		return raw
	}
	p.env.via(field, LevelRegion, "[data-test="+name+"] dd")
	return string(issn)
}

// editorsOf reads an editor list, keeping the role the page printed over it.
//
// The markup is one <dl> per role: <dt>Editor-in-Chief</dt> and a <dd> holding
// a list of names. A journal with three boards ships three of these regions
// under the same name, so all of them are read and the role travels with each
// name rather than being thrown away in the flattening.
func editorsOf(p *page, name string) []Author {
	var out []Author
	for _, n := range p.reg.all(name) {
		role := strings.TrimSuffix(strings.TrimSpace(term(n)), ":")
		names := listItems(n)
		if len(names) == 0 {
			if d := strings.TrimSpace(detail(n)); d != "" {
				names = []string{d}
			}
		}
		for _, who := range names {
			out = append(out, Author{Name: who, Role: role, Position: len(out)})
		}
	}
	if len(out) > 0 {
		p.env.via("editors", LevelRegion, "[data-test="+name+"]")
	}
	return out
}

// journalMetricNames pairs each metric's label region with its value region.
// Springer ships them as separate elements, so the pairing is by name and not
// by position in the document.
var journalMetricNames = []struct{ label, value string }{
	{"impact-factor-label", "impact-factor-value"},
	{"five-year-impact-factor-label", "five-year-impact-factor-value"},
	{"metrics-speed-label", "metrics-speed-value"},
	{"metrics-downloads-label", "metrics-downloads-value"},
}

// journalMetrics reads the statistics block.
//
// A metric with no year is not emitted. An impact factor of 4.9 with no year
// attached is not comparable with anything, including itself a year later, so a
// metric whose year cannot be read is named in the envelope with its printed
// text instead. On the measured journal that is exactly one of the four:
// "Submission to first decision (median), 5 days", which the page prints
// without a year and which is a duration rather than an annual figure.
func journalMetrics(p *page) []Metric {
	var out []Metric
	found := false
	for _, pair := range journalMetricNames {
		label := strings.TrimSpace(p.reg.text(pair.label))
		value := strings.TrimSpace(p.reg.text(pair.value))
		if label == "" || value == "" {
			continue
		}
		found = true
		m, ok := parseMetric(label, value)
		if !ok {
			p.env.miss("metrics", "the page printed "+strconv.Quote(label+", "+value)+
				" with no year, and a metric with no year is not comparable with anything")
			continue
		}
		out = append(out, m)
	}
	if found && len(out) > 0 {
		p.env.via("metrics", LevelRegion, "[data-test=impact-factor-value] and its three siblings")
	}
	return out
}

// parseMetric reads "4.9 (2025)" and "2.4M (2025)" into a metric.
//
// Raw keeps what the page printed, because 2.4M is the publisher's number and
// 2400000 is this tool's reading of it, and the two should never be confused
// for one another in a table somebody is about to cite.
func parseMetric(label, value string) (Metric, bool) {
	open := strings.LastIndex(value, "(")
	closing := strings.LastIndex(value, ")")
	if open < 0 || closing < open {
		return Metric{}, false
	}
	year, err := strconv.Atoi(strings.TrimSpace(value[open+1 : closing]))
	if err != nil || year < 1800 || year > 2200 {
		return Metric{}, false
	}
	raw := strings.TrimSpace(value[:open])
	m := Metric{Name: label, Raw: raw, Year: year}
	if v, ok := parseScaled(raw); ok {
		m.Value = v
	}
	return m, true
}

// scales are the suffixes Springer prints on a large count.
var scales = map[byte]float64{'k': 1e3, 'K': 1e3, 'M': 1e6, 'B': 1e9}

// parseScaled reads 4.9 and 2.4M. A string that is not a number in either form
// leaves Value absent rather than zero, since a metric of zero is a claim and
// an unreadable metric is not.
func parseScaled(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	mult := 1.0
	if m, ok := scales[s[len(s)-1]]; ok {
		mult = m
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v * mult, true
}

// indexedIn reads the services that abstract and index the journal.
//
// It sits inside the journal information block as one more description list
// item, with no data-test of its own, so it is matched on its printed label.
// That is rung 4 work and the envelope says so.
func indexedIn(p *page) []string {
	n := p.reg.first("about-this-journal")
	if n == nil {
		return nil
	}
	for _, item := range findClass(n, "c-list-description__item") {
		if !strings.EqualFold(term(item), "Abstracted and indexed in") {
			continue
		}
		if names := listItems(item); len(names) > 0 {
			p.env.via("indexed_in", LevelSelector, "[data-test=about-this-journal] .c-list-description__item")
			return names
		}
	}
	return nil
}

// linkHref returns the href of the first anchor under a node, resolved against
// the site.
func linkHref(n *html.Node) string {
	if n == nil {
		return ""
	}
	if a := attr(n, "href"); a != "" {
		return trimQuery(a)
	}
	for _, a := range findTag(n, atom.A) {
		if href := attr(a, "href"); href != "" {
			return trimQuery(href)
		}
	}
	return ""
}
