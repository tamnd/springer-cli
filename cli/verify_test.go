package cli

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/springer-cli/spr"
)

// Nothing in this file goes to the network, and that is not a convenience. The
// point of verify is that the cache path never falls through to a request, so a
// test of it that quietly made one would be testing the opposite of the claim.
// Every case here either seeds a cache directory of its own or asserts on the
// message that only the no fetch path produces.

// seedCache writes one stored capture into a fresh cache directory and returns
// the directory. The bytes are the same gzipped pages the ledger was measured
// from, which is what makes a verify run over them come out ok.
func seedCache(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	cache := spr.NewCache(dir, time.Hour)
	for _, name := range names {
		c, ok := spr.CaptureNamed(name)
		if !ok {
			t.Fatalf("no capture is named %s", name)
		}
		if err := cache.Put(&spr.Response{
			URL:     c.URL,
			Final:   c.URL,
			Code:    200,
			Body:    captureBody(t, c),
			Fetched: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// captureBody reads one page out of the spr package's testdata. Reaching across
// the two directories is deliberate: these are the only bytes in the repository
// that the ledger has anything to say about.
func captureBody(t *testing.T, c spr.Capture) []byte {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "spr", "testdata", "captures", c.File+".gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()

	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// The pages in the cache are the pages the ledger was measured from, so a run
// over them agrees with it, and the run says which of the two sources it read
// before it says anything else.
func TestVerifyReadsTheCacheAndSaysSo(t *testing.T) {
	dir := seedCache(t, "article_oa", "journal")

	out, err := run(t, "verify", "--cache", dir, "--capture", "article_oa", "--capture", "journal")
	if err != nil {
		t.Fatalf("%v, and printed:\n%s", err, out)
	}
	for _, want := range []string{"the page cache at " + dir, "ok", "article_oa.html", "journal.html", "2 ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "refetch") {
		t.Errorf("a cached run called itself a refetch:\n%s", out)
	}
}

// A page that is not in the cache is reported as unread and is not fetched to
// make the report look complete. The message is the proof: a run that fell
// through to a request would fail with a transport error or succeed, and
// neither of those says not in the cache.
func TestVerifyDoesNotFetchWhatTheCacheDoesNotHave(t *testing.T) {
	out, err := run(t, "verify", "--cache", t.TempDir(), "--capture", "article_oa")
	if err == nil {
		t.Fatalf("an empty cache verified anyway:\n%s", out)
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeNoData {
		t.Errorf("exit code is %v, want %d", err, CodeNoData)
	}
	if !strings.Contains(out, "not in the cache") {
		t.Errorf("the report does not say the page was missing:\n%s", out)
	}
	if !strings.Contains(out, "1 unread") {
		t.Errorf("the summary does not count the page as unread:\n%s", out)
	}
}

// Every finding repeats where it was read, because a finding without that is
// the one that costs an afternoon.
func TestEveryFindingNamesTheSourceItWasReadFrom(t *testing.T) {
	out, err := run(t, "verify", "--cache", t.TempDir(), "--capture", "book")
	if err == nil {
		t.Fatal("an empty cache verified anyway")
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.HasPrefix(line, "            ") {
			continue
		}
		if !strings.Contains(line, "read from the page cache at") {
			t.Errorf("a finding does not say where it was read: %q", line)
		}
	}
}

// --no-cache turns off the only thing a default run reads, so it is refused
// rather than answered with fourteen pages unread.
func TestVerifyRefusesToReadACacheThatIsTurnedOff(t *testing.T) {
	out, err := run(t, "verify", "--no-cache")
	if err == nil {
		t.Fatalf("accepted, and printed:\n%s", out)
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeUsage {
		t.Errorf("exit code is %v, want %d", err, CodeUsage)
	}
	if !strings.Contains(err.Error(), "--live") {
		t.Errorf("the refusal does not name the flag that fixes it: %v", err)
	}
}

// A name that is not in the table is a usage error, and the message lists the
// names rather than leaving somebody to find the table in the source.
func TestVerifyNamesTheCapturesItKnows(t *testing.T) {
	out, err := run(t, "verify", "--capture", "articles")
	if err == nil {
		t.Fatalf("accepted, and printed:\n%s", out)
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeUsage {
		t.Errorf("exit code is %v, want %d", err, CodeUsage)
	}
	for _, want := range []string{"article_oa", "referenceworkentry", "search"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message does not list %q: %v", want, err)
		}
	}
}

// Naming a capture works with or without the extension it is stored under.
func TestVerifyTakesACaptureNameEitherWay(t *testing.T) {
	dir := seedCache(t, "series")
	for _, name := range []string{"series", "series.html"} {
		out, err := run(t, "verify", "--cache", dir, "--capture", name)
		if err != nil {
			t.Fatalf("%s: %v, and printed:\n%s", name, err, out)
		}
		if !strings.Contains(out, "1 ok") {
			t.Errorf("%s did not verify:\n%s", name, out)
		}
	}
}

// The json form carries the source on every result rather than once at the top,
// for the same reason the text form repeats it on every line.
func TestVerifyJSONCarriesTheSourceOnEveryResult(t *testing.T) {
	dir := seedCache(t, "article_oa", "book")

	out, err := run(t, "verify", "-o", "json", "--cache", dir, "--capture", "article_oa", "--capture", "book")
	if err != nil {
		t.Fatalf("%v, and printed:\n%s", err, out)
	}
	var got []struct {
		Capture string `json:"capture"`
		URL     string `json:"url"`
		Source  string `json:"source"`
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v, and printed:\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("%d results, want 2", len(got))
	}
	for _, r := range got {
		if r.Verdict != "ok" {
			t.Errorf("%s graded %q, want ok", r.Capture, r.Verdict)
		}
		if !strings.Contains(r.Source, dir) {
			t.Errorf("%s does not name the cache it was read from: %q", r.Capture, r.Source)
		}
		if r.URL == "" {
			t.Errorf("%s carries no url", r.Capture)
		}
	}
}

// The vocabularies agree on every stored page, which is the finding, so the
// report prints the agreements. A page with one vocabulary is not a
// disagreement and is not reported as one.
func TestVocabPrintsTheAgreementsItFound(t *testing.T) {
	dir := seedCache(t, "article_oa", "search")

	out, err := run(t, "verify", "--vocab", "--cache", dir, "--capture", "article_oa", "--capture", "search")
	if err != nil {
		t.Fatalf("%v, and printed:\n%s", err, out)
	}
	for _, want := range []string{
		"agree",
		"access",
		"highwire:access",
		"jsonld:isAccessibleForFree",
		"no fact on this page is stated by more than one vocabulary",
		"and every one of them agrees",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "DISAGREE") {
		t.Errorf("a stored page disagreed with itself:\n%s", out)
	}
}

// Two runs over one page print the same line, because the claims come out of a
// map and a report that reorders itself between runs cannot be diffed.
func TestVocabPrintsTheSameLineTwice(t *testing.T) {
	dir := seedCache(t, "article_oa")

	first, err := run(t, "verify", "--vocab", "--cache", dir, "--capture", "article_oa")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		again, err := run(t, "verify", "--vocab", "--cache", dir, "--capture", "article_oa")
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("run %d differs from the first:\n%s\n%s", i+2, first, again)
		}
	}
}

// Help says which source a default run reads, because the whole command is an
// argument about telling the two apart.
func TestVerifyHelpSaysWhichSourceItReads(t *testing.T) {
	out, err := run(t, "verify", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"page cache by default", "--live", "--vocab", "--capture"} {
		if !strings.Contains(out, want) {
			t.Errorf("help does not mention %q:\n%s", want, out)
		}
	}
}
