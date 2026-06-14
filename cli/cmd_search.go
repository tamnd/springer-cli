package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search Springer Nature papers by keyword",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			if query == "" {
				return codeError(exitUsage, fmt.Errorf("query cannot be empty"))
			}
			limit := a.effectiveLimit(20)
			papers, err := a.client.Search(cmd.Context(), query, limit)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(papers, len(papers))
		},
	}
}
