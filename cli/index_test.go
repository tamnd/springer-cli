package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/tamnd/springer-cli/spr"
)

// Nothing in this file makes a request. The flag checks run before the client
// exists, and the printers take a record built here.

// The bad flag combinations, each of which is a request that would either cost
// a lot and answer nothing useful or ask a host a question it cannot answer.
func TestIndexRefusesBadFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"crossref with nothing at all", []string{"crossref"}, "needs a doi, or --query"},
		{"crossref narrowed by nothing", []string{"crossref", "--rows", "1000", "--sort", "published"}, "needs a doi, or --query"},
		{"crossref references with no work", []string{"crossref", "--references", "--query", "x"}, "it needs a doi"},
		{"crossref given a title instead of a doi", []string{"crossref", "aleatoric uncertainty"}, "is not a doi"},
		{"crossref given a bad issn", []string{"crossref", "--issn", "0885-6126"}, "check digit"},
		{"openalex with nothing at all", []string{"openalex"}, "needs a doi or a work id"},
		{"openalex above the page size", []string{"openalex", "--query", "x", "--rows", "500"}, "above the 200 OpenAlex serves"},
		{"openalex given neither identifier", []string{"openalex", "not-an-id"}, "neither a doi nor an openalex work id"},
		{"cited-by given neither identifier", []string{"cited-by", "machine learning"}, "neither a doi nor an openalex work id"},
		{"api with nothing at all", []string{"api"}, "api needs terms"},
		{"api on an endpoint that does not exist", []string{"api", "x", "--endpoint", "v3"}, "meta/v2"},
		{"search widened twice by one backend", []string{"search", "x", "--also", "crossref", "--also", "crossref"}, "given twice"},
		{"search widened by a host this tool does not read", []string{"search", "x", "--also", "scopus"}, "not one of crossref, openalex"},
		{"facets widened by an index that counts something else", []string{"search", "x", "--facets", "--also", "crossref"}, "nothing to add"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := run(t, c.args...)
			if err == nil {
				t.Fatalf("accepted, and printed:\n%s", out)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error is %q, want it to mention %q", err, c.want)
			}
			var ee *ExitError
			if !errors.As(err, &ee) || ee.Code != CodeUsage {
				t.Errorf("exit code is not %d, and nothing was fetched", CodeUsage)
			}
		})
	}
}

// A 404 from an index is a fact about the identifier rather than a broken run,
// so it is the no data code and not the transport one. A script asking three
// backends about the same DOI has to be able to tell those apart.
func TestIndexExitCodes(t *testing.T) {
	var ee *ExitError

	missing := &spr.NoRecord{Host: spr.CrossrefHost, URL: spr.CrossrefBase + "/10.1007/nope", Code: 404, Says: "Resource not found."}
	if err := indexError(missing); !errors.As(err, &ee) || ee.Code != CodeNoData {
		t.Errorf("a 404 from an index exits %v, want %d", err, CodeNoData)
	}
	if !strings.Contains(missing.Error(), "Resource not found.") {
		t.Errorf("the host's own words are not in the error: %v", missing)
	}
	if err := indexError(errors.New("dial tcp: i/o timeout")); !errors.As(err, &ee) || ee.Code != CodeTransport {
		t.Errorf("a network failure exits %v, want %d", err, CodeTransport)
	}

	if err := crossrefExit(&spr.CrossrefResult{}); !errors.As(err, &ee) || ee.Code != CodeNoData {
		t.Errorf("an empty Crossref page exits %v, want %d", err, CodeNoData)
	}
	// A facet run returns counts and no items, and it found something.
	facets := &spr.CrossrefResult{Facets: []spr.CrossrefFacet{{Name: "type-name"}}}
	if err := crossrefExit(facets); err != nil {
		t.Errorf("a facet listing with no items exits %v, and it answered the question asked", err)
	}
	if err := openAlexExit(&spr.OpenAlexResult{}); !errors.As(err, &ee) || ee.Code != CodeNoData {
		t.Errorf("an empty OpenAlex page exits %v, want %d", err, CodeNoData)
	}
}

