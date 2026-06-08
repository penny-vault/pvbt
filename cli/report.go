package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/penny-vault/pvbt/cli/summary"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/spf13/cobra"
)

func newReportCmd(_ engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report <db file>",
		Short: "Render the summary report from a saved backtest database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(args[0], cmd.OutOrStdout())
		},
	}

	return cmd
}

func runReport(path string, writer io.Writer) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("read backtest database %q: %w", path, err)
	}

	acct, err := portfolio.FromSQLite(path)
	if err != nil {
		return fmt.Errorf("load backtest database %q: %w", path, err)
	}

	if err := summary.Render(acct, writer); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	return nil
}
