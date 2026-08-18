package spr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Turning records into nodes and edges.
//
// Every function here takes a record this tool already produces and adds what
// it states to a graph. None of them makes a request and none of them infers a
// relation the record did not state. The rule the whole file follows is that an
// identifier that did not parse produces no uri, an absent uri produces no
// edge, and a relation nobody asserted does not appear.

// AddWork adds one work and everything it names.
//
// rails controls the recommender strip, which is off by default because it is
// the publisher's recommender output rather than anything the authors or the
// citation record assert.
func (g *Graph) AddWork(w *Work, rails bool) string {
	if w == nil {
		return ""
	}
	doi, err := ParseDOI(w.DOI)
	if err != nil {
		// A work with no DOI has no identity in this uri space and nothing to
		// hang an edge on. It is recorded as missed rather than given a hashed
		// name, because inventing a key for the one entity on this site that
		// always has a real one would be inventing a fact.
		g.Envelope.miss("work.doi", "the record carries no doi that parses, so it has no node")
		return ""
	}
	tier, at := w.Envelope.Tier, w.Envelope.Fetched
	uri := WorkURI(doi)

	props := map[string]string{}
	putIf(props, "doi", string(doi))
	putIf(props, "type", w.Type)
	putIf(props, "url", w.URL)
	putIf(props, "language", w.Language)
	if w.Published != nil {
		putIf(props, "published", w.Published.String())
	}
	if w.Access.Free != nil {
		props["free"] = strconv.FormatBool(*w.Access.Free)
	}
	if w.Access.Statement != "" {
		// The publisher's own sentence, kept verbatim, because it is the only
		// place a paywalled page says what it is withholding.
		putIf(props, "access_statement", w.Access.Statement)
	}
	g.AddNode(Node{
		URI: uri, Kind: NodeWork, Label: w.Title, Props: props,
		Via: "highwire:citation_doi", Tier: tier, Fetched: at,
	})

	g.addContainer(w, uri, tier, at)
	g.addPeople(uri, w.Authors, EdgeAuthoredBy, "jsonld:author[]", tier, at)
	g.addPeople(uri, w.Editors, EdgeEditedBy, "region:editors", tier, at)
	g.addSubjects(uri, w.Subjects, "dc:subject", tier, at)
	g.addPublisher(uri, w.Publisher, "highwire:citation_publisher", tier, at)
	g.addReferences(uri, w.References, tier, at)

	if rails {
		for _, r := range w.Rails {
			if d, err := ParseDOI(r.ID); err == nil {
				g.AddNode(Node{URI: WorkURI(d), Kind: NodeWork, Label: r.Name, Tier: tier, Fetched: at})
				g.AddEdge(Edge{
					From: uri, To: WorkURI(d), Kind: EdgeRecommends,
					Via: "region:inline-recommendations", Tier: tier, Fetched: at,
				})
			}
		}
	}
	return uri
}

