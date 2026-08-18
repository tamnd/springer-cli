package spr

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// OpenAlex, the derived index.
//
// This is the only backend in this tool that answers "who cited this". Springer
// does not publish that direction and Crossref only holds a count of it. Here it
// is a filter, cites:W3014596384, and the answer is a list you can page through.
//
// The two directions are not symmetric and this file does not pretend they are.
// Outbound is a list on the record, referenced_works, 111 entries. Inbound is a
// query against the whole index and it costs one request per page of 200.
//
// The abstract arrives inverted, which is a word to positions map, and the
// concepts, the ROR institution ids and the field weighted citation impact are
// the three things this backend has that no other one here does.

const (
	// OpenAlexBase is the works endpoint.
	OpenAlexBase = "https://" + OpenAlexHost + "/works"

	// OpenAlexPageSize is the largest page the API serves. Inbound citations on
	// a well cited work run to eight pages of it.
	OpenAlexPageSize = 200
)

// OpenAlexWork is one record from /works/{id}.
type OpenAlexWork struct {
	// ID is the OpenAlex work id, W followed by digits, in its short form. It
	// is the key both citation directions are expressed in, so it is carried
	// even when the DOI is the identifier the caller asked with.
	ID  string `json:"id,omitempty"`
	DOI DOI    `json:"doi,omitempty"`

	Title    string `json:"title,omitempty"`
	Type     string `json:"type,omitempty"`
	Language string `json:"language,omitempty"`

	PublicationDate string `json:"publication_date,omitempty"`
	PublicationYear int    `json:"publication_year,omitempty"`

	// Source is the journal or the book series, with both ISSNs and the linking
	// ISSN that identifies the title across media.
	Source *OpenAlexSource `json:"source,omitempty"`

	Volume string `json:"volume,omitempty"`
	Issue  string `json:"issue,omitempty"`
	Pages  string `json:"pages,omitempty"`

	// Abstract is the inverted index put back in order. See invertAbstract for
	// what that reconstruction can and cannot recover.
	Abstract string `json:"abstract,omitempty"`

	Authors []OpenAlexAuthor `json:"authors,omitempty"`

	// Concepts is the older classification, scored, and Topics is the newer
	// one. Both are published, they disagree, and picking one for the caller
	// would be picking a vocabulary on their behalf.
	Concepts []OpenAlexTag `json:"concepts,omitempty"`
	Topics   []OpenAlexTag `json:"topics,omitempty"`

	OpenAccess *OpenAlexAccess `json:"open_access,omitempty"`

	// ReferencedWorks is the outbound direction, as OpenAlex ids.
	ReferencedWorks []string `json:"referenced_works,omitempty"`

	Counts   OpenAlexCounts `json:"counts"`
	Envelope Envelope       `json:"envelope"`
}

// OpenAlexCounts is what OpenAlex counts, under names that say OpenAlex.
type OpenAlexCounts struct {
	// Citations is cited_by_count as stored on the record. It is a stored
	// aggregate and it is not live: the measured record said 1,563 with an
	// updated_date two days old, while listing the same work's citations in the
	// same minute returned 1,554. Both numbers are OpenAlex's, nine apart, and
	// UpdatedDate below is how a reader tells which one they are holding.
	Citations int `json:"openalex_citations"`

	// References is referenced_works_count, 111 where Crossref deposited 122.
	// OpenAlex resolves a reference to a work in its own index or drops it, so
	// the difference is the eleven Crossref entries it could not place.
	References int `json:"openalex_references"`

	// FWCI is the field weighted citation impact, 113.99 on the measured work,
	// meaning it is cited 114 times as often as the average work of its field,
	// year and type.
	FWCI float64 `json:"fwci,omitempty"`

	// Percentile is citation_normalized_percentile, 0.99970283, with the two
	// flags OpenAlex publishes next to it rather than a threshold computed here.
	Percentile      float64 `json:"percentile,omitempty"`
	InTopOnePercent bool    `json:"in_top_1_percent,omitempty"`
	InTopTenPercent bool    `json:"in_top_10_percent,omitempty"`

	// ByYear is counts_by_year, newest first as OpenAlex sends it. It is the
	// only citation history any backend in this tool publishes.
	ByYear []YearCount `json:"by_year,omitempty"`

	// UpdatedDate is when OpenAlex last rebuilt this record, which is the date
	// Citations above is true as of.
	UpdatedDate string `json:"updated_date,omitempty"`
}

