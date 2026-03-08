package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newBacktestCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backtest",
		Short: "Run a historical backtest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacktest(strategy)
		},
	}

	now := time.Now()
	fiveYearsAgo := now.AddDate(-5, 0, 0)

	cmd.Flags().String("start", fiveYearsAgo.Format("2006-01-02"), "Backtest start date (YYYY-MM-DD)")
	cmd.Flags().String("end", now.Format("2006-01-02"), "Backtest end date (YYYY-MM-DD)")
	cmd.Flags().Float64("cash", 100000, "Initial cash balance")
	cmd.Flags().String("output", "", "Output file path (default: auto-generated)")
	cmd.Flags().Bool("output-transactions", false, "Write transaction log")
	cmd.Flags().Bool("output-holdings", false, "Write holdings snapshots")
	cmd.Flags().Bool("output-metrics", false, "Write rolling performance metrics")
	cmd.Flags().Bool("tui", false, "Enable interactive TUI")

	viper.BindPFlags(cmd.Flags())

	registerStrategyFlags(cmd, strategy)

	return cmd
}

func runID() (string, string) {
	id := uuid.New().String()
	return id, id[:5]
}

func defaultOutputPath(strategyName string, start, end time.Time, shortID string) string {
	return fmt.Sprintf("%s-backtest-%s-%s-%s.jsonl",
		strings.ToLower(strategyName),
		start.Format("20060102"),
		end.Format("20060102"),
		shortID,
	)
}

func outputBasePath(path string) (string, string) {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	if ext == "" {
		ext = ".jsonl"
	}
	return base, ext
}

func runBacktest(strategy engine.Strategy) error {
	ctx := context.Background()

	start, err := time.Parse("2006-01-02", viper.GetString("start"))
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}
	end, err := time.Parse("2006-01-02", viper.GetString("end"))
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	cash := viper.GetFloat64("cash")
	fullID, shortID := runID()

	outputPath := viper.GetString("output")
	if outputPath == "" {
		outputPath = defaultOutputPath(strategy.Name(), start, end, shortID)
	}

	log.Info().
		Str("strategy", strategy.Name()).
		Time("start", start).
		Time("end", end).
		Float64("cash", cash).
		Str("output", outputPath).
		Str("run_id", fullID).
		Msg("starting backtest")

	applyStrategyFlags(strategy)

	if viper.GetBool("tui") {
		return runBacktestWithTUI(strategy)
	}

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	eng := engine.New(strategy, engine.WithDataProvider(provider))
	defer eng.Close()

	acct := portfolio.New(portfolio.WithCash(cash))
	acct, err = eng.Run(ctx, acct, start, end)
	if err != nil {
		return fmt.Errorf("backtest failed: %w", err)
	}

	base, ext := outputBasePath(outputPath)
	if err := writePortfolio(base, ext, fullID, strategy.Name(), start, end, cash, acct); err != nil {
		return err
	}

	if viper.GetBool("output-transactions") {
		if err := writeTransactions(base, ext, acct); err != nil {
			return err
		}
	}

	if viper.GetBool("output-holdings") {
		if err := writeHoldings(base, ext, acct); err != nil {
			return err
		}
	}

	if viper.GetBool("output-metrics") {
		if err := writeMetrics(base, ext, acct); err != nil {
			return err
		}
	}

	printSummary(acct)

	return nil
}

func runBacktestWithTUI(strategy engine.Strategy) error {
	m := newTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	// redirect logs to TUI
	w := newTUILogWriter(p)
	log.Logger = zerolog.New(w).With().Timestamp().Logger()

	// run backtest in background
	go func() {
		// simulate some progress for now since Engine.Run is a stub
		p.Send(doneMsg{})
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