// addContainer wires a work to whatever holds it, which is a journal, a book or
// a reference work, plus the volume and issue inside a journal.
func (g *Graph) addContainer(w *Work, work, tier string, at time.Time) {

	// A journal is identified by its ISSN. The page gives up to two of them and
	// does not say which is which, so each one that parses gets a node and they
	// are tied together with sameAs rather than one being picked as canonical.
	var journal string
	var journals []string
	for _, raw := range w.ISSN {
		i, err := ParseISSN(raw)
		if err != nil {
			g.Envelope.miss("journal.issn", fmt.Sprintf("%q did not pass its check digit, so it names no journal", raw))
			continue
		}
		u := JournalURI(i)
		journals = append(journals, u)
		g.AddNode(Node{
			URI: u, Kind: NodeJournal, Label: w.ContainerTitle,
			Props: prop("issn", string(i)),
			Via:   "highwire:citation_issn", Tier: tier, Fetched: at,
		})
		if journal == "" {
			journal = u
		}
	}
	for i := 1; i < len(journals); i++ {
		g.sameAs(journals[0], journals[i], "identifier equivalence", tier, at)
	}

	// The publisher's own journal number joins a graph built from urls to one
	// built from identifiers, which is why it is a second uri and a sameAs
	// rather than a property nobody can traverse.
	if id, ok := SpringerIDFromDOI(mustDOI(w.DOI)); ok {
		u := SpringerJournalURI(id)
		g.AddNode(Node{
			URI: u, Kind: NodeJournal, Label: w.ContainerTitle,
			Props: prop("springer_id", string(id)),
			Via:   "doi:suffix", Tier: tier, Fetched: at,
		})
		if journal != "" {
			g.sameAs(journal, u, "the same journal by its publisher number", tier, at)
		} else {
			journal = u
		}
	}

	// The journal nodes above stand on their own, so they are added before this
	// check rather than after it. The edges below cannot be, because an edge
	// from a work with no DOI has no end to start at.
	if work == "" {
		return
	}

	switch {
	case journal != "":
		g.AddEdge(Edge{From: work, To: journal, Kind: EdgePartOf, Via: "highwire:citation_journal_title", Tier: tier, Fetched: at})

		// A volume number is meaningless on its own, so both live under the
		// journal uri and an issue lives under its volume.
		if vol := VolumeURI(journal, w.Volume); vol != "" {
			g.AddNode(Node{URI: vol, Kind: NodeVolume, Label: w.Volume, Via: "highwire:citation_volume", Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: work, To: vol, Kind: EdgeInVolume, Via: "highwire:citation_volume", Tier: tier, Fetched: at})
			if iss := IssueURI(vol, w.Issue); iss != "" {
				g.AddNode(Node{URI: iss, Kind: NodeIssue, Label: w.Issue, Via: "highwire:citation_issue", Tier: tier, Fetched: at})
				g.AddEdge(Edge{From: work, To: iss, Kind: EdgeInIssue, Via: "highwire:citation_issue", Tier: tier, Fetched: at})
			}
		}

	case w.ISBN != "":
		// A chapter belongs to a book, and the book's identity is its ISBN.
		if isbn, err := ParseISBN(w.ISBN); err == nil {
			u := BookURI(isbn)
			g.AddNode(Node{
				URI: u, Kind: NodeBook, Label: w.ContainerTitle,
				Props: prop("isbn", string(isbn)),
				Via:   "highwire:citation_isbn", Tier: tier, Fetched: at,
			})
			g.AddEdge(Edge{From: work, To: u, Kind: EdgePartOf, Via: "highwire:citation_inbook_title", Tier: tier, Fetched: at})
		} else {
			g.Envelope.miss("book.isbn", fmt.Sprintf("%q did not pass its check digit, so it names no book", w.ISBN))
		}
	}

	if w.Series != nil {
		if u := seriesURIOf(w.Series); u != "" {
			g.AddNode(Node{URI: u, Kind: NodeSeries, Label: w.Series.Name, Via: "highwire:citation_series_title", Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: work, To: u, Kind: EdgeInSeries, Via: "highwire:citation_series_title", Tier: tier, Fetched: at})
		}
	}
	if w.Conference != nil {
		if u := ConferenceURI(w.Conference.ID, 0); u != "" {
			g.AddNode(Node{URI: u, Kind: NodeConference, Label: w.Conference.Name, Via: "highwire:citation_conference_title", Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: work, To: u, Kind: EdgePresentedAt, Via: "highwire:citation_conference_title", Tier: tier, Fetched: at})
		}
	}
}

