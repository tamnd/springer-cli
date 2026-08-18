// Package cli is the cobra command tree for spr.
package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// Version metadata, stamped at build time through -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// The product token names this build, so the linker stamped version has to
// reach the spr package. -ldflags runs before init, so this sees the real one.
func init() { spr.SetVersion(Version) }

// Exit codes. Each one is a different thing for a script to do about it, which
// is the only reason to have more than two.
const (
	// CodeOK is a command that did what it was asked.
	CodeOK = 0

	// CodeUsage is a flag or an argument this tool does not understand. Nothing
	// was fetched.
	CodeUsage = 1

	// CodeChallenged is the search surface answering with a client challenge
	// and no fallback left. It is a rate rather than a refusal, so it is worth
	// trying again later, which is why it is not the same code as a refusal.
	CodeChallenged = 2

	// CodeNoData is a page that was fetched and understood and had nothing in
	// it. That is a page that changed shape, or a record that does not exist.
	CodeNoData = 3

	// CodeRestricted is the publisher stating access=No. The metadata was read
	// and printed; only the body is missing. It is an exit code rather than an
	// error so a script can tell, and it is not an error message because
	// nothing went wrong.
	CodeRestricted = 4

	// CodeTransport is a network failure, a timeout, or a 5xx that outlived the
	// retries.
	CodeTransport = 5

	// CodeRateLimited is an upstream saying, in a header, that the budget is
	// spent. Waiting is the fix and the message says how long.
	CodeRateLimited = 6
)

// ExitError carries a process exit code out of a command.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Silent reports whether this failure has already said everything it has to
// say. A restricted page and a challenge both print their result first and then
// carry an exit code out, and printing "Error:" under output that is correct
// would read as though the fetch had failed.
func (e *ExitError) Silent() bool { return e.Err == nil }

func (e *ExitError) Unwrap() error { return e.Err }

// ExitCode reports the exit code this failure carries.
func (e *ExitError) ExitCode() int { return e.Code }

func exit(code int, err error) error { return &ExitError{Code: code, Err: err} }

// globals are the flags every command shares.
type globals struct {
	pace    time.Duration
	timeout time.Duration
	retries int
	cache   string
	noCache bool
	mailto  string
	debug   bool
	format  string
}

var g globals

// Root builds the command tree.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "spr",
		Short: "A delightful command line for link.springer.com",
		Long: "spr reads what link.springer.com publishes about works, journals, books and series,\n" +
			"and records where each field came from.\n\n" +
			"Every record carries an envelope naming the source that answered for each field, what\n" +
			"was looked for and not found, and what was left unread.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	f := root.PersistentFlags()
	f.DurationVar(&g.pace, "pace", spr.DefaultPace, "interval between requests to one host, never below "+spr.PaceFloor.String())
	f.DurationVar(&g.timeout, "timeout", 30*time.Second, "per request timeout")
	f.IntVar(&g.retries, "retries", 2, "retries on a transport error or a 5xx, never on a challenge")
	f.StringVar(&g.cache, "cache", spr.DefaultCacheDir(), "cache directory")
	f.BoolVar(&g.noCache, "no-cache", false, "fetch every page fresh and store nothing")
	f.StringVar(&g.mailto, "mailto", "", "contact address for the Crossref and OpenAlex polite pools")
	f.BoolVar(&g.debug, "debug", false, "one line per request on stderr")
	f.StringVarP(&g.format, "output", "o", "text", "output format: text or json")

	root.AddCommand(
		getCmd(),
		workCmd(),
		journalCmd(),
		bookCmd(),
		seriesCmd(),
		extractionCmd(),
		versionCmd(),
		cacheCmd(),
	)
	return root
}

// client builds a client from the global flags. A pace under the floor is
// accepted, applied as the floor, and reported, because refusing the run over
// it would be worse and doing it silently would be a lie.
func client(stderr io.Writer) *spr.Client {
	pace := g.pace
	if pace < spr.PaceFloor {
		fmt.Fprintf(stderr, "spr: --pace %s is below the %s floor, using the floor\n", pace, spr.PaceFloor)
		pace = spr.PaceFloor
	}
	dir := g.cache
	if g.noCache {
		dir = ""
	}
	opts := []spr.Option{
		spr.WithPace(pace),
		spr.WithTimeout(g.timeout),
		spr.WithRetries(g.retries),
		spr.WithCache(dir, spr.DefaultTTL),
		spr.WithMailto(g.mailto),
	}
	if g.debug {
		opts = append(opts, spr.WithDebug(stderr))
	}
	return spr.New(opts...)
}
