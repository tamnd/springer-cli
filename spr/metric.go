package spr

import (
	"time"
)

// The three work subpages and the records they produce.
//
// A work page is a document. These three are not: /metrics is a dashboard whose
// numbers change daily, /figures/N is one asset at full resolution, and
// /tables/N is the only place the table body is published at all. Each is its
// own record because none of them is a smaller version of the others, and none
// belongs inside Work: a metrics count read on Tuesday and a title read on
// Tuesday are facts with very different shelf lives, and folding them into one
// record invites a consumer to cache them together.

// Metrics is the /metrics subpage: how often a work was read, how often it was
// cited, and how much attention it drew outside the literature.
//
// Every count here is a claim by a named body and none of them is a fact about
// the work. That is why Citations carries its Source and why there is still no
// bare Citations int on the Work record: this page says 1,906 and attributes it
// to Dimensions in its own prose, Crossref says 1,553 for the same DOI and
// OpenAlex says 1,563, and the three are counting different corpora under
// different inclusion rules. A consumer that wants one number has to choose
// which body it believes, and this tool will not choose for it.
type Metrics struct {
	// DOI and ArticleURL identify what the numbers are about. Title is carried
	// because a metrics record printed on its own is otherwise a page of digits
	// with nothing to say what they count.
	DOI        string `json:"doi,omitempty"`
	Title      string `json:"title,omitempty"`
	URL        string `json:"url,omitempty"`
	ArticleURL string `json:"article_url,omitempty"`

	// Updated is the stamp the page prints on itself, and it is required. Every
	// other field here is a number that changes daily, so a metrics record with
	// no timestamp is not a weaker record, it is a set of numbers that cannot
	// be compared with anything, including a later reading of the same page.
	Updated *time.Time `json:"updated,omitempty"`

	Accesses  *Accesses  `json:"accesses,omitempty"`
	Citations *Citations `json:"citations,omitempty"`
	Altmetric *Altmetric `json:"altmetric,omitempty"`

	// Mentions is the news and blog coverage the page lists by name. It is not
	// the whole of the attention: on the measured capture the breakdown counts
	// 20 tweeters, 1,307 Mendeley readers and 2 Redditors, and lists none of
	// them. Only the 2 news outlets and 3 blogs are named, and those 5 are what
	// appears here.
	Mentions []Mention `json:"mentions,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Accesses is the view and download count, which the page prints abbreviated.
//
// Raw survives for the same reason it does on a journal metric. 134k is what
// Springer published and 134000 is this tool's reading of it, and the page says
// in its own words that the number is "an approximate count of unique views and
// downloads", so treating 134000 as exact would be inventing three digits of
// precision the publisher explicitly disclaimed.
type Accesses struct {
	Raw   string `json:"raw,omitempty"`
	Value int    `json:"value,omitempty"`

	// Approximate is what the page states about its own number, and it was true
	// on every capture measured. It is read rather than assumed so that a page
	// which one day drops the caveat stops asserting it here too.
	Approximate bool `json:"approximate,omitempty"`
}

// Citations is a count and the body that produced it, never one without the
// other.
//
// Source is read out of the page's own sentence, "Citation counts are provided
// by Dimensions and depend on their data availability", rather than hardcoded.
// Springer could switch providers tomorrow and a hardcoded string would then be
// a lie stored next to a number, which is worse than having no attribution at
// all because it looks like it was checked.
type Citations struct {
	Count  int    `json:"count"`
	Source string `json:"source,omitempty"`
}

// Altmetric is the attention score and everything the page says around it.
type Altmetric struct {
	Score int `json:"score"`

	// ID is Altmetric's own identifier for this work, taken from the details
	// link. It is the only stable handle on the page, since the score itself
	// moves and the badge image url encodes the score into its query string.
	ID         string `json:"id,omitempty"`
	DetailsURL string `json:"details_url,omitempty"`
	BadgeURL   string `json:"badge_url,omitempty"`

	// Breakdown is the donut legend: which kinds of attention, and how much of
	// each.
	Breakdown []AttentionKey `json:"breakdown,omitempty"`

	// Cohorts is what the page compares this work against, and there are two of
	// them rather than one. The measured article is in the 95th percentile of
	// 474,090 tracked articles of a similar age in all journals, and the 96th
	// percentile of the 29 of a similar age in Machine Learning. A record that
	// kept one percentile would have to pick, and the two answer different
	// questions.
	Cohorts []Cohort `json:"cohorts,omitempty"`
}

// AttentionKey is one wedge of the donut.
//
// Kind is the machine name off the legend's own class, twitter, blogs, news,
// reddit or mendeley, and Label is the printed English beside it. The two are
// kept apart because "20 tweeters" and "1307 Mendeley" are not the same
// grammar, and matching on the printed noun would break on the first
// pluralization change or translation.
type AttentionKey struct {
	Kind  string `json:"kind,omitempty"`
	Count int    `json:"count"`
	Label string `json:"label,omitempty"`
}

// Cohort is one comparison the Altmetric block draws.
//
// Scope is what the set is, "all journals" or the journal's own name. Size is
// how many works are in it, which is what makes a percentile mean anything: the
// 96th percentile of 29 tracked articles is one article out of 29, and printing
// it beside the 95th percentile of 474,090 without the sizes would suggest the
// two are comparable.
type Cohort struct {
	Scope      string `json:"scope,omitempty"`
	Percentile int    `json:"percentile,omitempty"`
	Rank       int    `json:"rank,omitempty"`
	Size       int    `json:"size,omitempty"`
}

// Mention is one named piece of coverage.
type Mention struct {
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
	Outlet string `json:"outlet,omitempty"`
}

// FigurePage is the /figures/N subpage: one figure at full resolution.
//
// The article page already carries every figure's label, caption and a link,
// so this record exists for what the article page does not have. The inline
// image is served at lw685, 685 pixels wide, and this page serves the same
// asset under full at 1,177. The full url can be guessed by swapping one path
// segment, and guessing a CDN's url scheme is a thing that works until it does
// not. This page states it, along with the true pixel dimensions and the
// reference links inside the caption, which the inline copy does not resolve.
type FigurePage struct {
	// Label is what the page prints as its heading, "Fig. 1". Number is the
	// same figure as an integer, which is what the url is addressed by.
	Label  string `json:"label,omitempty"`
	Number int    `json:"number,omitempty"`

	Caption string `json:"caption,omitempty"`
	URL     string `json:"url,omitempty"`

	// Anchor is the fragment on the article page this figure sits at, #Fig1,
	// which is how a reader gets from here back to the sentence that cited it.
	Anchor       string `json:"anchor,omitempty"`
	ArticleURL   string `json:"article_url,omitempty"`
	ArticleTitle string `json:"article_title,omitempty"`

	Image *Image `json:"image,omitempty"`

	// Refs is the works the caption cites. On the measured figure the caption
	// says "Predictions by EfficientNet (Tan and Le 2019)" and the 2019 is a
	// link carrying the full reference string in its title attribute, so the
	// figure is attributable without going back to the reference list.
	Refs []CaptionRef `json:"refs,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Image is one rendition of a figure.
//
// WebP is a separate field rather than a replacement because the page offers
// both and they are different bytes at the same url stem. Width and Height are
// what the page declares, not what the file turns out to be, and they are worth
// having precisely because they differ between renditions.
type Image struct {
	URL    string `json:"url,omitempty"`
	WebP   string `json:"webp,omitempty"`
	Alt    string `json:"alt,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// CaptionRef is a reference cited from inside a caption.
//
// Citation is the full reference string, which the page carries in the link's
// title attribute rather than in its text. The text is only the year.
type CaptionRef struct {
	Text     string `json:"text,omitempty"`
	URL      string `json:"url,omitempty"`
	Citation string `json:"citation,omitempty"`
}

// TablePage is the /tables/N subpage, and it is the only place the table body
// is published.
//
// This is not a convenience. The article page carries the table's caption and a
// link to here and nothing else: the open access capture has one inline table
// region and zero <table> elements in 718 KB of html. Whatever the table says,
// it says here or nowhere.
type TablePage struct {
	Label  string `json:"label,omitempty"`
	Number int    `json:"number,omitempty"`

	Caption string `json:"caption,omitempty"`
	URL     string `json:"url,omitempty"`

	Anchor       string `json:"anchor,omitempty"`
	ArticleURL   string `json:"article_url,omitempty"`
	ArticleTitle string `json:"article_title,omitempty"`

	// Head is the header row and Rows are the body rows, both as printed text.
	// Cells carry LaTeX in a span the browser hands to MathJax, and it is kept
	// as the source string rather than rendered, since \(\mathcal{X}\) is what
	// the publisher wrote and any rendering of it is this tool's opinion.
	Head []string   `json:"head,omitempty"`
	Rows [][]string `json:"rows,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Cols returns the widest row in the table, which is what a printer needs and
// what tells a consumer whether the header row covers every column.
func (t *TablePage) Cols() int {
	n := len(t.Head)
	for _, r := range t.Rows {
		if len(r) > n {
			n = len(r)
		}
	}
	return n
}