// The Springer API is the one surface here that needs a credential, and a run
// without one has to say so before it makes a request rather than after a 401
// that reads the same as a wrong key.
func TestAPIWithoutAKeySaysWhereToPutOne(t *testing.T) {
	t.Setenv(spr.KeyEnv, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out, err := run(t, "api", "--doi", "10.1007/s10994-021-05946-3")
	if err == nil {
		t.Fatalf("a keyless run was accepted, and printed:\n%s", out)
	}
	if !errors.Is(err, spr.ErrNoKey) {
		t.Errorf("error is %q, want the no key error", err)
	}
	if !strings.Contains(err.Error(), spr.KeyEnv) {
		t.Errorf("error is %q, and it does not name the environment variable to set", err)
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeUsage {
		t.Errorf("a missing key exits %v, want %d, because nothing was fetched", err, CodeUsage)
	}
}

// Where the key came from is a question worth being able to ask, and the key
// itself is not. spr version answers the first and never the second.
func TestVersionNamesTheKeySourceAndNotTheKey(t *testing.T) {
	const key = "b1a4c7e2d9f04a6b8c3e5d7f1a2b4c6d"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	t.Setenv(spr.KeyEnv, "")
	out, err := run(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not configured") {
		t.Errorf("version does not say the key is unset:\n%s", out)
	}

	t.Setenv(spr.KeyEnv, key)
	out, err = run(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, spr.KeyEnv) {
		t.Errorf("version does not name where the key came from:\n%s", out)
	}
	if strings.Contains(out, key) {
		t.Errorf("version printed the key itself:\n%s", out)
	}
}

// The three counts are printed under three names on three commands, and no
// command prints a field called citations. This is the assertion the whole
// design rests on, so it is made against printed bytes and not against a
// struct tag.
func TestPrintedCountsNameWhoCounted(t *testing.T) {
	var crossref bytes.Buffer
	printCrossrefWork(&crossref, crossrefFixture(), false)
	for _, want := range []string{
		"crossref_citations             1,553, deposited citations only",
		"crossref_references            122 deposited",
		"crossref_references_with_doi   66 of those resolve to something",
	} {
		if !strings.Contains(crossref.String(), want) {
			t.Errorf("the Crossref record does not say %q:\n%s", want, crossref.String())
		}
	}

	var openalex bytes.Buffer
	printOpenAlexWork(&openalex, openAlexFixture(), false)
	for _, want := range []string{
		"openalex_citations             1,563, as stored on 2026-08-16T07:02:28.622633",
		"openalex_references            111 resolved to works in the index",
		"fwci                           113.99",
		"in the top 1 percent",
	} {
		if !strings.Contains(openalex.String(), want) {
			t.Errorf("the OpenAlex record does not say %q:\n%s", want, openalex.String())
		}
	}

	for name, got := range map[string]string{"crossref": crossref.String(), "openalex": openalex.String()} {
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "citations ") {
				t.Errorf("the %s record printed a bare citations field: %q", name, line)
			}
		}
	}
}