// YearCount is one year and the citations recorded in it.
type YearCount struct {
	Year  int `json:"year"`
	Count int `json:"count"`
}

// OpenAlexSource is the venue a work appeared in.
type OpenAlexSource struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`

	// ISSNL is the linking ISSN, the one identifier that names the title across
	// print and electronic rather than naming a medium.
	ISSNL ISSN   `json:"issn_l,omitempty"`
	ISSNs []ISSN `json:"issns,omitempty"`

	Publisher string `json:"publisher,omitempty"`
	Type      string `json:"type,omitempty"`
	IsOA      bool   `json:"is_oa,omitempty"`
	InDOAJ    bool   `json:"is_in_doaj,omitempty"`
}

// OpenAlexAuthor is one authorship, which is a person and where they were.
type OpenAlexAuthor struct {
	Name  string `json:"name,omitempty"`
	ID    string `json:"id,omitempty"`
	ORCID ORCID  `json:"orcid,omitempty"`

	// Position is OpenAlex's own word, "first", "middle" or "last".
	Position string `json:"position,omitempty"`

	IsCorresponding bool `json:"is_corresponding,omitempty"`

	Institutions []OpenAlexInstitution `json:"institutions,omitempty"`

	// RawAffiliations is the affiliation as printed on the paper, kept because
	// the resolved institution is a match and the string is the evidence.
	RawAffiliations []string `json:"raw_affiliations,omitempty"`
}

// OpenAlexInstitution is a resolved affiliation, with the ROR id that makes it
// joinable to anything else in the world.
type OpenAlexInstitution struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	ROR         ROR    `json:"ror,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	Type        string `json:"type,omitempty"`
}

// OpenAlexTag is one concept or topic and how strongly it applies.
type OpenAlexTag struct {
	Name  string  `json:"name"`
	ID    string  `json:"id,omitempty"`
	Score float64 `json:"score,omitempty"`

	// Level is a concept's depth in the concept tree, 0 for a root such as
	// Computer science. Topics have no level and leave it at zero, which is why
	// it is omitted rather than printed as a meaningful 0.
	Level int `json:"level,omitempty"`
}

// OpenAlexAccess is the open access state, in OpenAlex's vocabulary.
type OpenAlexAccess struct {
	IsOA   bool   `json:"is_oa"`
	Status string `json:"status,omitempty"`
	URL    string `json:"url,omitempty"`
}

// openAlexWire is the wire form. It is separate from the record for the same
// reason Crossref's is: the wire has an inverted abstract, ids as full urls and
// two overlapping classifications, and a consumer should see none of that.
type openAlexWire struct {
	ID              string `json:"id"`
	DOI             string `json:"doi"`
	Title           string `json:"title"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	Language        string `json:"language"`
	PublicationDate string `json:"publication_date"`
	PublicationYear int    `json:"publication_year"`
	UpdatedDate     string `json:"updated_date"`

	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`

	Biblio struct {
		Volume    string `json:"volume"`
		Issue     string `json:"issue"`
		FirstPage string `json:"first_page"`
		LastPage  string `json:"last_page"`
	} `json:"biblio"`

	PrimaryLocation struct {
		Source struct {
			ID          string   `json:"id"`
			DisplayName string   `json:"display_name"`
			ISSNL       string   `json:"issn_l"`
			ISSN        []string `json:"issn"`
			HostOrgName string   `json:"host_organization_name"`
			Type        string   `json:"type"`
			IsOA        bool     `json:"is_oa"`
			IsInDOAJ    bool     `json:"is_in_doaj"`
		} `json:"source"`
	} `json:"primary_location"`

	OpenAccess struct {
		IsOA     bool   `json:"is_oa"`
		OAStatus string `json:"oa_status"`
		OAURL    string `json:"oa_url"`
	} `json:"open_access"`

	Authorships []struct {
		AuthorPosition string `json:"author_position"`
		Author         struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			ORCID       string `json:"orcid"`
		} `json:"author"`
		Institutions []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			ROR         string `json:"ror"`
			CountryCode string `json:"country_code"`
			Type        string `json:"type"`
		} `json:"institutions"`
		IsCorresponding      bool     `json:"is_corresponding"`
		RawAffiliationString []string `json:"raw_affiliation_strings"`
	} `json:"authorships"`

	Concepts []struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"display_name"`
		Level       int     `json:"level"`
		Score       float64 `json:"score"`
	} `json:"concepts"`

	Topics []struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"display_name"`
		Score       float64 `json:"score"`
	} `json:"topics"`

	ReferencedWorks      []string `json:"referenced_works"`
	ReferencedWorksCount int      `json:"referenced_works_count"`
	CitedByCount         int      `json:"cited_by_count"`

	FWCI                         float64 `json:"fwci"`
	CitationNormalizedPercentile struct {
		Value          float64 `json:"value"`
		InTop1Percent  bool    `json:"is_in_top_1_percent"`
		InTop10Percent bool    `json:"is_in_top_10_percent"`
	} `json:"citation_normalized_percentile"`

	CountsByYear []struct {
		Year         int `json:"year"`
		CitedByCount int `json:"cited_by_count"`
	} `json:"counts_by_year"`
}

