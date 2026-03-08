# CLI Framework Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a reusable CLI framework in the `cli` package that strategy authors call with `cli.Run(&MyStrategy{})` to get a full-featured backtest CLI with output writers and optional TUI.

**Architecture:** Cobra root command with `backtest` and `live` subcommands. The backtest command creates a PVDataProvider from `~/.pvdata.toml`, builds an engine, runs the backtest, writes results to JSONL/Parquet, and prints a styled summary to stdout. An optional bubbletea TUI shows live equity curve, metrics sidebar, logs, and progress. Strategy-specific flags are generated via reflection from struct fields.

**Tech Stack:** cobra, viper, zerolog, charmbracelet/bubbletea, charmbracelet/lipgloss, NimbleMarkets/ntcharts, google/uuid, parquet-go

---

### Task 1: Add dependencies and create cli package skeleton

**Files:**
- Create: `cli/run.go`

**Step 1: Install dependencies**

Run:
```bash
go get github.com/spf13/cobra
go get github.com/spf13/viper
go get github.com/google/uuid
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbletea
go get github.com/NimbleMarkets/ntcharts
go get github.com/parquet-go/parquet-go
```

**Step 2: Create cli/run.go with root command and Run function**

```go
package cli

import (
	"fmt"
	"os"

	"github.com/penny-vault/pvbt/engine"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Run is the single entry point for strategy authors. It builds the
// cobra command tree, parses flags, and executes the appropriate
// subcommand.
func Run(strategy engine.Strategy) {
	rootCmd := &cobra.Command{
		Use:   strategy.Name(),
		Short: fmt.Sprintf("Run the %s strategy", strategy.Name()),
	}

	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		level, err := zerolog.ParseLevel(viper.GetString("log-level"))
		if err != nil {
			level = zerolog.InfoLevel
		}
		zerolog.SetGlobalLevel(level)
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			With().Timestamp().Logger()
	}

	rootCmd.AddCommand(newBacktestCmd(strategy))
	rootCmd.AddCommand(newLiveCmd(strategy))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 3: Verify it compiles**

Run: `go build ./...`
Expected: clean build (newBacktestCmd and newLiveCmd don't exist yet, so this will fail -- that's fine, we create them next)

**Step 4: Commit**

```bash
git add cli/run.go
git commit -m "feat(cli): add package skeleton with root command and Run entry point"
```

---

### Task 2: Backtest subcommand with flags

**Files:**
- Create: `cli/backtest.go`

**Step 1: Create cli/backtest.go**

```go
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
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

	return cmd
}

// runID returns a new UUID and its 5-char prefix.
func runID() (string, string) {
	id := uuid.New().String()
	return id, id[:5]
}

// defaultOutputPath builds the default output file name.
func defaultOutputPath(strategyName string, start, end time.Time, shortID string) string {
	return fmt.Sprintf("%s-backtest-%s-%s-%s.jsonl",
		strings.ToLower(strategyName),
		start.Format("20060102"),
		end.Format("20060102"),
		shortID,
	)
}

// outputBasePath returns the base path (without extension) and the extension.
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

	// parse dates
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

	// determine output path
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

	// create data provider
	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	// create engine
	eng := engine.New(strategy, engine.WithDataProvider(provider))
	defer eng.Close()

	// create account and run
	acct := portfolio.New()
	acct, err = eng.Run(ctx, acct, start, end)
	if err != nil {
		return fmt.Errorf("backtest failed: %w", err)
	}

	// write output
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

	// print summary to stdout
	printSummary(acct)

	return nil
}
```

**Step 2: Verify it compiles (will fail on missing output/summary functions)**

Run: `go build ./...`
Expected: fails -- writePortfolio, writeTransactions, writeHoldings, writeMetrics, printSummary not defined yet

**Step 3: Commit**

```bash
git add cli/backtest.go
git commit -m "feat(cli): add backtest subcommand with flags and run orchestration"
```

---

### Task 3: Live subcommand stub

**Files:**
- Create: `cli/live.go`

**Step 1: Create cli/live.go**

```go
package cli

import (
	"fmt"

	"github.com/penny-vault/pvbt/engine"
	"github.com/spf13/cobra"
)