// addPeople wires an author or editor list to a work or a container.
//
// The order is carried on the edge because author position means something in
// every field that publishes here: first author, last author and corresponding
// author are three different roles, and a graph that loses the order cannot
// tell any of them apart.
func (g *Graph) addPeople(subject string, people []Author, kind EdgeKind, via, tier string, at time.Time) {
	for i, p := range people {
		uri, source := personURI(p)
		if uri == "" {
			continue
		}
		props := map[string]string{}
		putIf(props, "name", p.Name)
		putIf(props, "orcid", string(mustORCID(p.ORCID)))
		g.AddNode(Node{URI: uri, Kind: NodePerson, Label: p.Name, Props: props, Via: source, Tier: tier, Fetched: at})

		// The records count an author list from zero and an edge counts from
		// one, because position 1 on an edge should mean first author rather
		// than second. The conversion happens here, once, rather than in every
		// reader of the graph.
		g.AddEdge(Edge{
			From: subject, To: uri, Kind: kind,
			Position: i + 1, Sequence: p.Sequence, Role: p.Role,
			Via: via, Tier: tier, Fetched: at,
		})

		for _, a := range p.Affiliations {
			org, orgVia := orgURI(a)
			if org == "" {
				continue
			}
			oprops := map[string]string{}
			putIf(oprops, "name", a.Name)
			putIf(oprops, "ror", string(mustROR(a.ROR)))
			putIf(oprops, "country", a.Country)
			g.AddNode(Node{URI: org, Kind: NodeOrg, Label: a.Name, Props: oprops, Via: orgVia, Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: uri, To: org, Kind: EdgeAffiliatedWith, Via: orgVia, Tier: tier, Fetched: at})
		}
	}
}

func (g *Graph) addSubjects(subject string, terms []string, via, tier string, at time.Time) {
	for _, t := range terms {
		u := SubjectURI(t)
		if u == "" {
			continue
		}
		g.AddNode(Node{URI: u, Kind: NodeSubject, Label: t, Via: via, Tier: tier, Fetched: at})
		g.AddEdge(Edge{From: subject, To: u, Kind: EdgeHasSubject, Via: via, Tier: tier, Fetched: at})
	}
}

func (g *Graph) addPublisher(subject, name, via, tier string, at time.Time) {
	u := PublisherURI(name)
	if u == "" {
		return
	}
	g.AddNode(Node{URI: u, Kind: NodePublisher, Label: name, Props: prop("name", name), Via: via, Tier: tier, Fetched: at})
	g.AddEdge(Edge{From: subject, To: u, Kind: EdgePublishedBy, Via: via, Tier: tier, Fetched: at})
}

// addReferences turns a reference list into citation edges, and mostly does
// not.
//
// The article page publishes 122 references and 54 of them are free text with
// no structured pairs at all. A reference that carries no DOI produces no edge,
// because a citation to a work you cannot identify is not a citation edge, and
// the count of those goes into the notes so that a graph with 66 edges off a
// paper with 122 references is explicable rather than suspicious.
func (g *Graph) addReferences(work string, refs []Reference, tier string, at time.Time) {
	var linked, unresolved int
	for _, r := range refs {
		d, err := ParseDOI(r.DOI)
		if err != nil {
			unresolved++
			continue
		}
		to := WorkURI(d)
		g.AddNode(Node{URI: to, Kind: NodeWork, Label: r.Title, Via: "reference:doi", Tier: tier, Fetched: at})
		g.AddEdge(Edge{
			From: work, To: to, Kind: EdgeCites, Position: r.Position + 1,
			Via: "highwire:citation_reference", Tier: tier, Fetched: at,
		})
		linked++
	}
	if unresolved > 0 {
		g.Notes = append(g.Notes, fmt.Sprintf(
			"%d of %d references resolved to an identifier and became edges, and %d did not and stay in the record as text",
			linked, linked+unresolved, unresolved))
	}
}

