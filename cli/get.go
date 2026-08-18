package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

func getCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <url or path>",
		Short: "Fetch one url and report how it was classified",
		Long: "get is the client with nothing on top of it. It follows the redirect chain, counts the\n" +
			"hops, classifies the response on its content, and prints what it found without parsing\n" +
			"a single field.\n\n" +
			"It is here because a 200 from this site means very little on its own. A client challenge,\n" +
			"an html preview served from a pdf url and a paywalled article all answer 200, and a\n" +
			"missing article answers 404 with 122 KB of body. Use this when a later command says\n" +
			"something surprising and the question is whether the page or the parser changed.",
		Args: cobra.ExactArgs(1),
		Example: "  spr get /article/10.1007/s10994-021-05946-3\n" +
			"  spr get --body /journal/10994 > journal.html\n" +
			"  spr get -o json '/search?query=uncertainty&page=10'",
	}

	var (
		body bool
		kind string
	)
	cmd.Flags().BoolVar(&body, "body", false, "write the response body to stdout instead of the summary")
	cmd.Flags().StringVar(&kind, "kind", "any", "kind expected: any, html, pdf or xml")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		want, err := parseKind(kind)
		if err != nil {
			return exit(CodeUsage, err)
		}
		c := client(cmd.ErrOrStderr())
		resp, err := c.Get(cmd.Context(), args[0], want)
		if err != nil {
			return exit(CodeTransport, err)
		}

		out := cmd.OutOrStdout()
		if body {
			if _, err := out.Write(resp.Body); err != nil {
				return exit(CodeTransport, err)
			}
			return statusExit(resp.Status)
		}

		if g.format == "json" {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(map[string]any{
				"url":          resp.URL,
				"final":        resp.Final,
				"code":         resp.Code,
				"status":       string(resp.Status),
				"redirects":    resp.Redirects,
				"bytes":        resp.Bytes(),
				"content_type": resp.Header.Get("Content-Type"),
				"from_cache":   resp.FromCache,
				"fetched":      resp.Fetched,
			}); err != nil {
				return exit(CodeTransport, err)
			}
			return statusExit(resp.Status)
		}

		fmt.Fprintf(out, "url          %s\n", resp.URL)
		if resp.Final != resp.URL {
			fmt.Fprintf(out, "final        %s\n", resp.Final)
		}
		fmt.Fprintf(out, "code         %d\n", resp.Code)
		fmt.Fprintf(out, "status       %s\n", resp.Status)
		fmt.Fprintf(out, "redirects    %d\n", resp.Redirects)
		fmt.Fprintf(out, "bytes        %d\n", resp.Bytes())
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			fmt.Fprintf(out, "content type %s\n", ct)
		}
		if resp.FromCache {
			fmt.Fprintf(out, "source       cache, fetched %s\n", resp.Fetched.Format("2006-01-02 15:04:05 MST"))
		}
		fmt.Fprintf(out, "\n%s\n", explain(resp.Status))
		return statusExit(resp.Status)
	}
	return cmd
}

func parseKind(s string) (spr.Kind, error) {
	switch s {
	case "any", "":
		return spr.KindAny, nil
	case "html":
		return spr.KindHTML, nil
	case "pdf":
		return spr.KindPDF, nil
	case "xml":
		return spr.KindXML, nil
	}
	return spr.KindAny, fmt.Errorf("--kind %q is not one of any, html, pdf or xml", s)
}

// explain says what a status means in the terms someone reading it cares about,
// because the one word on its own reads like an internal detail.
func explain(s spr.Status) string {
	switch s {
	case spr.StatusOK:
		return "The page came back whole."
	case spr.StatusRestricted:
		return "The publisher states access=No. The metadata is there and the body is not."
	case spr.StatusChallenged:
		return "The edge answered with a client challenge. It is triggered by volume rather than by the\n" +
			"query, it does not clear by waiting, and it applies to the search surface only. Read\n" +
			"/search.rss instead, or come back later."
	case spr.StatusWrongKind:
		return "The url answered with something other than what was asked for. A pdf url doing this is\n" +
			"the usual case and means the pdf is behind a subscription."
	case spr.StatusNotFound:
		return "There is no such page. The body may still be large, because this site serves a full\n" +
			"error page rather than an empty one."
	}
	return ""
}

// statusExit turns a classification into the process exit code for it. The
// output has already been written by this point, because a restricted page or a
// challenge is a result to print and then report, not a reason to print nothing.
func statusExit(s spr.Status) error {
	switch s {
	case spr.StatusRestricted:
		return exit(CodeRestricted, nil)
	case spr.StatusChallenged:
		return exit(CodeChallenged, nil)
	case spr.StatusNotFound, spr.StatusWrongKind:
		return exit(CodeNoData, nil)
	}
	return nil
}
