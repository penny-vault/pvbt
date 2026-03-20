package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	backtestReport "github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/report/terminal"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newLiveCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "live",
		Short: "Run the strategy in live mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLive(cmd, strategy)
		},
	}

	cmd.Flags().Float64("cash", 100000, "Initial cash balance")

	registerStrategyFlags(cmd, strategy)

	return cmd
}

func runLive(cmd *cobra.Command, strategy engine.Strategy) error {
	ctx := context.Background()

	cash, err := cmd.Flags().GetFloat64("cash")
	if err != nil {
		return err
	}

	applyStrategyFlags(cmd, strategy)

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithInitialDeposit(cash),
	)
	defer eng.Close()

	log.Info().
		Str("strategy", strategy.Name()).
		Float64("cash", cash).
		Msg("starting live mode")

	ch, err := eng.RunLive(ctx)
	if err != nil {
		return fmt.Errorf("live mode failed: %w", err)
	}

	info := engine.DescribeStrategy(strategy)

	for p := range ch {
		rpt, buildErr := backtestReport.Build(p, info, backtestReport.RunMeta{
			InitialCash: cash,
		})
		if buildErr != nil {
			log.Warn().Err(buildErr).Msg("some report metrics failed")
		}

		if renderErr := terminal.Render(rpt, os.Stdout); renderErr != nil {
			return fmt.Errorf("rendering report: %w", renderErr)
		}
	}

	return nil
}