// AddCrossrefWork adds what the registration agency holds, which is the funder
// list, the deposited author order and the reference list as identifiers.
func (g *Graph) AddCrossrefWork(w *CrossrefWork) string {
	if w == nil || w.DOI == "" {
		return ""
	}
	tier, at := w.Envelope.Tier, w.Envelope.Fetched
	uri := WorkURI(w.DOI)

	props := map[string]string{}
	putIf(props, "doi", string(w.DOI))
	putIf(props, "type", w.Type)
	putIf(props, "url", w.URL)
	if w.Issued != nil {
		putIf(props, "published", w.Issued.String())
	}
	g.AddNode(Node{URI: uri, Kind: NodeWork, Label: w.Title, Props: props, Via: "crossref:DOI", Tier: tier, Fetched: at})

	// The container, by whichever ISSN the deposit typed. Both are deposited
	// with their medium, which is more than the page says.
	var journal string
	for _, s := range w.ISSNs {
		u := JournalURI(s.Value)
		if u == "" {
			continue
		}
		g.AddNode(Node{
			URI: u, Kind: NodeJournal, Label: w.ContainerTitle,
			Props: map[string]string{"issn": string(s.Value), "issn_type": s.Type},
			Via:   "crossref:issn-type", Tier: tier, Fetched: at,
		})
		if journal == "" {
			journal = u
		} else {
			g.sameAs(journal, u, "the print and electronic issn of one journal", tier, at)
		}
	}
	if journal != "" {
		g.AddEdge(Edge{From: uri, To: journal, Kind: EdgePartOf, Via: "crossref:container-title", Tier: tier, Fetched: at})
		if vol := VolumeURI(journal, w.Volume); vol != "" {
			g.AddNode(Node{URI: vol, Kind: NodeVolume, Label: w.Volume, Via: "crossref:volume", Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: uri, To: vol, Kind: EdgeInVolume, Via: "crossref:volume", Tier: tier, Fetched: at})
			if iss := IssueURI(vol, w.Issue); iss != "" {
				g.AddNode(Node{URI: iss, Kind: NodeIssue, Label: w.Issue, Via: "crossref:issue", Tier: tier, Fetched: at})
				g.AddEdge(Edge{From: uri, To: iss, Kind: EdgeInIssue, Via: "crossref:issue", Tier: tier, Fetched: at})
			}
		}
	}

	g.addPeople(uri, crossrefAuthors(w.Authors), EdgeAuthoredBy, "crossref:author[]", tier, at)
	g.addPeople(uri, crossrefAuthors(w.Editors), EdgeEditedBy, "crossref:editor[]", tier, at)
	g.addPublisher(uri, w.Publisher, "crossref:publisher", tier, at)
	g.addSubjects(uri, w.Subjects, "crossref:subject", tier, at)

	// The funder is the one relation only this tier states. Projekt DEAL is
	// deposited with a name and no id on the measured work, so half of these
	// edges land on a name keyed node that joins to nothing, and the uri says
	// which half a given one is.
	for _, f := range w.Funders {
		u := FunderURI(mustDOI(f.DOI), f.Name)
		if u == "" {
			continue
		}
		props := map[string]string{"name": f.Name}
		putIf(props, "funder_doi", f.DOI)
		if len(f.Awards) > 0 {
			props["awards"] = strings.Join(f.Awards, ", ")
		}
		g.AddNode(Node{URI: u, Kind: NodeFunder, Label: f.Name, Props: props, Via: "crossref:funder[]", Tier: tier, Fetched: at})
		g.AddEdge(Edge{From: uri, To: u, Kind: EdgeFundedBy, Via: "crossref:funder[]", Tier: tier, Fetched: at})
	}
	return uri
}

// AddCrossrefReferences adds the deposited reference list as citation edges.
//
// unresolved is the count of entries that carried no DOI, which is 56 of 122 on
// the measured work. They produce nothing here, and the note is the only place
// that says so.
func (g *Graph) AddCrossrefReferences(work DOI, refs []DOI, unresolved int, at time.Time) {
	fromURI := WorkURI(work)
	if fromURI == "" {
		return
	}
	for i, d := range refs {
		to := WorkURI(d)
		g.AddNode(Node{URI: to, Kind: NodeWork, Via: "crossref:reference[].DOI", Tier: "crossref", Fetched: at})
		g.AddEdge(Edge{
			From: fromURI, To: to, Kind: EdgeCites, Position: i + 1,
			Via: "crossref:reference[].DOI", Tier: "crossref", Fetched: at,
		})
	}
	if unresolved > 0 {
		g.Notes = append(g.Notes, fmt.Sprintf(
			"crossref: %d of %d deposited references carry a doi and became edges, and %d do not and became nothing",
			len(refs), len(refs)+unresolved, unresolved))
	}
}