func newLiveCmd(strategy engine.Strategy) *cobra.Command {
	return &cobra.Command{
		Use:   "live",
		Short: "Run the strategy in live mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("live mode is not yet implemented")
		},
	}
}
```

**Step 2: Commit**

```bash
git add cli/live.go
git commit -m "feat(cli): add live subcommand stub"
```

---

### Task 4: JSONL output writers

**Files:**
- Create: `cli/output_jsonl.go`

**Step 1: Create cli/output_jsonl.go**

Implement the four JSONL writers: writePortfolioJSONL, writeTransactionsJSONL, writeHoldingsJSONL, writeMetricsJSONL. Each opens a file, writes a metadata line, then writes data lines.

Key details:
- Use `encoding/json` with `json.NewEncoder` to write one JSON object per line
- Metadata line has `"type": "metadata"` with run_id, strategy, start, end, cash, params
- Portfolio lines: date, value, cash, invested, daily_return, cumulative_return
- Transaction lines: date, action, ticker, figi, quantity, price, commission, total
- Holdings lines: date, holdings array with ticker, figi, quantity, price, value, weight
- Metrics lines: date, summary{}, risk{}, trade{}, withdrawal{} groups

Data comes from `*portfolio.Account` methods: `Transactions()`, `Holdings()`, `Summary()`, `RiskMetrics()`, `TradeMetrics()`, `WithdrawalMetrics()`.

Since Account is still a stub, write the writer functions with the correct signatures and types, but the actual data extraction will need to be filled in when Account is fully implemented. For now, write the structural code that opens files, writes metadata, iterates, and closes.

**Step 2: Commit**

```bash
git add cli/output_jsonl.go
git commit -m "feat(cli): add JSONL output writers for portfolio, transactions, holdings, metrics"
```

---

### Task 5: Parquet output writers

**Files:**
- Create: `cli/output_parquet.go`

**Step 1: Create cli/output_parquet.go**

Implement Parquet writers for the same four streams. Use `parquet-go` library. Define Go structs with parquet struct tags for each record type. The metadata (run_id etc.) is stored as key-value metadata in the Parquet file header.

Key details:
- Define struct types: PortfolioRecord, TransactionRecord, HoldingRecord, MetricRecord with `parquet` tags
- Use `parquet.NewGenericWriter[T]` to create typed writers
- Write file metadata (run_id, strategy, start, end) via `parquet.NewWriterConfig` with KeyValueMetadata

**Step 2: Commit**

```bash
git add cli/output_parquet.go
git commit -m "feat(cli): add Parquet output writers"
```

---

### Task 6: Output dispatcher

**Files:**
- Create: `cli/output.go`

**Step 1: Create cli/output.go**

Route writePortfolio/writeTransactions/writeHoldings/writeMetrics to JSONL or Parquet based on the file extension.

```go
package cli

import (
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/portfolio"
)