// openAlexPage is a list answer, which wraps results in a meta block.
type openAlexPage struct {
	Meta struct {
		Count   int     `json:"count"`
		Page    int     `json:"page"`
		PerPage int     `json:"per_page"`
		CostUSD float64 `json:"cost_usd"`
	} `json:"meta"`
	Results []openAlexWire `json:"results"`
	GroupBy []struct {
		Key            string `json:"key"`
		KeyDisplayName string `json:"key_display_name"`
		Count          int    `json:"count"`
	} `json:"group_by"`
}

// OpenAlexWork fetches one work by DOI.
func (c *Client) OpenAlexWork(ctx context.Context, d DOI) (*OpenAlexWork, error) {
	return c.openAlexRecord(ctx, OpenAlexBase+"/doi:"+strings.TrimSpace(string(d)))
}

// OpenAlexWorkByID fetches one work by its OpenAlex id, W followed by digits.
func (c *Client) OpenAlexWorkByID(ctx context.Context, id string) (*OpenAlexWork, error) {
	return c.openAlexRecord(ctx, OpenAlexBase+"/"+ShortOpenAlexID(id))
}

func (c *Client) openAlexRecord(ctx context.Context, target string) (*OpenAlexWork, error) {
	var wire openAlexWire
	resp, err := c.getJSON(ctx, target, &wire)
	if err != nil {
		return nil, err
	}
	w := openAlexRecord(&wire)
	w.Envelope.Tier = "openalex"
	w.Envelope.record(resp)
	openAlexProvenance(&w.Envelope, &wire)
	w.Envelope.sortMissed()
	return w, nil
}