// AddOpenAlexWork adds what the open index derived, which is the ROR ids and
// both citation directions.
func (g *Graph) AddOpenAlexWork(w *OpenAlexWork) string {
	if w == nil || w.DOI == "" {
		return ""
	}
	tier, at := w.Envelope.Tier, w.Envelope.Fetched
	uri := WorkURI(w.DOI)

	props := map[string]string{}
	putIf(props, "doi", string(w.DOI))
	putIf(props, "type", w.Type)
	putIf(props, "openalex_id", w.ID)
	putIf(props, "published", w.PublicationDate)
	if w.OpenAccess != nil {
		props["free"] = strconv.FormatBool(w.OpenAccess.IsOA)
		putIf(props, "oa_status", w.OpenAccess.Status)
	}
	g.AddNode(Node{URI: uri, Kind: NodeWork, Label: w.Title, Props: props, Via: "openalex:id", Tier: tier, Fetched: at})

	if w.Source != nil {
		var journal string
		for _, i := range append([]ISSN{w.Source.ISSNL}, w.Source.ISSNs...) {
			u := JournalURI(i)
			if u == "" {
				continue
			}
			g.AddNode(Node{
				URI: u, Kind: NodeJournal, Label: w.Source.DisplayName,
				Props: prop("issn", string(i)),
				Via:   "openalex:primary_location.source", Tier: tier, Fetched: at,
			})
			if journal == "" {
				journal = u
			} else {
				g.sameAs(journal, u, "the issns of one source", tier, at)
			}
		}
		if journal != "" {
			g.AddEdge(Edge{From: uri, To: journal, Kind: EdgePartOf, Via: "openalex:primary_location.source", Tier: tier, Fetched: at})
			g.addPublisher(journal, w.Source.Publisher, "openalex:source.host_organization_name", tier, at)
			if vol := VolumeURI(journal, w.Volume); vol != "" {
				g.AddNode(Node{URI: vol, Kind: NodeVolume, Label: w.Volume, Via: "openalex:biblio.volume", Tier: tier, Fetched: at})
				g.AddEdge(Edge{From: uri, To: vol, Kind: EdgeInVolume, Via: "openalex:biblio.volume", Tier: tier, Fetched: at})
			}
		}
	}

	// The authorship is the reason this tier is here. Crossref deposits an
	// empty affiliation array for every author on this work and OpenAlex has a
	// ROR id for the same people, which is an institution you can join on
	// rather than a string you can only compare.
	for i, a := range w.Authors {
		uriP, source := personURI(Author{Name: a.Name, ORCID: string(a.ORCID)})
		if uriP == "" {
			continue
		}
		props := map[string]string{}
		putIf(props, "name", a.Name)
		putIf(props, "orcid", string(a.ORCID))
		putIf(props, "openalex_id", a.ID)
		g.AddNode(Node{URI: uriP, Kind: NodePerson, Label: a.Name, Props: props, Via: source, Tier: tier, Fetched: at})
		g.AddEdge(Edge{
			From: uri, To: uriP, Kind: EdgeAuthoredBy, Position: i + 1, Sequence: a.Position,
			Via: "openalex:authorships[]", Tier: tier, Fetched: at,
		})
		for _, in := range a.Institutions {
			org := OrgRORURI(in.ROR)
			via := "openalex:institutions[].ror"
			if org == "" {
				org, via = OrgNameURI(in.DisplayName), "openalex:institutions[].display_name"
			}
			if org == "" {
				continue
			}
			oprops := map[string]string{}
			putIf(oprops, "name", in.DisplayName)
			putIf(oprops, "ror", string(in.ROR))
			putIf(oprops, "country", in.CountryCode)
			putIf(oprops, "type", in.Type)
			g.AddNode(Node{URI: org, Kind: NodeOrg, Label: in.DisplayName, Props: oprops, Via: via, Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: uriP, To: org, Kind: EdgeAffiliatedWith, Via: via, Tier: tier, Fetched: at})
		}
	}

	// Both classifications, because OpenAlex publishes both and they disagree,
	// and each edge says which vocabulary it came from.
	for _, t := range w.Concepts {
		g.addSubjects(uri, []string{t.Name}, "openalex:concepts[]", tier, at)
	}
	for _, t := range w.Topics {
		g.addSubjects(uri, []string{t.Name}, "openalex:topics[]", tier, at)
	}
	return uri
}

