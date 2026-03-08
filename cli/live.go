package cli

import (
	"fmt"

	"github.com/penny-vault/pvbt/engine"
	"github.com/spf13/cobra"
)

func newLiveCmd(strategy engine.Strategy) *cobra.Command {
	return &cobra.Command{
		Use:   "live",
		Short: "Run the strategy in live mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("live mode is not yet implemented")
		},
	}
}