// ShortOpenAlexID trims the url form of an OpenAlex id down to the id.
// Everything in the API is expressed in both, and W3014596384 is the form that
// fits in a filter and in a terminal.
func ShortOpenAlexID(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func openAlexRecord(w *openAlexWire) *OpenAlexWork {
	out := &OpenAlexWork{
		ID:              ShortOpenAlexID(w.ID),
		Title:           w.Title,
		Type:            w.Type,
		Language:        w.Language,
		PublicationDate: w.PublicationDate,
		PublicationYear: w.PublicationYear,
		Volume:          w.Biblio.Volume,
		Issue:           w.Biblio.Issue,
		Abstract:        invertAbstract(w.AbstractInvertedIndex),
		Counts: OpenAlexCounts{
			Citations:       w.CitedByCount,
			References:      w.ReferencedWorksCount,
			FWCI:            w.FWCI,
			Percentile:      w.CitationNormalizedPercentile.Value,
			InTopOnePercent: w.CitationNormalizedPercentile.InTop1Percent,
			InTopTenPercent: w.CitationNormalizedPercentile.InTop10Percent,
			UpdatedDate:     w.UpdatedDate,
		},
	}
	if out.Title == "" {
		out.Title = w.DisplayName
	}
	if d, err := ParseDOI(w.DOI); err == nil {
		out.DOI = d
	}
	out.Pages = joinPages(w.Biblio.FirstPage, w.Biblio.LastPage)

	if s := w.PrimaryLocation.Source; s.ID != "" || s.DisplayName != "" {
		src := &OpenAlexSource{
			ID:          ShortOpenAlexID(s.ID),
			DisplayName: s.DisplayName,
			Publisher:   s.HostOrgName,
			Type:        s.Type,
			IsOA:        s.IsOA,
			InDOAJ:      s.IsInDOAJ,
		}
		if issn, err := ParseISSN(s.ISSNL); err == nil {
			src.ISSNL = issn
		}
		for _, raw := range s.ISSN {
			if issn, err := ParseISSN(raw); err == nil {
				src.ISSNs = append(src.ISSNs, issn)
			}
		}
		out.Source = src
	}

	if w.OpenAccess.IsOA || w.OpenAccess.OAStatus != "" {
		out.OpenAccess = &OpenAlexAccess{
			IsOA:   w.OpenAccess.IsOA,
			Status: w.OpenAccess.OAStatus,
			URL:    w.OpenAccess.OAURL,
		}
	}

	for _, a := range w.Authorships {
		author := OpenAlexAuthor{
			Name:            a.Author.DisplayName,
			ID:              ShortOpenAlexID(a.Author.ID),
			Position:        a.AuthorPosition,
			IsCorresponding: a.IsCorresponding,
			RawAffiliations: a.RawAffiliationString,
		}
		if id, err := ParseORCID(a.Author.ORCID); err == nil {
			author.ORCID = id
		}
		for _, inst := range a.Institutions {
			i := OpenAlexInstitution{
				ID:          ShortOpenAlexID(inst.ID),
				DisplayName: inst.DisplayName,
				CountryCode: inst.CountryCode,
				Type:        inst.Type,
			}
			if r, err := ParseROR(inst.ROR); err == nil {
				i.ROR = r
			}
			author.Institutions = append(author.Institutions, i)
		}
		out.Authors = append(out.Authors, author)
	}

	for _, cpt := range w.Concepts {
		out.Concepts = append(out.Concepts, OpenAlexTag{
			Name:  cpt.DisplayName,
			ID:    ShortOpenAlexID(cpt.ID),
			Score: cpt.Score,
			Level: cpt.Level,
		})
	}
	for _, t := range w.Topics {
		out.Topics = append(out.Topics, OpenAlexTag{
			Name:  t.DisplayName,
			ID:    ShortOpenAlexID(t.ID),
			Score: t.Score,
		})
	}

	for _, r := range w.ReferencedWorks {
		out.ReferencedWorks = append(out.ReferencedWorks, ShortOpenAlexID(r))
	}
	for _, y := range w.CountsByYear {
		out.Counts.ByYear = append(out.Counts.ByYear, YearCount{Year: y.Year, Count: y.CitedByCount})
	}
	return out
}

func joinPages(first, last string) string {
	switch {
	case first != "" && last != "" && first != last:
		return first + "-" + last
	case first != "":
		return first
	}
	return last
}

// invertAbstract rebuilds the abstract from OpenAlex's word to positions map.
//
// The format is a copyright workaround: OpenAlex may not redistribute an
// abstract as prose, so it publishes the words and where each one occurs, and
// the text goes back together by sorting on position. That reconstruction is
// exact for the words and lossy for nothing else, because whitespace was never
// stored in the first place. A word appearing at three positions appears three
// times, which is the whole trick.
//
// The measured record has 94 distinct words at 141 positions running from 0 to
// 140, so nothing is missing from it and the reconstruction is the whole
// abstract. Nothing here checks for a gap or closes one: a hole in the position
// sequence is a hole in what OpenAlex published, and inventing a marker for it
// would put a token in the text that no publisher wrote.
//
// The leading "Abstract" is dropped, because it is the heading and not the
// first word of the abstract. Crossref's JATS carries the same heading in a
// jats:title element and jats() drops it there, and the two backends have to be
// comparable on this field or comparing them is measuring the headings.
func invertAbstract(inverted map[string][]int) string {
	if len(inverted) == 0 {
		return ""
	}
	type placed struct {
		at   int
		word string
	}
	words := make([]placed, 0, len(inverted))
	for word, positions := range inverted {
		for _, at := range positions {
			words = append(words, placed{at, word})
		}
	}
	sort.Slice(words, func(i, j int) bool {
		if words[i].at != words[j].at {
			return words[i].at < words[j].at
		}
		return words[i].word < words[j].word
	})
	parts := make([]string, 0, len(words))
	for _, w := range words {
		parts = append(parts, w.word)
	}
	if len(parts) > 0 && strings.EqualFold(parts[0], "abstract") {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}

// OpenAlexCitedBy lists the works that cite this one, which is the direction
// link.springer.com does not publish at all.
//
// The count returned is meta.count from the same request, and it is not the
// record's cited_by_count. The two were nine apart on the measured work, 1,554
// against 1,563, because the record's number is a stored aggregate rebuilt on
// its own schedule and this one is the live index. Reporting either as the
// citation count without saying which it is would be picking a number out of
// two that OpenAlex publishes for different reasons.
//
// limit of 0 means every page. Each page is one request and holds up to 200.
func (c *Client) OpenAlexCitedBy(ctx context.Context, id string, limit int) (works []OpenAlexWork, total int, err error) {
	id = ShortOpenAlexID(id)
	if id == "" {
		return nil, 0, fmt.Errorf("openalex: no work id to list citations of")
	}
	for page := 1; ; page++ {
		v := url.Values{}
		v.Set("filter", "cites:"+id)
		v.Set("per-page", strconv.Itoa(OpenAlexPageSize))
		v.Set("page", strconv.Itoa(page))
		// The full record is 19 KB and the listing needs six fields of it, so
		// the select is the difference between 8 pages at 3.8 MB and 8 pages at
		// a few hundred kilobytes.
		v.Set("select", "id,doi,title,publication_year,cited_by_count,type")

		var got openAlexPage
		if _, err := c.getJSON(ctx, OpenAlexBase+"?"+v.Encode(), &got); err != nil {
			return works, total, err
		}
		total = got.Meta.Count
		if len(got.Results) == 0 {
			return works, total, nil
		}
		for i := range got.Results {
			works = append(works, *openAlexRecord(&got.Results[i]))
			if limit > 0 && len(works) >= limit {
				return works[:limit], total, nil
			}
		}
		// OpenAlex caps basic paging at 10,000 records and answers an error
		// past it. Nothing measured comes close, and stopping at the stated
		// count is what ends the loop in practice.
		if len(works) >= total {
			return works, total, nil
		}
	}
}

// OpenAlexCitedByYear returns the inbound citation count grouped by year, in
// one request rather than the eight a full listing costs. The years come back
// newest first, which is the order group_by sends them in.
func (c *Client) OpenAlexCitedByYear(ctx context.Context, id string) ([]YearCount, int, error) {
	v := url.Values{}
	v.Set("filter", "cites:"+ShortOpenAlexID(id))
	v.Set("group_by", "publication_year")

	var got openAlexPage
	if _, err := c.getJSON(ctx, OpenAlexBase+"?"+v.Encode(), &got); err != nil {
		return nil, 0, err
	}
	var out []YearCount
	for _, g := range got.GroupBy {
		year, err := strconv.Atoi(g.Key)
		if err != nil {
			continue
		}
		out = append(out, YearCount{Year: year, Count: g.Count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Year > out[j].Year })
	return out, got.Meta.Count, nil
}

// OpenAlexQuery is a search of the works endpoint.
type OpenAlexQuery struct {
	// Search is full text, which OpenAlex scores and returns a relevance_score
	// for. Title is the narrower title.search filter.
	Search string
	Title  string
	Author string

	ISSN ISSN

	// From and Until are from_publication_date and to_publication_date, both of
	// which want a full date.
	From  string
	Until string

	// Cites and CitedBy are the two graph filters. Cites lists the works citing
	// the given id, and CitedBy lists the works it cites.
	Cites   string
	CitedBy string

	PerPage int
	Page    int
	Select  []string
}

// URL builds the request url for a query.
func (q OpenAlexQuery) URL() string {
	v := url.Values{}
	var filters []string
	add := func(k, s string) {
		if strings.TrimSpace(s) != "" {
			filters = append(filters, k+":"+s)
		}
	}
	// Free text is a filter here rather than its own parameter, except for
	// search, which OpenAlex takes at the top level and scores.
	if strings.TrimSpace(q.Search) != "" {
		v.Set("search", q.Search)
	}
	add("title.search", q.Title)
	add("raw_author_name.search", q.Author)
	add("primary_location.source.issn", string(q.ISSN))
	add("from_publication_date", q.From)
	add("to_publication_date", q.Until)
	add("cites", ShortOpenAlexID(q.Cites))
	add("cited_by", ShortOpenAlexID(q.CitedBy))
	if len(filters) > 0 {
		v.Set("filter", strings.Join(filters, ","))
	}
	if q.PerPage > 0 {
		v.Set("per-page", strconv.Itoa(q.PerPage))
	}
	if q.Page > 0 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	if len(q.Select) > 0 {
		v.Set("select", strings.Join(q.Select, ","))
	}
	return OpenAlexBase + "?" + v.Encode()
}

// OpenAlexResult is one page of a query.
type OpenAlexResult struct {
	Total int            `json:"total"`
	Items []OpenAlexWork `json:"items"`

	// CostUSD is what OpenAlex charged this request against the metered budget
	// it reports in its headers. A filter page cost $0.0001 and a search page
	// cost ten times that, against a stated budget of $0.10, which makes the
	// money the binding limit on that host long before the request count is.
	CostUSD float64 `json:"cost_usd,omitempty"`

	Envelope Envelope `json:"envelope"`
}

// OpenAlexSearch runs one query and returns one page.
func (c *Client) OpenAlexSearch(ctx context.Context, q OpenAlexQuery) (*OpenAlexResult, error) {
	target := q.URL()
	var got openAlexPage
	resp, err := c.getJSON(ctx, target, &got)
	if err != nil {
		return nil, err
	}
	out := &OpenAlexResult{Total: got.Meta.Count, CostUSD: got.Meta.CostUSD}
	for i := range got.Results {
		out.Items = append(out.Items, *openAlexRecord(&got.Results[i]))
	}
	out.Envelope.Tier = "openalex"
	out.Envelope.record(resp)
	out.Envelope.carry("total", "openalex:meta.count")
	if len(out.Items) > 0 {
		out.Envelope.carry("items", "openalex:results[]")
	}
	return out, nil
}

// openAlexProvenance writes where each field came from, and what was published
// and empty rather than absent.
func openAlexProvenance(e *Envelope, w *openAlexWire) {
	say := func(field, path string) { e.carry(field, "openalex:"+path) }

	say("id", "id")
	if w.DOI != "" {
		say("doi", "doi")
	}
	if len(w.AbstractInvertedIndex) > 0 {
		say("abstract", fmt.Sprintf("abstract_inverted_index, %d distinct words put back in position order", len(w.AbstractInvertedIndex)))
	} else {
		e.miss("abstract", "no inverted index published for this work")
	}
	if len(w.Authorships) > 0 {
		say("authors", "authorships[]")
		named, rored := 0, 0
		for _, a := range w.Authorships {
			if a.Author.ID != "" {
				named++
			}
			for _, i := range a.Institutions {
				if i.ROR != "" {
					rored++
					break
				}
			}
		}
		if rored > 0 {
			say("authors.institutions", "authorships[].institutions[].ror")
		}
		if named < len(w.Authorships) {
			e.miss("authors.id", fmt.Sprintf("%d of %d authorships carry a display name and orcid with a null author id, so the person is named and not addressable", len(w.Authorships)-named, len(w.Authorships)))
		}
	}
	if len(w.Concepts) > 0 {
		say("concepts", "concepts[]")
	}
	if len(w.Topics) > 0 {
		say("topics", "topics[]")
	}
	say("counts.openalex_citations", "cited_by_count, as stored on "+w.UpdatedDate)
	say("counts.openalex_references", "referenced_works_count")
	if w.FWCI != 0 {
		say("counts.fwci", "fwci")
	}
	if w.CitationNormalizedPercentile.Value != 0 {
		say("counts.percentile", "citation_normalized_percentile.value")
	}
	if len(w.CountsByYear) > 0 {
		say("counts.by_year", "counts_by_year[]")
	}
	if len(w.ReferencedWorks) > 0 {
		say("referenced_works", "referenced_works[]")
	}
}
