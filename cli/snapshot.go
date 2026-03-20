package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	_ "modernc.org/sqlite"
)

func newSnapshotCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Run a backtest and capture all data access to a snapshot file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSnapshot(cmd, strategy)
		},
	}

	now := time.Now()
	fiveYearsAgo := now.AddDate(-5, 0, 0)

	cmd.Flags().String("start", fiveYearsAgo.Format("2006-01-02"), "Backtest start date (YYYY-MM-DD)")
	cmd.Flags().String("end", now.Format("2006-01-02"), "Backtest end date (YYYY-MM-DD)")
	cmd.Flags().Float64("cash", 100000, "Initial cash balance")
	cmd.Flags().String("output", "", "Snapshot output path (default: pv-data-snapshot-{strategy}-{start}-{end}.db)")

	registerStrategyFlags(cmd, strategy)
	cmd.Flags().String("preset", "", "Apply a named parameter preset")

	return cmd
}

func defaultSnapshotPath(strategyName string, start, end time.Time) string {
	return fmt.Sprintf("pv-data-snapshot-%s-%s-%s.db",
		strings.ToLower(strategyName),
		start.Format("20060102"),
		end.Format("20060102"),
	)
}

func runSnapshot(cmd *cobra.Command, strategy engine.Strategy) error {
	ctx := log.Logger.WithContext(context.Background())

	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("load America/New_York timezone: %w", err)
	}

	startStr, err := cmd.Flags().GetString("start")
	if err != nil {
		return err
	}

	start, err := time.ParseInLocation("2006-01-02", startStr, nyc)
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}

	endStr, err := cmd.Flags().GetString("end")
	if err != nil {
		return err
	}

	end, err := time.ParseInLocation("2006-01-02", endStr, nyc)
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	cash, err := cmd.Flags().GetFloat64("cash")
	if err != nil {
		return err
	}

	outputPath, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	if outputPath == "" {
		outputPath = defaultSnapshotPath(strategy.Name(), start, end)
	}

	log.Info().
		Str("strategy", strategy.Name()).
		Time("start", start).
		Time("end", end).
		Str("output", outputPath).
		Msg("starting snapshot capture")

	if err := applyPreset(cmd, strategy); err != nil {
		return err
	}

	applyStrategyFlags(cmd, strategy)

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	recorder, err := data.NewSnapshotRecorder(outputPath, data.SnapshotRecorderConfig{
		BatchProvider: provider,
		AssetProvider: provider,
		// IndexProvider and RatingProvider are nil unless PVDataProvider
		// implements them in the future or the strategy registers its own.
	})
	if err != nil {
		provider.Close()

		return fmt.Errorf("create snapshot recorder: %w", err)
	}

	acct := portfolio.New(
		portfolio.WithCash(cash, start),
		portfolio.WithAllMetrics(),
	)

	eng := engine.New(strategy,
		engine.WithDataProvider(recorder),
		engine.WithAssetProvider(recorder),
		engine.WithAccount(acct),
	)

	// Do NOT defer eng.Close() here -- the engine would close the recorder
	// (its registered provider), causing a double-close when we also close
	// the recorder and the underlying provider below.

	_, err = eng.Backtest(ctx, start, end)
	if err != nil {
		// Engine.Close() only closes registered providers, and the recorder is
		// the only registered provider. Close recorder and underlying provider
		// directly -- no need to also close the engine.
		recorder.Close()
		provider.Close()

		return fmt.Errorf("backtest failed: %w", err)
	}

	// Close the recorder first (flushes SQLite), then the underlying provider
	// (releases the pgxpool). The engine does not own these lifetimes.
	if err := recorder.Close(); err != nil {
		provider.Close()
		return fmt.Errorf("close snapshot recorder: %w", err)
	}

	if err := provider.Close(); err != nil {
		return fmt.Errorf("close data provider: %w", err)
	}

	// Print summary with row counts per table.
	summaryDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		return fmt.Errorf("open snapshot for summary: %w", err)
	}
	defer summaryDB.Close()

	tables := []string{"assets", "eod", "metrics", "fundamentals", "ratings", "index_members", "market_holidays"}
	for _, table := range tables {
		var count int
		if err := summaryDB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			log.Warn().Err(err).Str("table", table).Msg("could not count rows")
			continue
		}

		if count > 0 {
			log.Info().Str("table", table).Int("rows", count).Msg("snapshot table")
		}
	}

	log.Info().Str("path", outputPath).Msg("snapshot written")

	return nil
}
