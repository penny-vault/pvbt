package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/penny-vault/pvbt/library"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed strategies",
		RunE: func(cmd *cobra.Command, args []string) error {
			strategies, err := library.List(library.DefaultLibDir())
			if err != nil {
				return fmt.Errorf("list strategies: %w", err)
			}

			if len(strategies) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No strategies installed. Use 'pvbt discover' to find strategies.")
				return nil
			}

			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "SHORT-CODE\tVERSION\tREPO")

			for _, strategy := range strategies {
				fmt.Fprintf(writer, "%s\t%s\t%s/%s\n",
					strategy.ShortCode, strategy.Version,
					strategy.RepoOwner, strategy.RepoName)
			}

			return writer.Flush()
		},
	}
}
