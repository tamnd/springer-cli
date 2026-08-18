package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Reading identifiers from stdin is what makes the rest of this tool worth
// piping into.
//
// spr sitemap writes urls to stdout one per line and everything else to stderr
// for exactly one reason, and spr crossref --references splits the identifiers
// and the count across the two streams for the same one. Both of those are only
// worth doing if something on the other end of the pipe reads a line at a time.

// Targets is how many identifiers a run may take before it is billed, which is
// the same twenty a graph walk gets. Twenty paced requests is forty seconds and
// nobody needs warning about forty seconds. Two thousand is an hour and ten
// minutes, and somebody who meant to pipe in ten lines should hear about it
// before the hour starts rather than after it.
const Targets = 20

// targets resolves what a run was asked to read: the arguments it was given,
// or, when it was given none, one identifier per line from stdin.
//
// A keyboard is not an empty pipe. No arguments with stdin attached to a
// terminal is somebody who forgot the argument, and answering that by waiting
// silently until they work out that ctrl-D exists is the worst thing a command
// line tool can do with a keyboard. The character device test says terminal for
// /dev/null too, which is why the message says not a pipe rather than naming a
// terminal it cannot actually prove is there.
func targets(cmd *cobra.Command, args []string, what string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && terminal(f) {
		return nil, fmt.Errorf("no %s given, and stdin is not a pipe, so there is nothing to read them from", what)
	}

	var found []string
	s := bufio.NewScanner(in)
	// A url in a sitemap is long and a scanner's default 64 KB line is not,
	// but neither is any identifier this tool reads, so the limit is generous
	// rather than unlimited.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		// Blank lines are what a file ends with and comments are what a hand
		// maintained list of dois collects, and neither is an identifier.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		found = append(found, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no %s given, and nothing arrived on stdin", what)
	}
	return found, nil
}

// terminal reports whether this file is a character device rather than a pipe
// or a regular file. That is the portable way to ask without a build tag per
// platform, and it is close enough: the two things it says yes to are a
// terminal and a null device, and neither has identifiers on it.
func terminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// each runs fn over every target and decides what the run's exit code is.
//
// One target behaves exactly as it did before any of this existed: whatever fn
// returns is what the command returns, error text and exit code alike. That is
// the case every script already written against this tool depends on.
//
// Many targets is a different question, because then the exit code is about the
// run rather than about any one record. One paywalled work in five hundred is
// not a restricted run. So a real failure decides the code, a status carried
// out of a record that printed perfectly well is counted and named, and a
// status that every single target shares becomes the run's status, since at
// that point it is a fact about all of them.
//
// A failure part way through does not stop the rest. A five hundred identifier
// run that dies on the third has thrown away twenty minutes of pacing to tell
// you something it could have told you at the end.
func each(cmd *cobra.Command, list []string, fn func(target string) error) error {
	if len(list) == 1 {
		return fn(list[0])
	}

	errw := cmd.ErrOrStderr()
	out := cmd.OutOrStdout()

	var (
		failed    error
		nFailed   int
		nRead     int
		statuses  = map[int]int{}
		firstStat = 0
	)
	for i, target := range list {
		if i > 0 && g.format != "json" {
			fmt.Fprintln(out)
		}
		err := fn(target)
		switch {
		case err == nil:
			nRead++
		case silent(err):
			// The record printed. This is the publisher stating something
			// about it, not the run going wrong.
			nRead++
			code := codeOf(err)
			if len(statuses) == 0 {
				firstStat = code
			}
			statuses[code]++
		default:
			nFailed++
			if failed == nil {
				failed = err
			}
			fmt.Fprintf(errw, "spr: %s: %v\n", target, err)
		}
	}

	summarise(errw, len(list), nRead, nFailed, statuses)

	if failed != nil {
		return failed
	}
	if len(statuses) == 1 && statuses[firstStat] == len(list) {
		return exit(firstStat, nil)
	}
	return nil
}

// summarise says how a run of many went, on stderr, where it cannot be mistaken
// for one of the records.
func summarise(errw io.Writer, total, read, failed int, statuses map[int]int) {
	parts := []string{fmt.Sprintf("%d of %d read", read, total)}
	if n := statuses[CodeRestricted]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d restricted", n))
	}
	if n := statuses[CodeChallenged]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d challenged", n))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	fmt.Fprintf(errw, "spr: %s\n", strings.Join(parts, ", "))
}

// silent reports whether this error is a status a command carried out of a
// record it printed, rather than something that went wrong.
func silent(err error) bool {
	var ee *ExitError
	return errors.As(err, &ee) && ee.Silent()
}

// codeOf returns the exit code an error carries, or CodeTransport for one that
// carries none.
func codeOf(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return CodeTransport
}

// bill stops a run that is about to cost more than it looks like it will.
//
// Twenty identifiers off a pipe is forty seconds and needs no ceremony. Two
// thousand is an hour and ten minutes, and the moment to say so is before the
// first request rather than after the four hundredth.
//
// what is the plural noun, spelled out by the caller rather than derived. There
// are only nine callers and one of them is series, so a rule would be more code
// than the nine words it replaces.
func bill(cmd *cobra.Command, list []string, yes bool, what string) error {
	if yes || len(list) <= Targets {
		return nil
	}
	errw := cmd.ErrOrStderr()
	fmt.Fprintf(errw, "spr: %d %s at %s pace is %s\n",
		len(list), what, effectivePace(),
		estimate(time.Duration(len(list))*effectivePace()))
	fmt.Fprintln(errw, "     pass --yes to read them, or pipe in fewer")
	return exit(CodeUsage, nil)
}
