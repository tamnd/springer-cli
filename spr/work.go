package spr

// Work is the central record. Four content types share it: article, chapter,
// protocol and entry. The Type field says which, and no field is invented for
// one type and faked for the others, so a chapter simply has no volume and an
// article simply has no isbn.
//
// Every field is omitempty on purpose. Absent means absent: a field the page
// did not carry is left out of the JSON rather than emitted as null, zero or
// an empty string, and a field that was declared and did not fill is named in
// Envelope.Missed with a reason. A consumer can therefore tell the three cases
// apart, which is the whole point.
type Work struct {
	// Identity.
	DOI         string `json:"doi,omitempty"`
	Type        string `json:"type,omitempty"`
	ArticleType string `json:"article_type,omitempty"`

	// URL is the requested url, never the effective one. The effective url
	// carries a per request uuid from the cookie dance and is not an identifier.
	URL         string `json:"url,omitempty"`
	PDFURL      string `json:"pdf_url,omitempty"`
	FulltextURL string `json:"fulltext_url,omitempty"`
	AbstractURL string `json:"abstract_url,omitempty"`
	APIURL      string `json:"api_url,omitempty"`
	MetricsURL  string `json:"metrics_url,omitempty"`

	// Bibliographic core.
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	Abstract string `json:"abstract,omitempty"`
	Language string `json:"language,omitempty"`

	// Keywords are the authors' own terms and Subjects are Springer's
	// classification. Two lists from two vocabularies, kept apart, because
	// merging them would put a publisher's taxonomy in a field a reader takes
	// to be the authors' words.
	Keywords []string `json:"keywords,omitempty"`
	Subjects []string `json:"subjects,omitempty"`

	// SectionLabel is the article's section within its issue, not a heading.
	SectionLabel string `json:"section_label,omitempty"`

	// Container and pagination.
	Container       *Ref   `json:"container,omitempty"`
	ContainerTitle  string `json:"container_title,omitempty"`
	ContainerAbbrev string `json:"container_abbrev,omitempty"`

	// ISSN is a list because a journal has a print and an electronic one and
	// the page gives both. JSON-LD returned two where Highwire gave one, so
	// both are kept, deduplicated, in the order the higher rung supplied.
	ISSN []string `json:"issn,omitempty"`

	ISBN      string `json:"isbn,omitempty"`
	Volume    string `json:"volume,omitempty"`
	Issue     string `json:"issue,omitempty"`
	FirstPage string `json:"first_page,omitempty"`
	LastPage  string `json:"last_page,omitempty"`

	// Pages is derived, and only when both ends parse as numbers. A page range
	// of xiv to xxii is real and is not arithmetic.
	Pages int `json:"pages,omitempty"`

	Series     *Ref   `json:"series,omitempty"`
	Conference *Ref   `json:"conference,omitempty"`
	Publisher  string `json:"publisher,omitempty"`

	// Four dates, and they are four different facts: when the issue is dated,
	// when it went online, what the cover says, and when the record last
	// changed. Flattening them into one would be the single most common cause
	// of a wrong sort in a bibliographic tool.
	Published *Date `json:"published,omitempty"`
	Online    *Date `json:"online,omitempty"`
	CoverDate *Date `json:"cover_date,omitempty"`
	Modified  *Date `json:"modified,omitempty"`

	Authors       []Author `json:"authors,omitempty"`
	Editors       []Author `json:"editors,omitempty"`
	Corresponding []string `json:"corresponding,omitempty"`

	Access Access `json:"access"`

	License     string `json:"license,omitempty"`
	Copyright   string `json:"copyright,omitempty"`
	Rights      string `json:"rights,omitempty"`
	RightsAgent string `json:"rights_agent,omitempty"`

	// Body.
	Sections  []Section `json:"sections,omitempty"`
	BodyText  string    `json:"body_text,omitempty"`
	Figures   []Figure  `json:"figures,omitempty"`
	Equations int       `json:"equations,omitempty"`
	Footnotes []string  `json:"footnotes,omitempty"`

	// Rails is the recommender strip, kept out of Sections.
	Rails []Ref `json:"rails,omitempty"`

	References      []Reference `json:"references,omitempty"`
	ReferencesCount int         `json:"references_count,omitempty"`
	ReferenceLinks  int         `json:"reference_links,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// Author is one person on a work.
//
// Family and Given are never split out of Name by this tool. Name splitting is
// wrong often enough across the world's naming conventions that a guess is
// worse than an absence, and Crossref publishes the split already. Those two
// fields fill only when that tier has been reached.
type Author struct {
	Name         string        `json:"name,omitempty"`
	Family       string        `json:"family,omitempty"`
	Given        string        `json:"given,omitempty"`
	ORCID        string        `json:"orcid,omitempty"`
	Email        string        `json:"email,omitempty"`
	Affiliations []Affiliation `json:"affiliations,omitempty"`
	Sequence     string        `json:"sequence,omitempty"`
	Position     int           `json:"position"`
}

// Affiliation is where an author says they work. The page gives a name and a
// postal address. ROR, country and a stable identifier come only from OpenAlex.
type Affiliation struct {
	Name          string `json:"name,omitempty"`
	Address       string `json:"address,omitempty"`
	ROR           string `json:"ror,omitempty"`
	Country       string `json:"country,omitempty"`
	InstitutionID string `json:"institution_id,omitempty"`
}

// Access is what this reader is being given, which is a different question
// from how the work is licensed.
//
// Free is what the page says about this request. OAStatus is a third party's
// classification of the work itself, and they disagree: a work OpenAlex reports
// as closed served 622 KB of full text with access Yes. Both are recorded,
// under different names, and this tool never reconciles them.
type Access struct {
	// Free is a pointer so that a page which declared nothing is
	// distinguishable from a page which declared No.
	Free *bool  `json:"free,omitempty"`
	Raw  string `json:"raw,omitempty"`

	// WorldReadable is citation_fulltext_world_readable, which is present and
	// empty on both article captures including the open access one, and is not
	// shipped at all on a chapter, a protocol or an entry. It is a pointer
	// because present-and-empty is the recorded fact, and a plain string would
	// make it indistinguishable from a tag that had gone away.
	WorldReadable *string `json:"world_readable,omitempty"`

	// Statement is the page's own sentence where the body would be.
	Statement string `json:"statement,omitempty"`

	OAStatus        string `json:"oa_status,omitempty"`
	OAURL           string `json:"oa_url,omitempty"`
	PublishingModel string `json:"publishing_model,omitempty"`
}

// Section is one node of the article's section tree.
//
// There are no id attributes on Springer's article sections, so Position is the
// only stable handle and this tool does not synthesize an anchor that would
// look stable and not be. Number is the printed number where the page prints
// one, which it does for the body and not for the back matter.
type Section struct {
	Number   string `json:"number,omitempty"`
	Title    string `json:"title,omitempty"`
	Depth    int    `json:"depth"`
	Position int    `json:"position"`
	Text     string `json:"text,omitempty"`
}

// Figure is one figure, read from the article page itself. The list, the
// captions and the links are all there already, so the default costs nothing
// and only the full resolution asset needs a request of its own.
type Figure struct {
	Label       string `json:"label,omitempty"`
	Caption     string `json:"caption,omitempty"`
	Description string `json:"description,omitempty"`
	PageURL     string `json:"page_url,omitempty"`
	Image       string `json:"image,omitempty"`
}

// Reference is one entry of the reference list.
//
// Text is the string Springer published and is kept even when the pairs parse,
// because the pairs are lossy. Structured false is a normal outcome and not a
// failure: 54 of the 122 references on the open access capture are free text
// with no pairs at all, and a reference nobody can identify produces no
// citation edge, which is the correct outcome rather than a missing one.
type Reference struct {
	Position   int    `json:"position"`
	Structured bool   `json:"structured"`
	Text       string `json:"text,omitempty"`

	Title     string   `json:"title,omitempty"`
	Journal   string   `json:"journal,omitempty"`
	Authors   []string `json:"authors,omitempty"`
	Year      string   `json:"year,omitempty"`
	Volume    string   `json:"volume,omitempty"`
	Issue     string   `json:"issue,omitempty"`
	FirstPage string   `json:"first_page,omitempty"`

	DOI    string `json:"doi,omitempty"`
	WorkID string `json:"work_id,omitempty"`
	Links  []Link `json:"links,omitempty"`
}

// Ref is a typed pointer to another record, used everywhere one entity names
// another. It is never resolved automatically: it is an identifier and a name,
// and fetching it is a decision the caller makes.
type Ref struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}