// The stored OpenAlex count is an aggregate and the date next to it is the only
// thing that says how old it is. Printing the number without the date would be
// presenting a figure from two days ago as today's.
func TestStoredCountCarriesItsDate(t *testing.T) {
	w := openAlexFixture()
	w.Counts.UpdatedDate = ""
	var out bytes.Buffer
	printOpenAlexWork(&out, w, false)
	if strings.Contains(out.String(), "as stored on") {
		t.Errorf("a record with no updated date claimed one:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "openalex_citations             1,563") {
		t.Errorf("the count went missing with the date:\n%s", out.String())
	}
}

// A record printed twice prints the same bytes twice, so that a diff between
// two runs means the record changed.
func TestPrintIndexRecordsAreStable(t *testing.T) {
	for name, print := range map[string]func(w *bytes.Buffer){
		"crossref": func(w *bytes.Buffer) { printCrossrefWork(w, crossrefFixture(), true) },
		"openalex": func(w *bytes.Buffer) { printOpenAlexWork(w, openAlexFixture(), true) },
	} {
		var first bytes.Buffer
		print(&first)
		for i := 0; i < 5; i++ {
			var again bytes.Buffer
			print(&again)
			if again.String() != first.String() {
				t.Fatalf("two runs of the same %s record printed different bytes", name)
			}
		}
	}
}

// Crossref deposits a funder with a name and no id on the measured article, and
// an author whose ORCID the publisher typed in rather than the person signing
// in to attach it. Both are worth a word on the line, because both change what
// the row can be joined to.
func TestCrossrefPrintsWhatCannotBeJoined(t *testing.T) {
	var out bytes.Buffer
	printCrossrefWork(&out, crossrefFixture(), false)
	got := out.String()

	if !strings.Contains(got, "Projekt DEAL  (no funder id deposited)") {
		t.Errorf("a funder with no id printed as though it had one:\n%s", got)
	}
	if !strings.Contains(got, "0000-0002-9944-4108 (unauthenticated)") {
		t.Errorf("an unauthenticated orcid printed as a verified one:\n%s", got)
	}
	// And the two ISSNs keep their medium, because the journal has one of each
	// and the numbers alone do not say which is which.
	if !strings.Contains(got, "0885-6125 print") || !strings.Contains(got, "1573-0565 electronic") {
		t.Errorf("the issns lost their medium:\n%s", got)
	}
}

// OpenAlex has ROR ids where Crossref has an empty affiliation array for the
// same two people, which is the reason both backends exist.
func TestOpenAlexPrintsInstitutions(t *testing.T) {
	var openalex, crossref bytes.Buffer
	printOpenAlexWork(&openalex, openAlexFixture(), false)
	printCrossrefWork(&crossref, crossrefFixture(), false)

	if !strings.Contains(openalex.String(), "https://ror.org/058kzsd48") {
		t.Errorf("no ror id in the OpenAlex record:\n%s", openalex.String())
	}
	if strings.Contains(crossref.String(), "ror.org") {
		t.Errorf("a ror id appeared in the Crossref record, which deposits none:\n%s", crossref.String())
	}
	// Both classifications are printed, because OpenAlex publishes both and
	// they disagree.
	for _, want := range []string{"concepts (2)", "topics (1)", "Machine learning", "Adversarial Robustness"} {
		if !strings.Contains(openalex.String(), want) {
			t.Errorf("the record does not say %q:\n%s", want, openalex.String())
		}
	}
}

// The bill has to show what --also costs and what it does not. The backends are
// separate hosts with their own pace buckets, so their requests do not queue
// behind the search surface's five second one.
func TestSearchBillsTheBackends(t *testing.T) {
	out, err := run(t, "search", "uncertainty", "--also", "crossref", "--also", "openalex", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	for _, want := range []string{
		"1 crossref page",
		"1 openalex page",
		"crossref and openalex, on their own hosts and their own pace, merged on doi",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the bill does not say %q:\n%s", want, out)
		}
	}
	// The same query without the flag pays the same wait. Asserting on the two
	// bills against each other rather than on a fixed number is the whole claim:
	// whatever the search surface costs, the backends do not add to it.
	plain, err := run(t, "search", "uncertainty", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if a, b := billLine(out, "estimate"), billLine(plain, "estimate"); a != b {
		t.Errorf("the backends were billed as though they queued behind the search pace: %q with --also and %q without", a, b)
	}
	// And the requests line does move, because two more requests are two more
	// requests even when they are free of the wait.
	if a, b := billLine(out, "requests"), billLine(plain, "requests"); a == b {
		t.Errorf("the backend requests were not billed at all: %q", a)
	}
}

// billLine returns the value of one named line of a dry run bill.
func billLine(out, name string) string {
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), name); ok && strings.HasPrefix(rest, " ") {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// The four new commands are on the tree and each one says on its first line
// which host it reads, because a command that goes somewhere other than
// link.springer.com should say so before it is run.
func TestIndexCommandsAreListed(t *testing.T) {
	out, err := run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"crossref", "openalex", "cited-by", "api"} {
		if !strings.Contains(out, want) {
			t.Errorf("spr --help does not list %q:\n%s", want, out)
		}
	}
	for cmd, host := range map[string]string{
		"crossref": "Crossref",
		"openalex": "OpenAlex",
		"cited-by": "OpenAlex",
		"api":      "Springer Nature API",
	} {
		got, err := run(t, cmd, "--help")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, host) {
			t.Errorf("spr %s --help does not name %s", cmd, host)
		}
	}
}

