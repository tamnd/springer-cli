package spr

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The /metrics subpage.
//
// 19 named regions against an article's 53, and the four that matter are
// link-metrics, citation-count, altmetric-score and metrics-mentions. Two of
// the numbers on it are not in any region at all. The accesses count sits in a
// bare .app-article-metrics-count paragraph, told apart from the citation count
// only by the heading above it, and the percentile sentence is prose with no
// class of its own. Both are rung 4 and the table says so.
//
// This page exists on articles. A chapter's /metrics answers 404 with 122 KB of
// error page, which is the same honest 404 the classifier already knows, so
// nothing special is needed for it beyond saying so in the error.

// ErrNotMetrics is returned for a page that is not a work's metrics subpage.
var ErrNotMetrics = errors.New("this page is not a metrics subpage")

// MetricsPath reports whether a url addresses a /metrics subpage.
func MetricsPath(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.TrimSuffix(u.Path, "/"), "/metrics")
}

// MetricsURL returns the metrics subpage for a work url or path.
func MetricsURL(work string) string {
	return strings.TrimSuffix(trimQuery(work), "/") + "/metrics"
}

// ExtractMetrics reads a metrics record out of a fetched /metrics page.
func ExtractMetrics(resp *Response) (*Metrics, error) {
	if resp == nil {
		return nil, errors.New("no response to extract from")
	}
	if !MetricsPath(resp.URL) && !MetricsPath(resp.Final) {
		return nil, ErrNotMetrics
	}
	p, err := newPage(resp)
	if err != nil {
		return nil, err
	}
	if p.reg.first("article-metrics-wrapper") == nil {
		return nil, ErrNotMetrics
	}

	m := &Metrics{URL: resp.URL}

	// The subpage carries the parent article's whole bibliographic head, all 66
	// meta names, so the work being counted is identified at rung 1 rather than
	// off the heading link that also states it.
	m.DOI = p.set("doi", LevelHighwire, "citation_doi", p.meta.First("citation_doi", "prism.doi"))
	if m.DOI == "" {
		m.DOI = p.set("doi", LevelSelector, "the url path", doiFromPath(resp.URL))
	}
	m.Title = p.set("title", LevelHighwire, "citation_title", p.meta.First("citation_title", "dc.title"))

	if a := firstTag(p.reg.first("article-metrics-wrapper"), atom.A); a != nil {
		m.ArticleURL = absolute(trimQuery(attr(a, "href")))
		p.env.via("article_url", LevelSelector, ".c-article-metrics__title a[href]")
		if m.Title == "" {
			m.Title = p.set("title", LevelSelector, ".c-article-metrics__title a", text(a))
		}
	}

	m.Updated = metricsUpdated(p)
	m.Accesses = accessesOf(p)
	m.Citations = citationsOf(p)
	m.Altmetric = altmetricOf(p)
	m.Mentions = mentionsOf(p)

	m.Envelope = p.finish()
	return m, nil
}

// metricsUpdated reads the stamp the page prints on itself.
//
// It is the one field on this page that is named in missed when it is absent,
// rather than simply left out. Everything else here is a count that a work can
// legitimately have none of, but a page of daily counts with no date on it is
// not a weaker record, it is numbers nobody can compare with anything.
func metricsUpdated(p *page) *time.Time {
	raw := ""
	for _, n := range findClass(p.doc, "c-article-metrics__updated") {
		raw = strings.TrimSpace(afterColon(text(n)))
		break
	}
	if raw == "" {
		p.env.miss("updated", "the page printed no last updated stamp, and a daily count with no date is not comparable with a later reading of the same page")
		return nil
	}
	// "Tue, 18 Aug 2026 10:34:56 UTC" on one capture and "Tue, 18 Aug 2026
	// 8:11:20 UTC" on the other, a single digit hour on the second. RFC1123
	// reads both, since Go's parser does not require the zero padding its own
	// reference layout shows.
	t, err := time.Parse(time.RFC1123, raw)
	if err != nil {
		p.env.miss("updated", "the page printed "+strconv.Quote(raw)+" as its last updated stamp and it did not parse as a date")
		return nil
	}
	p.env.via("updated", LevelSelector, ".c-article-metrics__updated")
	return &t
}

