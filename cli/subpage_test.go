package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/springer-cli/spr"
)

// Nothing in this file goes to the network either. The three subpage commands
// are exercised on their argument handling and their printing, and the
// extractors are covered against the real captures in the spr package.

func TestMetricsTarget(t *testing.T) {
	cases := []struct{ arg, want string }{
		{"10.1007/s10994-021-05946-3", "https://link.springer.com/article/10.1007/s10994-021-05946-3/metrics"},
		{"/article/10.1007/s10994-021-05946-3", "https://link.springer.com/article/10.1007/s10994-021-05946-3/metrics"},
		{"https://link.springer.com/article/10.1007/s10994-021-05946-3", "https://link.springer.com/article/10.1007/s10994-021-05946-3/metrics"},

		// Already a metrics url. Left alone rather than given a second suffix.
		{"/article/10.1007/s10994-021-05946-3/metrics", "/article/10.1007/s10994-021-05946-3/metrics"},
	}
	for _, c := range cases {
		got, err := metricsTarget(c.arg)
		if err != nil {
			t.Errorf("metricsTarget(%q): %v", c.arg, err)
			continue
		}
		if got != c.want {
			t.Errorf("metricsTarget(%q) = %q, want %q", c.arg, got, c.want)
		}
	}

	if _, err := metricsTarget("not an identifier"); err == nil {
		t.Error("a string that is neither a doi nor a url was accepted")
	}
}

// A bare doi goes straight to the article path rather than through the search
// spr work does, because this subpage exists for articles and a chapter doi is
// better told so after one request than after four.
func TestMetricsTargetDoesNotSearchPaths(t *testing.T) {
	got, err := metricsTarget("10.1007/978-3-031-28170-9_6")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/article/") {
		t.Errorf("metricsTarget = %q, want the article path tried first and only", got)
	}
}

// Pasting an address out of a browser should do the obvious thing. Asking for
// figure 4 while looking at figure 1 means the fourth figure of that article.
func TestAssetURLsTrimBackToTheWork(t *testing.T) {
	cases := []struct{ arg, want string }{
		{"10.1007/s10994-021-05946-3", "https://link.springer.com/article/10.1007/s10994-021-05946-3"},
		{"/article/10.1007/s10994-021-05946-3", "https://link.springer.com/article/10.1007/s10994-021-05946-3"},
		{"/article/10.1007/s10994-021-05946-3/figures/1", "https://link.springer.com/article/10.1007/s10994-021-05946-3"},
		{"/article/10.1007/s10994-021-05946-3/tables/12", "https://link.springer.com/article/10.1007/s10994-021-05946-3"},
		{"/article/10.1007/s10994-021-05946-3/metrics", "https://link.springer.com/article/10.1007/s10994-021-05946-3"},
		{"https://link.springer.com/article/10.1007/x/figures/3", "https://link.springer.com/article/10.1007/x"},
	}
	for _, c := range cases {
		got, err := assetWorkURL(c.arg)
		if err != nil {
			t.Errorf("assetWorkURL(%q): %v", c.arg, err)
			continue
		}
		if got != c.want {
			t.Errorf("assetWorkURL(%q) = %q, want %q", c.arg, got, c.want)
		}
	}

	if got := spr.FigureURL("/article/10.1007/x/figures/1", 4); got != "/article/10.1007/x/figures/1/figures/4" {
		t.Log("FigureURL appends without trimming, which is why assetWorkURL runs first")
	}
}

func TestAssetNumber(t *testing.T) {
	if n, err := assetNumber(" 7 "); err != nil || n != 7 {
		t.Errorf("assetNumber(\" 7 \") = %d, %v", n, err)
	}
	// Zero and negatives are refused here rather than sent, because the site
	// answers them with a page.
	for _, bad := range []string{"0", "-1", "one", "1.5", ""} {
		if _, err := assetNumber(bad); err == nil {
			t.Errorf("assetNumber(%q) was accepted", bad)
		}
	}
}

