package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tamnd/springer-cli/spr"
)

// Nothing in this file goes to the network. The three container commands are
// exercised on their argument handling and their printing, which is where the
// decisions are, and the extractors themselves are covered against the real
// captures in the spr package.

func TestContainerTargets(t *testing.T) {
	cases := []struct {
		arg, want string
	}{
		{"10994", "/journal/10994"},
		{"0885-6125", "/journal/0885-6125"},
		{"/journal/10994", "/journal/10994"},
		{"https://link.springer.com/journal/10994", "https://link.springer.com/journal/10994"},
	}
	for _, c := range cases {
		if got := journalTarget(c.arg); got != c.want {
			t.Errorf("journalTarget(%q) = %q, want %q", c.arg, got, c.want)
		}
	}

	if got := seriesTarget("558"); got != "/series/558" {
		t.Errorf("seriesTarget(558) = %q", got)
	}
	if got := seriesTarget("/series/558"); got != "/series/558" {
		t.Errorf("seriesTarget of a path rewrote it: %q", got)
	}
}

// A book is addressable two ways and both are used as given rather than
// converted between, because they are the same page and the site resolves both.
func TestBookTarget(t *testing.T) {
	doi, err := bookTarget("10.1007/978-3-031-28170-9")
	if err != nil {
		t.Fatal(err)
	}
	if doi != "/book/10.1007/978-3-031-28170-9" {
		t.Errorf("the doi form = %q", doi)
	}

	isbn, err := bookTarget("978-3-031-28170-9")
	if err != nil {
		t.Fatal(err)
	}
	if isbn != "/book/9783031281709" {
		t.Errorf("the isbn form = %q, and the site wants it without the hyphens", isbn)
	}

	if _, err := bookTarget("not an identifier"); err == nil {
		t.Error("a string that is neither a doi, an isbn nor a url was accepted")
	}
}

// A journal with unread volumes and a journal with no volumes are different
// facts, and the line that prints the pointer has to say which.
func TestConnSaysWhatIsHeld(t *testing.T) {
	unread := conn(spr.Conn{URL: "https://link.springer.com/journal/10994/volumes-and-issues"})
	if !strings.Contains(unread, "0 held") || !strings.Contains(unread, "more at") {
		t.Errorf("an unread pointer printed as %q", unread)
	}

	whole := conn(spr.Conn{Loaded: 348, TotalCount: 348, Complete: true})
	if !strings.Contains(whole, "348 of 348 held") || strings.Contains(whole, "more at") {
		t.Errorf("a complete collection printed as %q", whole)
	}
}

// The four isbns are printed under four names, because that is the one thing
// about a book page a reader is most likely to get wrong.
func TestPrintBookNamesEveryISBN(t *testing.T) {
	b := &spr.Book{
		Title:          "The Economics of Family Taxation",
		DOI:            "10.1007/978-3-031-28170-9",
		Kind:           "book",
		ISBNElectronic: "978-3-031-28170-9",
		ISBNHardcover:  "978-3-031-28169-3",
		ISBNSoftcover:  "978-3-031-28172-3",
		ISBNPrint:      "978-3-031-28172-3",
		Offers: []spr.Offer{
			{Kind: "ebook", Label: "eBook", Price: &spr.Money{Raw: "EUR 85.59", Amount: 85.59, Currency: "EUR"}},
		},
		Envelope: spr.Envelope{Tier: "html", Status: spr.StatusRestricted},
	}

	var out bytes.Buffer
	printBook(&out, b, false, false)
	s := out.String()
	for _, want := range []string{"isbn ebook", "isbn print", "isbn hard", "isbn soft", "EUR 85.59"} {
		if !strings.Contains(s, want) {
			t.Errorf("the book form does not mention %q:\n%s", want, s)
		}
	}
}

// A record printed twice is printed the same way twice, so that a diff between
// two runs means the page changed rather than a map iterating in a new order.
func TestPrintJournalIsStable(t *testing.T) {
	j := &spr.Journal{
		SpringerID:      "10994",
		Title:           "Machine Learning",
		ElectronicISSN:  "1573-0565",
		PrintISSN:       "0885-6125",
		PublishingModel: "Hybrid Access",
		Metrics: []spr.Metric{
			{Name: "Journal Impact Factor", Raw: "4.9", Value: 4.9, Year: 2025},
			{Name: "Downloads", Raw: "2.4M", Value: 2_400_000, Year: 2025},
		},
		Editors:  []spr.Author{{Name: "Michelangelo Ceci", Role: "Editor-in-Chief"}},
		Volumes:  &spr.Conn{URL: "https://link.springer.com/journal/10994/volumes-and-issues"},
		Envelope: spr.Envelope{Tier: "html", Status: spr.StatusOK, Via: map[string]string{"title": "region:datalayer.Journal Title", "print_issn": "region:[data-test=springer-print-issn] dd"}},
	}

	var first bytes.Buffer
	printJournal(&first, j, true)
	for i := 0; i < 5; i++ {
		var again bytes.Buffer
		printJournal(&again, j, true)
		if again.String() != first.String() {
			t.Fatal("two runs of the same record printed different bytes")
		}
	}

	s := first.String()
	// 2.4M is what the page said and 2400000 is this tool's reading of it, and
	// the human form prints the publisher's number rather than the arithmetic.
	if !strings.Contains(s, "2.4M (2025)") {
		t.Errorf("the printed metric is not the page's own number:\n%s", s)
	}
	if !strings.Contains(s, "Editor-in-Chief") {
		t.Errorf("the editorial role was flattened away:\n%s", s)
	}
	if !strings.Contains(s, "0 held") {
		t.Errorf("the unread volumes pointer does not say it is unread:\n%s", s)
	}
}

// A journal with one issn does not print a trailing separator for the one it
// does not have.
func TestJoinNonEmpty(t *testing.T) {
	if got := joinNonEmpty(", ", "1573-0565", ""); got != "1573-0565" {
		t.Errorf("joinNonEmpty gave %q", got)
	}
	if got := joinNonEmpty(", ", "", "  "); got != "" {
		t.Errorf("joinNonEmpty of nothing gave %q", got)
	}
}

// The extraction table now names the record each row belongs to, because the
// same field name means different things on different pages.
func TestExtractionByRecord(t *testing.T) {
	out, err := run(t, "extraction", "--record", "journal")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "citation_title") {
		t.Errorf("a work row came back from a journal filter:\n%s", out)
	}
	if !strings.Contains(out, "Publishing Model") {
		t.Errorf("the journal rows are missing:\n%s", out)
	}

	if _, err := run(t, "extraction", "--record", "podcast"); err == nil {
		t.Error("a record that does not exist was accepted")
	}
}

// Title is answered from rung 1 on a work and rung 3 on a journal, and asking
// by the bare name shows both rather than picking one and being wrong about the
// other.
func TestExtractionQualifiedName(t *testing.T) {
	both, err := run(t, "extraction", "title")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(both, "highwire") || !strings.Contains(both, "region") {
		t.Errorf("asking for title did not show both rungs:\n%s", both)
	}

	one, err := run(t, "extraction", "journal.title")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(one, "rung    3, region") {
		t.Errorf("the qualified name did not get the long form:\n%s", one)
	}
}