// AddCitedBy adds the direction link.springer.com has no page for.
//
// The edge runs from the citing work to this one, because cites is the relation
// the data states and citedBy is its inverse. Both are emitted, so a consumer
// that only walks one direction still finds everything, and both carry the same
// provenance.
func (g *Graph) AddCitedBy(work DOI, citing []OpenAlexWork, at time.Time) {
	to := WorkURI(work)
	if to == "" {
		return
	}
	for _, c := range citing {
		if c.DOI == "" {
			// A citing work with no DOI is real and is not addressable in this
			// uri space, so it is counted in the note and not given a node.
			continue
		}
		fromURI := WorkURI(c.DOI)
		g.AddNode(Node{URI: fromURI, Kind: NodeWork, Label: c.Title, Via: "openalex:cites", Tier: "openalex", Fetched: at})
		g.AddEdge(Edge{From: fromURI, To: to, Kind: EdgeCites, Via: "openalex:cites", Tier: "openalex", Fetched: at})
		g.AddEdge(Edge{From: to, To: fromURI, Kind: EdgeCitedBy, Via: "openalex:cites", Tier: "openalex", Fetched: at})
	}
}

// AddJournal adds a journal home page, which is where the editorial board and
// the subject list live.
func (g *Graph) AddJournal(j *Journal) string {
	if j == nil {
		return ""
	}
	tier, at := j.Envelope.Tier, j.Envelope.Fetched

	var uris []string
	for _, raw := range []string{j.ElectronicISSN, j.PrintISSN} {
		i, err := ParseISSN(raw)
		if err != nil {
			continue
		}
		uris = append(uris, JournalURI(i))
	}
	var springer string
	if id, err := ParseSpringerID(j.SpringerID); err == nil {
		springer = SpringerJournalURI(id)
	}
	uri := firstNonEmpty(append(uris, springer)...)
	if uri == "" {
		return ""
	}

	props := map[string]string{}
	putIf(props, "electronic_issn", j.ElectronicISSN)
	putIf(props, "print_issn", j.PrintISSN)
	putIf(props, "springer_id", j.SpringerID)
	putIf(props, "url", j.URL)
	putIf(props, "publishing_model", j.PublishingModel)
	for _, u := range append(uris, springer) {
		if u == "" {
			continue
		}
		g.AddNode(Node{URI: u, Kind: NodeJournal, Label: j.Title, Props: props, Via: "region:springer-electronic-issn", Tier: tier, Fetched: at})
		if u != uri {
			g.sameAs(uri, u, "another identifier for the same journal", tier, at)
		}
	}

	g.addPeople(uri, j.Editors, EdgeEditedBy, "region:editorial-board", tier, at)
	g.addSubjects(uri, j.Subjects, "region:subjects", tier, at)
	g.addPublisher(uri, firstNonEmpty(j.Imprint, j.PublisherBrand), "datalayer:imprint", tier, at)
	return uri
}