// A rank is printed the way the page prints it, because 22032nd and 22,032nd
// are the same number and only one of them can be read at a glance.
func TestOrdinal(t *testing.T) {
	cases := map[int]string{
		1: "1st", 2: "2nd", 3: "3rd", 4: "4th",
		11: "11th", 12: "12th", 13: "13th",
		21: "21st", 96: "96th", 101: "101st",
		1307: "1,307th", 22032: "22,032nd", 474090: "474,090th",
	}
	for n, want := range cases {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
	if got := group(474090); got != "474,090" {
		t.Errorf("group(474090) = %q", got)
	}
}

// The count and the counter are printed together or the line is not printed.
// Springer says 1,906 for one measured article where Crossref says 1,553, and a
// bare number implies a consensus that does not exist.
func TestPrintMetricsNamesTheCounter(t *testing.T) {
	updated := time.Date(2026, 8, 18, 10, 34, 56, 0, time.UTC)
	m := &spr.Metrics{
		Title:      "Aleatoric and epistemic uncertainty in machine learning",
		DOI:        "10.1007/s10994-021-05946-3",
		ArticleURL: "https://link.springer.com/article/10.1007/s10994-021-05946-3",
		Updated:    &updated,
		Accesses:   &spr.Accesses{Raw: "134k", Value: 134000, Approximate: true},
		Citations:  &spr.Citations{Count: 1906, Source: "Dimensions"},
		Altmetric: &spr.Altmetric{
			Score:      52,
			DetailsURL: "https://link.altmetric.com/details/69076743",
			Breakdown:  []spr.AttentionKey{{Kind: "twitter", Count: 20, Label: "tweeters"}},
			Cohorts: []spr.Cohort{
				{Scope: "all journals", Percentile: 95, Rank: 22032, Size: 474090},
				{Scope: "Machine Learning", Percentile: 96, Rank: 1, Size: 29},
			},
		},
		Mentions: []spr.Mention{{Outlet: "Medium US", Title: "A short note", URL: "https://example.org/a"}},
	}

	var buf bytes.Buffer
	printMetrics(&buf, m, false)
	got := buf.String()

	for _, want := range []string{
		"1,906 per Dimensions",
		"134k",
		"134k, about 134,000, which the page calls an approximate count",
		"22,032nd of 474,090 tracked articles in all journals",
		"1st of 29 tracked articles in Machine Learning",
		"20 tweeters",
		"the named coverage only",
		"Medium US",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the output does not contain %q:\n%s", want, got)
		}
	}
}

// A count with nobody's name on it says so. Dropping the source silently would
// leave the number looking like Springer's own, which is the one reading this
// tool must not allow.
func TestPrintMetricsSaysWhenNobodyIsNamed(t *testing.T) {
	var buf bytes.Buffer
	printMetrics(&buf, &spr.Metrics{Citations: &spr.Citations{Count: 7}}, false)
	got := buf.String()

	if !strings.Contains(got, "7 per an unnamed counter") {
		t.Errorf("an unsourced count printed as:\n%s", got)
	}
	if strings.Contains(got, "accesses") {
		t.Error("an accesses line was printed for a record that has no accesses count")
	}
}

// Both cohorts are printed. Quoting 96th percentile without the 29 behind it is
// how a percentile lies, and the printer is where that is enforced.
func TestPrintMetricsPrintsBothCohorts(t *testing.T) {
	m := &spr.Metrics{Altmetric: &spr.Altmetric{Score: 52, Cohorts: []spr.Cohort{
		{Scope: "all journals", Percentile: 95, Rank: 22032, Size: 474090},
		{Scope: "Machine Learning", Percentile: 96, Rank: 1, Size: 29},
	}}}
	var buf bytes.Buffer
	printMetrics(&buf, m, false)

	if n := strings.Count(buf.String(), "percentile"); n != 2 {
		t.Errorf("printed %d percentiles, want 2", n)
	}
	if !strings.Contains(buf.String(), "of 29 tracked") {
		t.Error("the small cohort's size was not printed, so its percentile stands unqualified")
	}
}

// The figure printer's job is to say what the subpage bought you, which is the
// asset and its real dimensions.
func TestPrintFigure(t *testing.T) {
	f := &spr.FigurePage{
		Label:        "Fig. 1",
		Caption:      "Predictions by EfficientNet on test images.",
		Anchor:       "#Fig1",
		ArticleURL:   "https://link.springer.com/article/10.1007/s10994-021-05946-3",
		ArticleTitle: "Aleatoric and epistemic uncertainty in machine learning",
		Image: &spr.Image{
			URL:    "https://media.springernature.com/full/springer-static/image/art%3A1/MediaObjects/1_Fig1.png",
			WebP:   "https://media.springernature.com/full/springer-static/image/art%3A1/MediaObjects/1_Fig1.png?as=webp",
			Width:  1177,
			Height: 420,
			Alt:    "figure 1",
		},
		Refs: []spr.CaptionRef{{Text: "2019", Citation: "Tan, M. EfficientNet.", URL: "https://example.org/r"}},
	}

	var buf bytes.Buffer
	printFigure(&buf, f, false)
	got := buf.String()

	for _, want := range []string{"Fig. 1", "1177 by 420", "#Fig1", "EfficientNet", "as=webp"} {
		if !strings.Contains(got, want) {
			t.Errorf("the output does not contain %q:\n%s", want, got)
		}
	}
}