func crossrefFixture() *spr.CrossrefWork {
	day := func(s string) *spr.Date {
		d, err := spr.ParseDate(s)
		if err != nil {
			panic(err)
		}
		return &d
	}
	return &spr.CrossrefWork{
		DOI:            "10.1007/s10994-021-05946-3",
		URL:            "https://doi.org/10.1007/s10994-021-05946-3",
		Type:           "journal-article",
		Title:          "Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods",
		ContainerTitle: "Machine Learning",
		Publisher:      "Springer Science and Business Media LLC",
		Volume:         "110",
		Issue:          "3",
		Pages:          "457-506",
		ISSNs: []spr.TypedISSN{
			{Value: "0885-6125", Type: "print"},
			{Value: "1573-0565", Type: "electronic"},
		},
		Abstract:        "The notion of uncertainty is of major importance in machine learning.",
		Issued:          day("2021-03"),
		PublishedOnline: day("2021-03-08"),
		Authors: []spr.CrossrefPerson{
			{Given: "Eyke", Family: "Hüllermeier", ORCID: "0000-0002-9944-4108", Sequence: "first"},
			{Given: "Willem", Family: "Waegeman", Sequence: "additional"},
		},
		Funders:  []spr.CrossrefFunder{{Name: "Projekt DEAL"}},
		Licenses: []spr.CrossrefLicense{{URL: "https://creativecommons.org/licenses/by/4.0", Version: "tdm"}},
		Links:    []spr.CrossrefLink{{URL: "https://link.springer.com/content/pdf/10.1007/s10994-021-05946-3.pdf", ContentType: "application/pdf", Application: "text-mining"}},
		Counts:   spr.CrossrefCounts{Citations: 1553, References: 122, ReferencesWithDOI: 66},
		Envelope: spr.Envelope{Tier: "crossref", Status: spr.StatusOK},
	}
}

func openAlexFixture() *spr.OpenAlexWork {
	return &spr.OpenAlexWork{
		ID:              "W3014596384",
		DOI:             "10.1007/s10994-021-05946-3",
		Type:            "article",
		Title:           "Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods",
		PublicationDate: "2021-03-08",
		PublicationYear: 2021,
		Volume:          "110",
		Issue:           "3",
		Pages:           "457-506",
		Source: &spr.OpenAlexSource{
			ID:          "S62148650",
			DisplayName: "Machine Learning",
			ISSNL:       "0885-6125",
			ISSNs:       []spr.ISSN{"0885-6125", "1573-0565"},
			Publisher:   "Springer Science+Business Media",
		},
		Abstract: "The notion of uncertainty is of major importance in machine learning.",
		Authors: []spr.OpenAlexAuthor{
			{
				Name:     "Eyke Hüllermeier",
				ORCID:    "0000-0002-9944-4108",
				Position: "first",
				Institutions: []spr.OpenAlexInstitution{
					{DisplayName: "Paderborn University", ROR: "https://ror.org/058kzsd48", CountryCode: "DE"},
				},
			},
			{Name: "Willem Waegeman", Position: "last"},
		},
		Concepts: []spr.OpenAlexTag{
			{Name: "Machine learning", Score: 0.7033, Level: 1},
			{Name: "Computer science", Score: 0.6112},
		},
		Topics:     []spr.OpenAlexTag{{Name: "Adversarial Robustness", Score: 0.4909}},
		OpenAccess: &spr.OpenAlexAccess{IsOA: true, Status: "hybrid"},
		Counts: spr.OpenAlexCounts{
			Citations:       1563,
			References:      111,
			FWCI:            113.99,
			Percentile:      0.99970283,
			InTopOnePercent: true,
			InTopTenPercent: true,
			ByYear:          []spr.YearCount{{Year: 2026, Count: 195}, {Year: 2025, Count: 431}},
			UpdatedDate:     "2026-08-16T07:02:28.622633",
		},
		Envelope: spr.Envelope{Tier: "openalex", Status: spr.StatusOK},
	}
}
