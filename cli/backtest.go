package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/cli/summary"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
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
	cmd.Flags().Bool("no-progress", false, "Disable the interactive progress bar (logs go straight to stderr)")
	cmd.Flags().Bool("json", false, "Output JSON Lines to stdout (for programmatic consumers)")

	registerStrategyFlags(cmd, strategy)
	cmd.Flags().String("preset", "", "Apply a named parameter preset")
	cmd.Flags().String("benchmark", "", "Benchmark ticker for performance comparison")
	cmd.Flags().String("risk-profile", "", "Risk profile (conservative, moderate, aggressive, none)")
	cmd.Flags().Bool("tax", false, "Enable tax optimization")

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

	jsonMode, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	var reporter *jsonReporter
	if jsonMode {
		savedLogger := log.Logger
		log.Logger = zerolog.New(os.Stdout).With().Str("type", "log").Timestamp().Logger()
		defer func() { log.Logger = savedLogger }()
		reporter = newJSONReporter(os.Stdout)
	}

	fullID, shortID := runID()

	outputPath, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	if outputPath == "" {
		info := engine.DescribeStrategy(strategy)

		filePrefix := info.ShortCode
		if filePrefix == "" {
			filePrefix = strategy.Name()
		}

		outputPath = defaultOutputPath(filePrefix, start, end, shortID)
	}

	log.Info().
		Str("strategy", strategy.Name()).
		Time("start", start).
		Time("end", end).
		Float64("cash", cash).
		Str("output", outputPath).
		Str("run_id", fullID).
		Msg("starting backtest")

	if jsonMode {
		reporter.Started(fullID, strategy.Name(), start.Format("2006-01-02"), end.Format("2006-01-02"), cash, outputPath)
	}

	if err := applyPreset(cmd, strategy); err != nil {
		return err
	}

	applyStrategyFlags(cmd, strategy)

	cfg, err := loadMiddlewareConfigFromCommand(cmd)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	noProgress, err := cmd.Flags().GetBool("no-progress")
	if err != nil {
		return err
	}

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	acct := portfolio.New(
		portfolio.WithCash(cash, start),
		portfolio.WithAllMetrics(),
	)

	engineOpts := []engine.Option{
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithAccount(acct),
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

	useProgress := !noProgress && !jsonMode && stderrIsTerminal()

	var (
		program   *tea.Program
		logWriter io.Writer
	)

	if useProgress {
		title := fmt.Sprintf("Backtest: %s", strategy.Name())
		model := newProgressModel(title, start, end)
		program = tea.NewProgram(model, tea.WithOutput(os.Stderr))

		engineOpts = append(engineOpts, engine.WithProgressCallback(func(ev engine.ProgressEvent) {
			program.Send(progressUpdateMsg{
				step:         ev.Step,
				totalSteps:   ev.TotalSteps,
				date:         ev.Date,
				measurements: ev.MeasurementsEvaluated,
			})
		}))

		logPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".log"

		lf, err := os.Create(logPath)
		if err != nil {
			return fmt.Errorf("create log file %q: %w", logPath, err)
		}

		defer lf.Close()

		logWriter = lf

		fmt.Fprintf(os.Stderr, "Logs: %s\n", logPath)
	}

	if jsonMode {
		engineOpts = append(engineOpts, engine.WithProgressCallback(func(ev engine.ProgressEvent) {
			reporter.Progress(ev)
		}))
	}

	eng := engine.New(strategy, engineOpts...)
	defer eng.Close()

	startTime := time.Now()

	result, err := runEngineBacktest(eng, program, logWriter, start, end)
	if err != nil {
		if jsonMode {
			reporter.Error(fullID, err)
		}
		return fmt.Errorf("backtest failed: %w", err)
	}

	elapsed := time.Since(startTime)

	// Set metadata on the portfolio.
	result.SetMetadata(portfolio.MetaRunElapsed, elapsed.String())
	result.SetMetadata(portfolio.MetaRunInitialCash, fmt.Sprintf("%.2f", cash))
	result.SetMetadata("run_id", fullID)
	result.SetMetadata(portfolio.MetaStrategyName, strategy.Name())
	result.SetMetadata(portfolio.MetaRunStart, start.Format("2006-01-02"))
	result.SetMetadata(portfolio.MetaRunEnd, end.Format("2006-01-02"))

	params := strategyParams(strategy)
	for k, v := range params {
		result.SetMetadata(fmt.Sprintf("param_%s", k), fmt.Sprintf("%v", v))
	}

	if err := acct.ToSQLite(outputPath); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	log.Info().Str("path", outputPath).Msg("backtest output written")

	if jsonMode {
		reporter.Completed(fullID, outputPath)
	} else {
		if err := summary.Render(acct, os.Stdout); err != nil {
			return fmt.Errorf("rendering report: %w", err)
		}
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

		if engine.IsTestOnlyField(field) {
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

// runEngineBacktest runs the engine, optionally rendering a bubble tea
// progress bar. When program is nil the engine is run inline and zerolog
// continues writing to its existing destination — intentionally, so that
// callers (e.g. the --json path) can pre-configure log.Logger before the
// call. When program is non-nil, zerolog is redirected to logWriter (a
// plain-text file) before the context is created, so all engine code using
// zerolog.Ctx(ctx) also writes to the file rather than stderr.
func runEngineBacktest(eng *engine.Engine, program *tea.Program, logWriter io.Writer, start, end time.Time) (portfolio.Portfolio, error) {
	if program == nil {
		ctx := log.Logger.WithContext(context.Background())

		return eng.Backtest(ctx, start, end)
	}

	savedLogger := log.Logger
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: logWriter, NoColor: true}).
		With().Timestamp().Logger()

	defer func() { log.Logger = savedLogger }()

	ctx := log.Logger.WithContext(context.Background())

	go func() {
		result, err := eng.Backtest(ctx, start, end)
		program.Send(progressDoneMsg{result: result, err: err})
	}()

	finalModel, runErr := program.Run()
	if runErr != nil {
		return nil, fmt.Errorf("progress UI: %w", runErr)
	}

	final, ok := finalModel.(progressModel)
	if !ok {
		return nil, fmt.Errorf("progress UI: unexpected model type %T", finalModel)
	}

	return final.result, final.err
}