// The rows go out tab separated rather than aligned. A cell here holds LaTeX and
// can be far wider than a terminal, so aligning would either wrap the table into
// nonsense or truncate the data.
func TestPrintTableIsTabSeparated(t *testing.T) {
	tb := &spr.TablePage{
		Label:   "Table 1",
		Caption: "Notation used throughout the paper",
		Anchor:  "#Tab1",
		Head:    []string{"Notation", "Meaning"},
		Rows: [][]string{
			{`\(P\)`, "Probability measure, density or mass function"},
			{`\(\mathcal{X}\)`, "Instance space"},
		},
	}

	var buf bytes.Buffer
	printTable(&buf, tb, false)
	got := buf.String()

	if !strings.Contains(got, "Notation\tMeaning") {
		t.Errorf("the header row is not tab separated:\n%s", got)
	}
	if !strings.Contains(got, "2 rows, 2 columns") {
		t.Errorf("the shape was not stated:\n%s", got)
	}
	if !strings.Contains(got, `\(\mathcal{X}\)`) {
		t.Error("the latex was rewritten, and this tool does not have an opinion about notation")
	}
}

// Listing tables from the article page says what that page can and cannot tell
// you, because a caller who reads the list and expects rows will find none.
func TestPrintTableListSaysWhereTheRowsAre(t *testing.T) {
	w := &spr.Work{
		Title:  "Aleatoric and epistemic uncertainty in machine learning",
		Tables: []spr.Table{{Label: "Table 1", Caption: "Notation used throughout the paper", PageURL: "https://link.springer.com/article/10.1007/x/tables/1"}},
	}
	var buf bytes.Buffer
	printTableList(&buf, w)
	got := buf.String()

	if !strings.Contains(got, "tables (1)") {
		t.Errorf("the count was not printed:\n%s", got)
	}
	if !strings.Contains(got, "carries no rows") {
		t.Errorf("the list did not say the rows are elsewhere:\n%s", got)
	}

	var empty bytes.Buffer
	printTableList(&empty, &spr.Work{Title: "A work with no tables"})
	if !strings.Contains(empty.String(), "publishes no tables") {
		t.Errorf("a work with no tables printed as %q", empty.String())
	}
}

func TestPrintFigureListPointsAtTheFullRendition(t *testing.T) {
	w := &spr.Work{
		Title:   "Aleatoric and epistemic uncertainty in machine learning",
		Figures: []spr.Figure{{Label: "Fig. 1", Caption: "Predictions.", PageURL: "https://link.springer.com/article/10.1007/x/figures/1"}},
	}
	var buf bytes.Buffer
	printFigureList(&buf, w)
	got := buf.String()

	if !strings.Contains(got, "figures (1)") || !strings.Contains(got, "Fig. 1") {
		t.Errorf("the list printed as:\n%s", got)
	}
	if !strings.Contains(got, "inline rendition") {
		t.Errorf("the list did not say the images are the small ones:\n%s", got)
	}
}

// An out of range number is answered with a healthy looking page and an empty
// body, so the message has to say that rather than reporting a parse failure.
func TestAssetErrorExplainsTheEmpty200(t *testing.T) {
	err := assetError(spr.ErrNoSuchFigure, "10.1007/x", "figure", 99)
	if err == nil {
		t.Fatal("no error")
	}
	if !strings.Contains(err.Error(), "has no figure 99") || !strings.Contains(err.Error(), "200") {
		t.Errorf("the message reads %q", err)
	}

	tblErr := assetError(spr.ErrNoSuchTable, "10.1007/x", "table", 99)
	if !strings.Contains(tblErr.Error(), "has no table 99") {
		t.Errorf("the message reads %q", tblErr)
	}
}

// Every command this package registers is reachable, which is the one thing a
// missing line in root.go would break silently.
func TestSubpageCommandsAreRegistered(t *testing.T) {
	want := map[string]bool{"metrics": false, "figures": false, "tables": false}
	for _, c := range Root().Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("spr %s is not registered", name)
		}
	}
}
