package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	backtestReport "github.com/penny-vault/pvbt/study/report"
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
	cmd.Flags().String("preset", "", "Apply a named parameter preset")
	cmd.Flags().String("benchmark", "", "Benchmark ticker for performance comparison")
	cmd.Flags().String("risk-profile", "", "Risk profile (conservative, moderate, aggressive, none)")
	cmd.Flags().Bool("tax", false, "Enable tax optimization")

	return cmd
}

func runLive(cmd *cobra.Command, strategy engine.Strategy) error {
	ctx := context.Background()

	cash, err := cmd.Flags().GetFloat64("cash")
	if err != nil {
		return err
	}

	if err := applyPreset(cmd, strategy); err != nil {
		return err
	}

	applyStrategyFlags(cmd, strategy)

	cfg, err := loadMiddlewareConfigFromCommand(cmd)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	engineOpts := []engine.Option{
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithInitialDeposit(cash),
	}

	if cfg.HasMiddleware() {
		engineOpts = append(engineOpts, engine.WithMiddlewareConfig(*cfg))
	}

	benchmarkTicker, err := cmd.Flags().GetString("benchmark")
	if err != nil {
		return err
	}

	if benchmarkTicker != "" {
		engineOpts = append(engineOpts, engine.WithBenchmarkTicker(benchmarkTicker))
	}

	eng := engine.New(strategy, engineOpts...)
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
		p.SetMetadata(portfolio.MetaRunInitialCash, fmt.Sprintf("%.2f", cash))

		reportable, ok := p.(backtestReport.ReportablePortfolio)
		if !ok {
			log.Warn().Msg("portfolio does not support full reporting")
			continue
		}

		rpt, buildErr := backtestReport.Summary(reportable)
		if buildErr != nil {
			log.Warn().Err(buildErr).Msg("some report metrics failed")
		}

		if renderErr := rpt.Render(backtestReport.FormatText, os.Stdout); renderErr != nil {
			return fmt.Errorf("rendering report: %w", renderErr)
		}
	}

	return nil
}
