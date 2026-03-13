package cli

import (
	"context"
	"fmt"
	"reflect"
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
	return fmt.Sprintf("%s-backtest-%s-%s-%s.db",
		strings.ToLower(strategyName),
		start.Format("20060102"),
		end.Format("20060102"),
		shortID,
	)
}

func runBacktest(strategy engine.Strategy) error {
	ctx := log.Logger.WithContext(context.Background())

	nyc, _ := time.LoadLocation("America/New_York")
	start, err := time.ParseInLocation("2006-01-02", viper.GetString("start"), nyc)
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}
	end, err := time.ParseInLocation("2006-01-02", viper.GetString("end"), nyc)
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

	acct := portfolio.New(
		portfolio.WithCash(cash),
		portfolio.WithAllMetrics(),
	)

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithAccount(acct),
	)
	defer eng.Close()

	p, err := eng.Backtest(ctx, start, end)
	if err != nil {
		return fmt.Errorf("backtest failed: %w", err)
	}

	// Set metadata on the portfolio.
	p.SetMetadata("run_id", fullID)
	p.SetMetadata("strategy", strategy.Name())
	p.SetMetadata("start", start.Format("2006-01-02"))
	p.SetMetadata("end", end.Format("2006-01-02"))

	params := strategyParams(strategy)
	for k, v := range params {
		p.SetMetadata(fmt.Sprintf("param_%s", k), fmt.Sprintf("%v", v))
	}

	if err := acct.ToSQLite(outputPath); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	log.Info().Str("path", outputPath).Msg("backtest output written")

	printSummary(p)

	return nil
}

func strategyParams(strategy engine.Strategy) map[string]any {
	params := make(map[string]any)
	v := reflect.ValueOf(strategy)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return params
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("pvbt")
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		params[name] = v.Field(i).Interface()
	}
	return params
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
