package cli

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	backtestReport "github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/report/terminal"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newBacktestCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backtest",
		Short: "Run a historical backtest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacktest(cmd, strategy)
		},
	}

	now := time.Now()
	fiveYearsAgo := now.AddDate(-5, 0, 0)

	cmd.Flags().String("start", fiveYearsAgo.Format("2006-01-02"), "Backtest start date (YYYY-MM-DD)")
	cmd.Flags().String("end", now.Format("2006-01-02"), "Backtest end date (YYYY-MM-DD)")
	cmd.Flags().Float64("cash", 100000, "Initial cash balance")
	cmd.Flags().String("output", "", "Output file path (default: auto-generated)")
	cmd.Flags().Bool("tui", false, "Enable interactive TUI")

	registerStrategyFlags(cmd, strategy)
	cmd.Flags().String("preset", "", "Apply a named parameter preset")

	return cmd
}

func runID() (string, string) {
	id := uuid.New().String()
	return id, id[:5]
}

func defaultOutputPath(strategyName string, start, end time.Time, shortID string) string {
	return fmt.Sprintf("%s-backtest-%s-%s-%s.db",
		strings.ToLower(strategyName),
		start.Format("20060102"),
		end.Format("20060102"),
		shortID,
	)
}

func runBacktest(cmd *cobra.Command, strategy engine.Strategy) error {
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

	fullID, shortID := runID()

	outputPath, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

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

	if err := applyPreset(cmd, strategy); err != nil {
		return err
	}

	applyStrategyFlags(cmd, strategy)

	useTUI, err := cmd.Flags().GetBool("tui")
	if err != nil {
		return err
	}

	if useTUI {
		return runBacktestWithTUI(strategy)
	}

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	acct := portfolio.New(
		portfolio.WithCash(cash, start),
		portfolio.WithAllMetrics(),
	)

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithAccount(acct),
	)
	defer eng.Close()

	startTime := time.Now()

	result, err := eng.Backtest(ctx, start, end)
	if err != nil {
		return fmt.Errorf("backtest failed: %w", err)
	}

	elapsed := time.Since(startTime)

	// Set metadata on the portfolio.
	result.SetMetadata("run_id", fullID)
	result.SetMetadata("strategy", strategy.Name())
	result.SetMetadata("start", start.Format("2006-01-02"))
	result.SetMetadata("end", end.Format("2006-01-02"))

	params := strategyParams(strategy)
	for k, v := range params {
		result.SetMetadata(fmt.Sprintf("param_%s", k), fmt.Sprintf("%v", v))
	}

	if err := acct.ToSQLite(outputPath); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	log.Info().Str("path", outputPath).Msg("backtest output written")

	info := engine.DescribeStrategy(strategy)

	steps := 0
	if result.PerfData() != nil {
		steps = result.PerfData().Len()
	}

	rpt, err := backtestReport.Build(result, info, backtestReport.RunMeta{
		Elapsed:     elapsed,
		Steps:       steps,
		InitialCash: cash,
	})
	if err != nil {
		log.Warn().Err(err).Msg("some report metrics failed")
	}

	if err := terminal.Render(rpt, os.Stdout); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	return nil
}

func strategyParams(strategy engine.Strategy) map[string]any {
	params := make(map[string]any)

	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	strategyType := val.Type()
	if strategyType.Kind() != reflect.Struct {
		return params
	}

	for ii := 0; ii < strategyType.NumField(); ii++ {
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = strings.ToLower(field.Name)
		}

		params[name] = val.Field(ii).Interface()
	}

	return params
}

func runBacktestWithTUI(strategy engine.Strategy) error {
	m := newTUIModel()
	program := tea.NewProgram(m, tea.WithAltScreen())

	// redirect logs to TUI
	w := newTUILogWriter(program)
	log.Logger = zerolog.New(w).With().Timestamp().Logger()

	// run backtest in background
	go func() {
		// simulate some progress for now since Engine.Run is a stub
		program.Send(doneMsg{})
	}()

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