func writePortfolio(base, ext, runID, strategy string, start, end time.Time, cash float64, acct *portfolio.Account) error {
	switch ext {
	case ".jsonl":
		return writePortfolioJSONL(base+ext, runID, strategy, start, end, cash, acct)
	case ".parquet":
		return writePortfolioParquet(base+ext, runID, strategy, start, end, cash, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

func writeTransactions(base, ext string, acct *portfolio.Account) error {
	path := base + "-transactions" + ext
	switch ext {
	case ".jsonl":
		return writeTransactionsJSONL(path, acct)
	case ".parquet":
		return writeTransactionsParquet(path, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

func writeHoldings(base, ext string, acct *portfolio.Account) error {
	path := base + "-holdings" + ext
	switch ext {
	case ".jsonl":
		return writeHoldingsJSONL(path, acct)
	case ".parquet":
		return writeHoldingsParquet(path, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

func writeMetrics(base, ext string, acct *portfolio.Account) error {
	path := base + "-metrics" + ext
	switch ext {
	case ".jsonl":
		return writeMetricsJSONL(path, acct)
	case ".parquet":
		return writeMetricsParquet(path, acct)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}
```

**Step 2: Verify full build**

Run: `go build ./...`
Expected: clean build

**Step 3: Commit**

```bash
git add cli/output.go
git commit -m "feat(cli): add output format dispatcher routing to JSONL or Parquet by extension"
```

---

### Task 7: Styled stdout summary

**Files:**
- Create: `cli/summary.go`

**Step 1: Create cli/summary.go**

Use lipgloss to pretty-print the four metric groups (Summary, RiskMetrics, TradeMetrics, WithdrawalMetrics) to stdout in a styled table layout. Group metrics into labeled sections with aligned columns.

Key details:
- Use `lipgloss.NewStyle()` for headers, labels, values
- Four sections: Returns, Risk, Trading, Withdrawals
- Format percentages with `%.2f%%`, ratios with `%.3f`, currency with `$%,.2f`
- Call `acct.Summary()`, `acct.RiskMetrics()`, `acct.TradeMetrics()`, `acct.WithdrawalMetrics()`

**Step 2: Verify build**

Run: `go build ./...`
Expected: clean build

**Step 3: Commit**

```bash
git add cli/summary.go
git commit -m "feat(cli): add lipgloss-styled stdout summary of performance metrics"
```

---

### Task 8: Reflection-based strategy flag generation

**Files:**
- Create: `cli/flags.go`

**Step 1: Create cli/flags.go**

Use reflection to inspect the strategy struct's exported fields and register cobra flags for each one. Match fields by `pvbt` struct tag (falling back to field name). Support these Go types:

| Go Type | Flag Type |
|---------|-----------|
| `float64` | `Float64` |
| `string` | `String` |
| `bool` | `Bool` |
| `int` | `Int` |
| `time.Duration` | `Duration` |

Key details:
- `registerStrategyFlags(cmd *cobra.Command, strategy engine.Strategy)` inspects the struct via `reflect.TypeOf` / `reflect.ValueOf`
- Uses the `pvbt` struct tag for the flag name, field name as fallback (converted to kebab-case)
- Uses the `desc` struct tag for the flag description
- Uses the `default` struct tag for the default value
- After parsing, `applyStrategyFlags(strategy engine.Strategy)` uses viper to read flag values and sets them on the struct via reflection
- Call `registerStrategyFlags` in `newBacktestCmd` before returning
- Call `applyStrategyFlags` in `runBacktest` before creating the engine

**Step 2: Verify build**

Run: `go build ./...`
Expected: clean build

**Step 3: Commit**

```bash
git add cli/flags.go
git commit -m "feat(cli): add reflection-based strategy flag generation from struct fields"
```

---

### Task 9: TUI model with bubbletea

**Files:**
- Create: `cli/tui.go`

**Step 1: Create cli/tui.go**

Build the bubbletea Model with four panes:

```
+-- Equity Curve ---------+-- Metrics --+
|  (ntcharts line chart)  | Key metrics |
|                         | updating    |
+-- Logs -----------------+-------------+
| Scrollable log output                 |
+---------------------------------------+
| Progress bar with ETA                 |
+---------------------------------------+
```

Key components:
- `tuiModel` struct implementing `tea.Model` with `Init()`, `Update()`, `View()`
- Uses `ntcharts` `timeserieslinechart` for equity curve
- Metrics sidebar: lipgloss-styled labels/values
- Log pane: `viewport` bubble for scrollable logs
- Progress bar: `progress` bubble along the bottom
- Custom `tea.Msg` types: `tickMsg` (equity value update), `logMsg` (new log line), `progressMsg` (date advancement), `doneMsg` (backtest complete)
- A `zerolog.Writer` adapter that sends `logMsg` to the bubbletea program
- Layout uses lipgloss `JoinHorizontal` and `JoinVertical`

**Step 2: Wire TUI into backtest command**

In `cli/backtest.go`, when `--tui` is set:
- Create the bubbletea program
- Redirect zerolog output to the TUI log writer
- Run the backtest in a goroutine, sending messages to the program
- Block on `program.Run()`

**Step 3: Verify build**

Run: `go build ./...`
Expected: clean build

**Step 4: Commit**

```bash
git add cli/tui.go cli/backtest.go
git commit -m "feat(cli): add bubbletea TUI with equity curve, metrics sidebar, logs, and progress"
```

---

### Task 10: Integration test with a mock strategy

**Files:**
- Create: `cli/cli_test.go`

**Step 1: Create a test strategy and verify the CLI wires up correctly**

Write a test that:
- Defines a minimal mock strategy implementing `engine.Strategy`
- Calls the backtest command programmatically (via cobra's `Execute` with args)
- Verifies the output file is created with a valid metadata line
- Verifies the default filename pattern matches `{strategy}-backtest-{YYYYMMDD}-{YYYYMMDD}-{5char}.jsonl`
- Verifies `--output-transactions`, `--output-holdings`, `--output-metrics` flags create additional files

Note: since `Engine.Run` is a stub that returns the account unchanged, these tests verify the CLI wiring, flag parsing, file creation, and output format -- not backtest correctness.

Test file uses ginkgo/gomega to match project conventions.

**Step 2: Run tests**

Run: `ginkgo ./cli/...`
Expected: all tests pass

**Step 3: Commit**

```bash
git add cli/cli_test.go
git commit -m "test(cli): add integration tests for CLI wiring, flags, and output file creation"
```

---

### Task 11: Final build and cleanup

**Step 1: Run full build**

Run: `go build ./...`
Expected: clean build

**Step 2: Run all tests**

Run: `ginkgo ./...`
Expected: all tests pass

**Step 3: Run go vet**

Run: `go vet ./...`
Expected: no issues

**Step 4: Commit any cleanup**

```bash
git add -A
git commit -m "chore(cli): final cleanup and vet fixes"
```
