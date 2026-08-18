package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Nothing in this file goes to the network. Every case either exercises the
// three helpers directly or stops on a usage error, and the two that go through
// the real command tree are both refusals that happen before a client is built.

// runIn is run with something on stdin. The pipe is a reader rather than the
// process's own stdin, because a test that reads the terminal it was started
// from is a test that hangs on somebody's laptop and passes in CI.
func runIn(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := Root()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// pipe is a command with something on stdin and its output collected, for the
// helpers that take a command rather than a command line.
func pipe(stdin string) (*cobra.Command, *bytes.Buffer) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	return cmd, &out
}

// Arguments win. A run that was given identifiers does not read the pipe, so a
// command in the middle of a pipeline that also names a work reads that work
// and does not silently take on whatever the previous stage wrote.
func TestArgumentsWinOverStdin(t *testing.T) {
	cmd, _ := pipe("from-the-pipe\n")

	got, err := targets(cmd, []string{"typed"}, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "typed" {
		t.Errorf("read %v, want the typed argument", got)
	}
}

// One identifier per line, with the things a real file collects thrown out:
// trailing whitespace, blank lines and the comments somebody leaves in a hand
// maintained list.
func TestOneIdentifierPerLine(t *testing.T) {
	cmd, _ := pipe("10.1007/a\n\n  10.1007/b  \n# the third one is broken\n10.1007/c\n")

	got, err := targets(cmd, nil, "work")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.1007/a", "10.1007/b", "10.1007/c"}
	if len(got) != len(want) {
		t.Fatalf("read %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i+1, got[i], want[i])
		}
	}
}

// A pipe that carried nothing is a run with nothing to do, and it says so
// rather than exiting quietly as though it had read everything it was asked to.
func TestNothingOnStdinIsAnError(t *testing.T) {
	cmd, _ := pipe("\n\n# only comments\n")

	if _, err := targets(cmd, nil, "journal"); err == nil {
		t.Fatal("an empty pipe was accepted")
	} else if !strings.Contains(err.Error(), "nothing arrived on stdin") {
		t.Errorf("the message does not say the pipe was empty: %v", err)
	}
}

// A terminal is not an empty pipe. No arguments with a keyboard attached is
// somebody who forgot the argument, and waiting silently for them to work out
// that ctrl-D exists is the worst answer available.
func TestATerminalIsNotAnEmptyPipe(t *testing.T) {
	dev, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("no character device to test with: %v", err)
	}
	defer func() { _ = dev.Close() }()
	if !terminal(dev) {
		t.Skip("this platform's null device is not a character device")
	}

	cmd := &cobra.Command{}
	cmd.SetIn(dev)

	if _, err := targets(cmd, nil, "work"); err == nil {
		t.Fatal("a terminal was read as an empty pipe")
	} else if !strings.Contains(err.Error(), "not a pipe") {
		t.Errorf("the message does not say what stdin is: %v", err)
	}
}

// One target behaves exactly as it did before any of this existed. The error is
// returned as it was given, not wrapped and not summarised, because every
// script already written against this tool depends on that.
func TestOneTargetIsReturnedVerbatim(t *testing.T) {
	cmd, out := pipe("")
	want := errors.New("the page did not answer")

	got := each(cmd, []string{"only"}, func(string) error { return want })
	if !errors.Is(got, want) {
		t.Errorf("returned %v, want the error the record produced", got)
	}
	if out.Len() != 0 {
		t.Errorf("a single target printed a summary:\n%s", out)
	}
}

// A status carried out of a record that printed perfectly well is not a failed
// run. It becomes the run's status only when every target shares it, because at
// that point it is a fact about all of them.
func TestOneStatusForEveryTargetBecomesTheRuns(t *testing.T) {
	cmd, out := pipe("")

	err := each(cmd, []string{"a", "b", "c"}, func(string) error {
		return exit(CodeRestricted, nil)
	})
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeRestricted {
		t.Fatalf("exit code is %v, want %d", err, CodeRestricted)
	}
	if !strings.Contains(out.String(), "3 of 3 read, 3 restricted") {
		t.Errorf("the summary does not count the run:\n%s", out)
	}
}

// One paywalled work in three is not a restricted run. The count is printed and
// the exit code stays zero, because the run did what it was asked.
func TestAMixOfStatusesIsNotTheRunsStatus(t *testing.T) {
	cmd, out := pipe("")

	statuses := []error{exit(CodeRestricted, nil), nil, exit(CodeChallenged, nil)}
	i := 0
	err := each(cmd, []string{"a", "b", "c"}, func(string) error {
		s := statuses[i]
		i++
		return s
	})
	if err != nil {
		t.Errorf("returned %v, want a run that succeeded", err)
	}
	if !strings.Contains(out.String(), "3 of 3 read, 1 restricted, 1 challenged") {
		t.Errorf("the summary does not name both statuses:\n%s", out)
	}
}

