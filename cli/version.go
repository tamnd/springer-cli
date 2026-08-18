package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, the build, and the settings that are not configurable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()

			// Where the API key came from, and never the key itself. It is the
			// one setting that is invisible until something 401s, so it is
			// worth being able to ask without echoing a credential.
			key := spr.KeySource()
			if key == "" {
				key = "not configured, which only affects spr api"
			}

			if g.format == "json" {
				return json.NewEncoder(out).Encode(map[string]any{
					"version":     Version,
					"commit":      Commit,
					"date":        Date,
					"go":          runtime.Version(),
					"platform":    runtime.GOOS + "/" + runtime.GOARCH,
					"user_agent":  spr.UserAgent(),
					"pace_floor":  spr.PaceFloor.String(),
					"pace_search": spr.SearchPace.String(),
					"api_key":     key,
				})
			}
			fmt.Fprintf(out, "spr %s (%s, %s)\n", Version, Commit, Date)
			fmt.Fprintf(out, "%s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
			fmt.Fprintf(out, "user agent  %s\n", spr.UserAgent())
			fmt.Fprintf(out, "pace floor  %s, not lowerable\n", spr.PaceFloor)
			fmt.Fprintf(out, "search pace %s, its own bucket\n", spr.SearchPace)
			fmt.Fprintf(out, "api key     %s\n", key)
			return nil
		},
	}
}