// accessesOf reads the view and download count.
//
// The count paragraph carries no data-test of its own, and the citation count
// beside it carries the same class, so the two are told apart by the heading of
// the section they sit in. Matching on the printed English heading is rung 4
// and it is the weakest match on this page, which is why the section walk looks
// for the heading rather than taking the first paragraph and hoping the order
// holds.
func accessesOf(p *page) *Accesses {
	box := p.reg.first("link-metrics")
	if box == nil {
		return nil
	}
	sec := metricsSection(box, "Accesses")
	if sec == nil {
		return nil
	}
	count := firstClass(sec, "app-article-metrics-count")
	if count == nil {
		return nil
	}
	raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text(count)), "Accesses"))
	if raw == "" {
		return nil
	}
	a := &Accesses{Raw: raw}
	if v, ok := parseScaled(raw); ok {
		a.Value = int(v)
	}
	// The page states its own caveat in prose. Reading it rather than assuming
	// it means a page that stops disclaiming stops being quoted as disclaiming.
	a.Approximate = strings.Contains(strings.ToLower(text(sec)), "approximate count")
	p.env.via("accesses", LevelSelector, ".app-article-metrics-container with an Accesses heading")
	return a
}

// citationsOf reads the count and, from the page's own sentence beside it, who
// counted.
func citationsOf(p *page) *Citations {
	n := p.reg.first("citation-count")
	if n == nil {
		return nil
	}
	count := firstInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text(n)), "Citations")))
	if count == 0 {
		return nil
	}
	c := &Citations{Count: count}
	p.env.via("citations", LevelRegion, "[data-test=citation-count]")

	// "Citation counts are provided by Dimensions and depend on their data
	// availability." The provider is never hardcoded, because a hardcoded name
	// stored beside a number is a lie that looks like it was checked.
	if box := p.reg.first("link-metrics"); box != nil {
		if sec := metricsSection(box, "Citations"); sec != nil {
			if src := providedBy(text(sec)); src != "" {
				c.Source = src
				p.env.via("citations_source", LevelSelector, "the Citations section prose, provided by ...")
			}
		}
	}
	if c.Source == "" {
		p.env.miss("citations_source", "the page stated a citation count without naming who produced it, and an unattributed count is not one this tool will pass on as a fact")
	}
	return c
}

// providedBy pulls the attribution out of "... are provided by Dimensions and
// depend on ...". It stops at the first word that ends the clause so that a
// provider with a two word name still comes through whole.
var providedByRe = regexp.MustCompile(`(?i)provided by\s+(.+?)\s*(?:\band\b|,|\.|$)`)

func providedBy(s string) string {
	m := providedByRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// metricsSection finds the counter box whose heading is this word.
func metricsSection(root *html.Node, heading string) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil || !hasClass(n, "app-article-metrics-container") {
			return found == nil
		}
		if h := firstTag(n, atom.H2); h != nil && strings.EqualFold(strings.TrimSpace(text(h)), heading) {
			found = n
			return false
		}
		return true
	})
	return found
}

// altmetricOf reads the score, the donut legend and the two cohorts.
func altmetricOf(p *page) *Altmetric {
	box := p.reg.first("altmetric-score")
	if box == nil {
		return nil
	}
	a := &Altmetric{}

	// The score is stated twice, in the badge image's alt text and again in its
	// src query string. The alt text is taken, because it is the one a screen
	// reader gets and therefore the one Springer has a reason to keep correct.
	if img := firstTag(box, atom.Img); img != nil {
		a.Score = firstInt(attr(img, "alt"))
		a.BadgeURL = absolute(attr(img, "src"))
	}
	for _, link := range findTag(box, atom.A) {
		href := attr(link, "href")
		if !strings.Contains(href, "altmetric.com/details/") {
			continue
		}
		a.DetailsURL = href
		a.ID = href[strings.LastIndex(href, "/")+1:]
		break
	}
	if a.Score == 0 && a.ID == "" {
		return nil
	}
	p.env.via("altmetric", LevelRegion, "[data-test=altmetric-score]")

	a.Breakdown = attentionKeys(p)
	a.Cohorts = cohortsOf(text(box))
	if len(a.Cohorts) > 0 {
		p.env.via("altmetric_cohorts", LevelSelector, "the Altmetric context sentence")
	}
	return a
}