// A failure part way through does not stop the rest. A five hundred identifier
// run that dies on the third has thrown away twenty minutes of pacing to say
// something it could have said at the end.
func TestAFailureDoesNotStopTheRest(t *testing.T) {
	cmd, out := pipe("")

	var seen []string
	err := each(cmd, []string{"a", "b", "c"}, func(target string) error {
		seen = append(seen, target)
		if target == "b" {
			return errors.New("boom")
		}
		return nil
	})
	if len(seen) != 3 {
		t.Errorf("read %v, want all three", seen)
	}
	if err == nil || err.Error() != "boom" {
		t.Errorf("returned %v, want the first real failure", err)
	}
	for _, want := range []string{"spr: b: boom", "2 of 3 read, 1 failed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report does not mention %q:\n%s", want, out)
		}
	}
}

// Records are separated in the text form and not in the json form, where a
// blank line between two documents is something a reader has to be taught to
// skip.
func TestRecordsAreSeparatedInTextAndNotInJSON(t *testing.T) {
	was := g.format
	defer func() { g.format = was }()

	for _, tc := range []struct {
		format string
		want   int
	}{{"text", 1}, {"json", 0}} {
		g.format = tc.format
		cmd, out := pipe("")
		err := each(cmd, []string{"a", "b"}, func(target string) error {
			_, err := cmd.OutOrStdout().Write([]byte(target + "\n"))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(out.String(), "\n\n"); got != tc.want {
			t.Errorf("%s printed %d separators, want %d:\n%q", tc.format, got, tc.want, out)
		}
	}
}

// A run larger than the bill is stopped before the first request rather than
// after the four hundredth, and the message says both what it would cost and
// how to ask for it anyway.
func TestALongRunIsBilledFirst(t *testing.T) {
	list := make([]string, Targets+1)
	for i := range list {
		list[i] = "10.1007/s10994-021-05946-3"
	}

	out, err := run(t, append([]string{"work"}, list...)...)
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeUsage {
		t.Fatalf("exit code is %v, want %d", err, CodeUsage)
	}
	for _, want := range []string{"21 works", "--yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the bill does not mention %q:\n%s", want, out)
		}
	}
}

// A run at or under the bill is never billed, so the twenty line pipe that is
// the common case says nothing at all.
func TestAShortRunIsNotBilled(t *testing.T) {
	cmd, out := pipe("")
	list := make([]string, Targets)

	if err := bill(cmd, list, false, "works"); err != nil {
		t.Fatalf("a run of %d was billed: %v", Targets, err)
	}
	if out.Len() != 0 {
		t.Errorf("a short run printed a bill:\n%s", out)
	}
}

// --body writes one body to stdout, so two urls is refused rather than answered
// with two html documents written back to back into a file nothing can read.
func TestBodyTakesOneURL(t *testing.T) {
	out, err := runIn(t, "", "get", "--body", "/journal/10994", "/journal/41586")
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != CodeUsage {
		t.Fatalf("exit code is %v, and printed:\n%s", err, out)
	}
	if !strings.Contains(err.Error(), "--body") {
		t.Errorf("the refusal does not name the flag: %v", err)
	}
}

// crossref and openalex have two modes and one empty argument list, so a run
// with neither an identifier nor anything to search with answers with the whole
// list of what it accepts rather than with a sentence about stdin.
func TestTheTwoModeCommandsSayWhatTheyAccept(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want string
	}{
		{"crossref", "--query, --title, --author or a filter"},
		{"openalex", "--query, --title, --author, --issn, --cites or --cited-by"},
	} {
		out, err := runIn(t, "", tc.cmd)
		var ee *ExitError
		if !errors.As(err, &ee) || ee.Code != CodeUsage {
			t.Fatalf("%s: exit code is %v, and printed:\n%s", tc.cmd, err, out)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: the message does not list what it accepts: %v", tc.cmd, err)
		}
	}
}

// Every command the spec says is variadic is variadic, and carries the flag
// that lifts the bill. This is a structural test on purpose: the failure it
// catches is a tenth command being added next to these nine and quietly taking
// one argument.
func TestEveryVariadicCommandTakesAList(t *testing.T) {
	want := map[string]bool{
		"get": true, "work": true, "journal": true, "book": true, "series": true,
		"metrics": true, "crossref": true, "openalex": true, "cited-by": true,
	}

	for _, c := range Root().Commands() {
		name := c.Name()
		if !want[name] {
			continue
		}
		delete(want, name)
		if err := c.Args(c, []string{"one", "two", "three"}); err != nil {
			t.Errorf("%s refuses three arguments: %v", name, err)
		}
		if c.Flags().Lookup("yes") == nil {
			t.Errorf("%s has no --yes, so a long run cannot be asked for", name)
		}
		if !strings.Contains(c.Long, "stdin") {
			t.Errorf("%s does not say it reads stdin", name)
		}
	}
	for name := range want {
		t.Errorf("%s is not in the command tree", name)
	}
}
