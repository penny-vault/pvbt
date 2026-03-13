package cli

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newLiveCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "live",
		Short: "Run the strategy in live mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLive(strategy)
		},
	}

	cmd.Flags().Float64("cash", 100000, "Initial cash balance")

	registerStrategyFlags(cmd, strategy)

	viper.BindPFlags(cmd.Flags())

	return cmd
}

func runLive(strategy engine.Strategy) error {
	ctx := context.Background()

	cash := viper.GetFloat64("cash")

	applyStrategyFlags(strategy)

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

	for p := range ch {
		printSummary(p)
	}

	return nil
}