// attentionKeys reads the donut legend.
//
// The kind comes off the legend swatch's own class, --twitter, --blogs, --news,
// --reddit, --mendeley, and the printed text supplies the count and the English
// noun. Reading the kind off the noun instead would mean deciding that
// "tweeters" and "Mendeley" belong to the same vocabulary, and they plainly do
// not.
func attentionKeys(p *page) []AttentionKey {
	var out []AttentionKey
	for _, ul := range p.reg.all("metrics-counts") {
		for _, li := range findTag(ul, atom.Li) {
			k := AttentionKey{}
			for _, span := range findClass(li, "c-article-metrics__altmetric-key") {
				for _, f := range strings.Fields(attr(span, "class")) {
					if _, kind, ok := strings.Cut(f, "c-article-metrics__altmetric-key--"); ok {
						k.Kind = kind
					}
				}
				break
			}
			label := strings.TrimSpace(text(li))
			if label == "" {
				continue
			}
			k.Count = firstInt(label)
			k.Label = strings.TrimSpace(strings.TrimPrefix(label, strconv.Itoa(k.Count)))
			out = append(out, k)
		}
	}
	if len(out) > 0 {
		p.env.via("altmetric_breakdown", LevelRegion, "[data-test=metrics-counts] li")
	}
	return out
}

// cohortRe reads one comparison out of the context sentence.
//
// The sentence is "This article is in the 95th percentile (ranked 22,032nd) of
// the 474,090 tracked articles of a similar age in all journals and the 96th
// percentile (ranked 1st) of the 29 tracked articles of a similar age in
// Machine Learning." It has no class, no data-test and no structure. It is the
// most fragile read in this package and it earns its place because the two
// cohort sizes are the only thing that makes the two percentiles mean anything.
var cohortRe = regexp.MustCompile(`(?i)(\d+)\s*(?:st|nd|rd|th)?\s*percentile\s*\(\s*ranked\s+([\d,]+)\s*(?:st|nd|rd|th)?\s*\)\s*of\s+the\s+([\d,]+)\s+tracked articles of a similar age in\s+(.+?)\s*(?:\band the\b|\.\s*View more|\.$|$)`)

func cohortsOf(s string) []Cohort {
	var out []Cohort
	for _, m := range cohortRe.FindAllStringSubmatch(collapse(s), -1) {
		c := Cohort{
			Percentile: parseCount(m[1]),
			Rank:       parseCount(m[2]),
			Size:       parseCount(m[3]),
			Scope:      strings.TrimRight(strings.TrimSpace(m[4]), "."),
		}
		if c.Size == 0 {
			continue
		}
		out = append(out, c)
	}
	return out
}

// mentionsOf reads the named coverage.
//
// These are the news outlets and blogs only. The breakdown counts tweeters,
// Redditors and Mendeley readers too and names none of them, so a consumer that
// treats len(Mentions) as the attention total will be wrong by three orders of
// magnitude on the measured capture, which is why the field is called mentions
// and not attention.
func mentionsOf(p *page) []Mention {
	box := p.reg.first("metrics-mentions")
	if box == nil {
		return nil
	}
	var out []Mention
	for _, card := range findClass(box, "c-card-metrics") {
		m := Mention{}
		if a := firstTag(firstClass(card, "c-card-metrics__heading"), atom.A); a != nil {
			m.Title = strings.TrimSpace(text(a))
			m.URL = attr(a, "href")
		}
		m.Outlet = strings.TrimSpace(text(firstClass(card, "c-card-metrics__authors")))
		if m.Title == "" {
			continue
		}
		out = append(out, m)
	}
	if len(out) > 0 {
		p.env.via("mentions", LevelRegion, "[data-test=metrics-mentions] .c-card-metrics")
	}
	return out
}

// parseCount reads a printed integer with thousands separators in it. firstInt
// stops at the comma and would read 22,032 as 22, which is the kind of wrong
// that looks right.
func parseCount(s string) int {
	n, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
	if err != nil {
		return 0
	}
	return n
}

// collapse squeezes runs of whitespace, so that a sentence broken across eight
// indented source lines matches as one.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