// AddBook adds a book, its series, its conference and its table of contents.
func (g *Graph) AddBook(b *Book) string {
	if b == nil {
		return ""
	}
	tier, at := b.Envelope.Tier, b.Envelope.Fetched

	// The electronic ISBN is the book's identity on this site and the other
	// three are editions of it, so they become sameAs rather than four books.
	var uris []string
	for _, raw := range []string{b.ISBNElectronic, b.ISBNPrint, b.ISBNHardcover, b.ISBNSoftcover} {
		i, err := ParseISBN(raw)
		if err != nil {
			continue
		}
		uris = append(uris, BookURI(i))
	}
	uri := firstNonEmpty(uris...)
	if uri == "" {
		if d, err := ParseDOI(b.DOI); err == nil {
			uri = BookDOIURI(d)
		}
	}
	if uri == "" {
		return ""
	}

	props := map[string]string{}
	putIf(props, "isbn", b.ISBNElectronic)
	putIf(props, "doi", b.DOI)
	putIf(props, "kind", b.Kind)
	putIf(props, "url", b.URL)
	if b.CopyrightYear > 0 {
		props["copyright_year"] = strconv.Itoa(b.CopyrightYear)
	}
	g.AddNode(Node{URI: uri, Kind: NodeBook, Label: b.Title, Props: props, Via: "highwire:citation_isbn", Tier: tier, Fetched: at})
	for _, u := range uris {
		if u != uri {
			g.AddNode(Node{URI: u, Kind: NodeBook, Label: b.Title, Via: "region:isbn", Tier: tier, Fetched: at})
			g.sameAs(uri, u, "another edition's isbn for the same book", tier, at)
		}
	}

	g.addPeople(uri, b.Authors, EdgeAuthoredBy, "region:authors", tier, at)
	g.addPeople(uri, b.Editors, EdgeEditedBy, "region:editors", tier, at)
	g.addSubjects(uri, b.Subjects, "region:subjects", tier, at)
	g.addPublisher(uri, b.Publisher, "highwire:citation_publisher", tier, at)

	if b.Series != nil {
		s := seriesURIOf(b.Series)
		if s == "" {
			if i, err := ParseISSN(b.SeriesISSN); err == nil {
				s = SeriesURI(i)
			}
		}
		if s != "" {
			g.AddNode(Node{URI: s, Kind: NodeSeries, Label: b.Series.Name, Via: "region:series", Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: uri, To: s, Kind: EdgeInSeries, Via: "region:series", Tier: tier, Fetched: at})
		}
	}
	if b.Conference != nil {
		if u := ConferenceURI(firstNonEmpty(b.Conference.Acronym, b.Conference.Name), b.Conference.Year); u != "" {
			g.AddNode(Node{URI: u, Kind: NodeConference, Label: b.Conference.Name, Via: "region:conference", Tier: tier, Fetched: at})
			g.AddEdge(Edge{From: uri, To: u, Kind: EdgePresentedAt, Via: "region:conference", Tier: tier, Fetched: at})
		}
	}

	// The table of contents is a list of works this book holds, and the front
	// and back matter rows carry no DOI and become nothing.
	for _, c := range b.Chapters {
		d, err := ParseDOI(c.DOI)
		if err != nil {
			continue
		}
		ch := WorkURI(d)
		g.AddNode(Node{URI: ch, Kind: NodeWork, Label: c.Title, Props: prop("doi", string(d)), Via: "region:toc", Tier: tier, Fetched: at})
		g.AddEdge(Edge{From: ch, To: uri, Kind: EdgePartOf, Position: c.Position + 1, Via: "region:toc", Tier: tier, Fetched: at})
	}
	return uri
}

// AddSeries adds a book series and the books its home page shows.
func (g *Graph) AddSeries(s *Series) string {
	if s == nil {
		return ""
	}
	tier, at := s.Envelope.Tier, s.Envelope.Fetched

	var uris []string
	for _, raw := range []string{s.ElectronicISSN, s.PrintISSN} {
		if i, err := ParseISSN(raw); err == nil {
			uris = append(uris, SeriesURI(i))
		}
	}
	uri := firstNonEmpty(uris...)
	if uri == "" {
		return ""
	}
	props := map[string]string{}
	putIf(props, "electronic_issn", s.ElectronicISSN)
	putIf(props, "print_issn", s.PrintISSN)
	putIf(props, "series_id", s.SeriesID)
	putIf(props, "url", s.URL)
	for _, u := range uris {
		g.AddNode(Node{URI: u, Kind: NodeSeries, Label: s.Title, Props: props, Via: "region:series-issn", Tier: tier, Fetched: at})
		if u != uri {
			g.sameAs(uri, u, "the print and electronic issn of one series", tier, at)
		}
	}
	g.addPeople(uri, s.Editors, EdgeEditedBy, "region:series-editors", tier, at)
	return uri
}

