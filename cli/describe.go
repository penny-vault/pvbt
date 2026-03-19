package cli

import (
	"encoding/json"
	"fmt"

	"github.com/penny-vault/pvbt/engine"
	"github.com/spf13/cobra"
)

func newDescribeCmd(strategy engine.Strategy) *cobra.Command {
	return &cobra.Command{
		Use:   "describe",
		Short: "Output strategy metadata as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			descriptor, ok := strategy.(engine.Descriptor)
			if !ok {
				return fmt.Errorf("strategy does not implement Descriptor interface")
			}

			desc := descriptor.Describe()

			data, err := json.MarshalIndent(desc, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal descriptor: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(data))

			return nil
		},
	}
}
