// Command spr is a delightful command line for link.springer.com.
package main

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/tamnd/springer-cli/cli"
)

func main() {
	err := fang.Execute(
		context.Background(),
		cli.Root(),
		fang.WithVersion(cli.Version),
		fang.WithCommit(cli.Commit),
		fang.WithErrorHandler(handle),
	)
	if err == nil {
		return
	}
	var ee *cli.ExitError
	if errors.As(err, &ee) {
		os.Exit(ee.Code)
	}
	os.Exit(cli.CodeUsage)
}

// handle prints a failure once, and prints nothing for a failure that is only
// an exit code. A page the publisher restricts has already printed everything
// it knows; the 4 that follows it is for a script and not for a reader.
func handle(w io.Writer, styles fang.Styles, err error) {
	var ee *cli.ExitError
	if errors.As(err, &ee) && ee.Silent() {
		return
	}
	// Almost every error this tool reports contains a url, and the default style
	// title cases the text it is given, which turns link.springer.com/content
	// into Link.springer.com/Content. A url that cannot be pasted back in is
	// worse than a plain one.
	styles.ErrorText = styles.ErrorText.UnsetTransform()
	fang.DefaultErrorHandler(w, styles, err)
}
