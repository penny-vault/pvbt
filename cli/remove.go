package cli

import (
	"fmt"

	"github.com/penny-vault/pvbt/library"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <short-code>",
		Short: "Remove an installed strategy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shortCode := args[0]

			if err := library.Remove(library.DefaultLibDir(), shortCode); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed strategy %q\n", shortCode)

			return nil
		},
	}
}
