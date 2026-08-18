package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

func cacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Show or clear the page cache",
		Long: "The cache is keyed on the url that was requested rather than the url the redirect\n" +
			"chain landed on. Every first request to link.springer.com is redirected twice onto\n" +
			"the same path with a per request uuid appended, so keying on where the response came\n" +
			"from would mean no key is ever seen twice.",
		Args: cobra.NoArgs,
	}

	var clear bool
	cmd.Flags().BoolVar(&clear, "clear", false, "delete every cached response")

	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		c := spr.NewCache(g.cache, spr.DefaultTTL)
		if clear {
			if err := c.Clear(); err != nil {
				return exit(CodeTransport, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cleared %s\n", g.cache)
			return nil
		}
		n, bytes, err := c.Stats()
		if err != nil {
			return exit(CodeTransport, err)
		}
		out := cmd.OutOrStdout()
		if g.format == "json" {
			return json.NewEncoder(out).Encode(map[string]any{
				"dir":       g.cache,
				"responses": n,
				"bytes":     bytes,
				"ttl":       spr.DefaultTTL.String(),
			})
		}
		fmt.Fprintf(out, "dir       %s\n", g.cache)
		fmt.Fprintf(out, "responses %d\n", n)
		fmt.Fprintf(out, "size      %s\n", humanBytes(bytes))
		fmt.Fprintf(out, "ttl       %s\n", spr.DefaultTTL)
		return nil
	}
	return cmd
}

// humanBytes formats a byte count the way a person reads one.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