// sameAs asserts identifier equivalence, which is the only ground this tool
// ever asserts it on. Both directions are emitted because a consumer walking
// one way should not have to know which of two identifiers was seen first.
func (g *Graph) sameAs(a, b, why, tier string, at time.Time) {
	if a == "" || b == "" || a == b {
		return
	}
	g.AddEdge(Edge{From: a, To: b, Kind: EdgeSameAs, Via: why, Tier: tier, Fetched: at})
	g.AddEdge(Edge{From: b, To: a, Kind: EdgeSameAs, Via: why, Tier: tier, Fetched: at})
}

// personURI is the identity rule in one function.
//
// An ORCID that passes its checksum names a person. Anything else names a
// string that appeared in an author position, and the uri says which of the two
// this is. There is no third case and no confidence score.
func personURI(p Author) (uri, via string) {
	if o, err := ParseORCID(p.ORCID); err == nil {
		return PersonORCIDURI(o), "orcid"
	}
	if strings.TrimSpace(p.Name) == "" {
		return "", ""
	}
	return PersonNameURI(p.Name), "name"
}

func orgURI(a Affiliation) (uri, via string) {
	if r, err := ParseROR(a.ROR); err == nil {
		return OrgRORURI(r), "ror"
	}
	if strings.TrimSpace(a.Name) == "" {
		return "", ""
	}
	return OrgNameURI(a.Name), "name"
}

// seriesURIOf reads a series ref, which carries an ISSN on some pages and a
// name on others.
func seriesURIOf(r *Ref) string {
	if r == nil {
		return ""
	}
	if i, err := ParseISSN(r.ID); err == nil {
		return SeriesURI(i)
	}
	if strings.TrimSpace(r.Name) == "" {
		return ""
	}
	// A series with no ISSN is named and not identified, and the uri says so in
	// the same way a person with no ORCID is.
	return URIPrefix + "series/name/" + nameHash(r.Name)
}

// crossrefAuthors converts the deposit's person shape into the one the shared
// author walker takes, so that the identity rule lives in one place rather than
// once per tier.
func crossrefAuthors(people []CrossrefPerson) []Author {
	out := make([]Author, 0, len(people))
	for i, p := range people {
		a := Author{
			Name:     p.Display(),
			Family:   p.Family,
			Given:    p.Given,
			ORCID:    string(p.ORCID),
			Sequence: p.Sequence,
			// Zero based, which is what every record in this package uses. The
			// edge is one based and the conversion happens there.
			Position: i,
		}
		for _, s := range p.Affiliations {
			a.Affiliations = append(a.Affiliations, Affiliation{Name: s})
		}
		out = append(out, a)
	}
	return out
}

func putIf(m map[string]string, k, v string) {
	if strings.TrimSpace(v) != "" {
		m[k] = v
	}
}

func prop(k, v string) map[string]string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return map[string]string{k: v}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// mustDOI and mustORCID return the normalized value or the empty one. They
// exist so that a caller who has already decided an unparseable value produces
// no node does not need a second error branch to say it again.
func mustDOI(s string) DOI {
	d, err := ParseDOI(s)
	if err != nil {
		return ""
	}
	return d
}

func mustORCID(s string) ORCID {
	o, err := ParseORCID(s)
	if err != nil {
		return ""
	}
	return o
}

func mustROR(r string) ROR {
	v, err := ParseROR(r)
	if err != nil {
		return ""
	}
	return v
}
