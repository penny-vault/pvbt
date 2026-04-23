# Backtest JSON Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `--json` flag to the `backtest` subcommand that switches stdout to a JSON Lines stream of progress, log, and lifecycle messages for consumption by a web UI.

**Architecture:** A `jsonReporter` helper in `cli/` handles all structured writes to stdout; zerolog is reconfigured to output native JSON (with a `"type":"log"` context field) instead of the styled console writer; the existing progress callback abstraction routes engine events through the reporter instead of the bubble tea model.

**Tech Stack:** Go stdlib `encoding/json`, `github.com/rs/zerolog`, `github.com/penny-vault/pvbt/engine.ProgressEvent`, Ginkgo/Gomega for tests.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `cli/json_reporter.go` | Create | `jsonReporter` struct, `progressFraction`, `computeEtaMS` |
| `cli/json_reporter_test.go` | Create | Unit tests for all reporter methods and helpers |
| `cli/backtest.go` | Modify | Register `--json` flag, wire reporter and progress callback |

---

## Task 1: Write failing tests for `jsonReporter`

**Files:**
- Create: `cli/json_reporter_test.go`

- [ ] **Step 1.1: Create the test file**

```go
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/engine"
)

// parseLines parses all newline-delimited JSON objects written to buf.
func parseLines(buf *bytes.Buffer) []map[string]any {
	GinkgoHelper()
	var results []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		Expect(json.Unmarshal([]byte(line), &m)).To(Succeed())
		results = append(results, m)
	}
	return results
}

var _ = Describe("progressFraction", func() {
	It("returns 0 when Date is zero", func() {
		ev := engine.ProgressEvent{
			Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		Expect(progressFraction(ev)).To(Equal(0.0))
	})

	It("returns 0 when End is not after Start", func() {
		t := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		ev := engine.ProgressEvent{Start: t, End: t, Date: t}
		Expect(progressFraction(ev)).To(Equal(0.0))
	})

	It("returns ~0.5 for the midpoint date", func() {
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
		mid := start.Add(end.Sub(start) / 2)
		ev := engine.ProgressEvent{Start: start, End: end, Date: mid}
		Expect(progressFraction(ev)).To(BeNumerically("~", 0.5, 0.01))
	})

	It("clamps to 1 when Date exceeds End", func() {
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
		ev := engine.ProgressEvent{Start: start, End: end, Date: end.AddDate(1, 0, 0)}
		Expect(progressFraction(ev)).To(Equal(1.0))
	})
})

var _ = Describe("computeEtaMS", func() {
	It("returns 0 when pct is 0", func() {
		Expect(computeEtaMS(10*time.Second, 0)).To(Equal(int64(0)))
	})

	It("returns 0 when pct is 1", func() {
		Expect(computeEtaMS(10*time.Second, 1.0)).To(Equal(int64(0)))
	})

	It("returns approximate remaining ms at 25% completion", func() {
		// 25% done in 10s → 30s remaining
		eta := computeEtaMS(10*time.Second, 0.25)
		Expect(eta).To(BeNumerically("~", int64(30000), int64(100)))
	})
})

var _ = Describe("jsonReporter", func() {
	var (
		buf      *bytes.Buffer
		reporter *jsonReporter
	)

	BeforeEach(func() {
		buf = &bytes.Buffer{}
		reporter = newJSONReporter(buf)
	})

	Describe("Started", func() {
		It("emits a started status line with all fields", func() {
			reporter.Started("run123", "my-strategy", "2020-01-01", "2025-01-01", 100000, "/tmp/out.db")
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("status"))
			Expect(m["event"]).To(Equal("started"))
			Expect(m["run_id"]).To(Equal("run123"))
			Expect(m["strategy"]).To(Equal("my-strategy"))
			Expect(m["start"]).To(Equal("2020-01-01"))
			Expect(m["end"]).To(Equal("2025-01-01"))
			Expect(m["cash"]).To(BeNumerically("~", 100000.0, 0.01))
			Expect(m["output"]).To(Equal("/tmp/out.db"))
			Expect(m["time"]).NotTo(BeEmpty())
		})
	})

	Describe("Completed", func() {
		It("emits a completed status line with elapsed_ms", func() {
			reporter.Completed("run123", "/tmp/out.db")
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("status"))
			Expect(m["event"]).To(Equal("completed"))
			Expect(m["run_id"]).To(Equal("run123"))
			Expect(m["output"]).To(Equal("/tmp/out.db"))
			Expect(m["elapsed_ms"]).NotTo(BeNil())
			Expect(m["time"]).NotTo(BeEmpty())
		})
	})

	Describe("Error", func() {
		It("emits an error status line with the error message", func() {
			reporter.Error("run123", fmt.Errorf("data provider unavailable"))
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("status"))
			Expect(m["event"]).To(Equal("error"))
			Expect(m["run_id"]).To(Equal("run123"))
			Expect(m["error"]).To(Equal("data provider unavailable"))
			Expect(m["time"]).NotTo(BeEmpty())
		})
	})

	Describe("Progress", func() {
		It("emits a progress line with all fields", func() {
			start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			mid := start.Add(end.Sub(start) / 2)
			ev := engine.ProgressEvent{
				Step:                  500,
				TotalSteps:            1000,
				Date:                  mid,
				Start:                 start,
				End:                   end,
				MeasurementsEvaluated: 12345,
			}
			reporter.Progress(ev)
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("progress"))
			Expect(m["step"]).To(BeNumerically("~", float64(500), 1))
			Expect(m["total_steps"]).To(BeNumerically("~", float64(1000), 1))
			Expect(m["current_date"]).To(Equal(mid.Format("2006-01-02")))
			Expect(m["target_date"]).To(Equal("2025-01-01"))
			Expect(m["pct"]).To(BeNumerically("~", float64(50), 1))
			Expect(m["elapsed_ms"]).NotTo(BeNil())
			Expect(m["measurements"]).To(BeNumerically("~", float64(12345), 1))
		})
	})
})

var _ = Describe("backtest --json flag", func() {
	It("is registered on the backtest command", func() {
		strategy := &testStrategy{}
		cmd := newBacktestCmd(strategy)
		f := cmd.Flags().Lookup("json")
		Expect(f).NotTo(BeNil())
		Expect(f.Value.Type()).To(Equal("bool"))
	})
})
```

- [ ] **Step 1.2: Run the tests to confirm they fail**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
go test ./cli/... -run "progressFraction|computeEtaMS|jsonReporter|backtest.*json" -v 2>&1 | head -40
```

Expected: compile errors — `progressFraction`, `computeEtaMS`, `newJSONReporter`, and `reporter.Started/Completed/Error/Progress` are undefined.

---

## Task 2: Implement `cli/json_reporter.go`

**Files:**
- Create: `cli/json_reporter.go`

- [ ] **Step 2.1: Create the implementation file**

```go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/penny-vault/pvbt/engine"
)

type jsonReporter struct {
	w         io.Writer
	startTime time.Time
}

func newJSONReporter(w io.Writer) *jsonReporter {
	return &jsonReporter{w: w, startTime: time.Now()}
}

type statusStartedMsg struct {
	Type     string  `json:"type"`
	Event    string  `json:"event"`
	RunID    string  `json:"run_id"`
	Strategy string  `json:"strategy"`
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Cash     float64 `json:"cash"`
	Output   string  `json:"output"`
	Time     string  `json:"time"`
}

type statusCompletedMsg struct {
	Type      string `json:"type"`
	Event     string `json:"event"`
	RunID     string `json:"run_id"`
	Output    string `json:"output"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Time      string `json:"time"`
}

type statusErrorMsg struct {
	Type   string `json:"type"`
	Event  string `json:"event"`
	RunID  string `json:"run_id"`
	Error  string `json:"error"`
	Time   string `json:"time"`
}

type progressLineMsg struct {
	Type         string  `json:"type"`
	Step         int     `json:"step"`
	TotalSteps   int     `json:"total_steps"`
	CurrentDate  string  `json:"current_date"`
	TargetDate   string  `json:"target_date"`
	Pct          float64 `json:"pct"`
	ElapsedMS    int64   `json:"elapsed_ms"`
	EtaMS        int64   `json:"eta_ms"`
	Measurements int     `json:"measurements"`
}

func (r *jsonReporter) Started(runID, strategy, startDate, endDate string, cash float64, output string) {
	r.writeJSON(statusStartedMsg{
		Type:     "status",
		Event:    "started",
		RunID:    runID,
		Strategy: strategy,
		Start:    startDate,
		End:      endDate,
		Cash:     cash,
		Output:   output,
		Time:     time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *jsonReporter) Completed(runID, output string) {
	r.writeJSON(statusCompletedMsg{
		Type:      "status",
		Event:     "completed",
		RunID:     runID,
		Output:    output,
		ElapsedMS: time.Since(r.startTime).Milliseconds(),
		Time:      time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *jsonReporter) Error(runID string, err error) {
	r.writeJSON(statusErrorMsg{
		Type:   "status",
		Event:  "error",
		RunID:  runID,
		Error:  err.Error(),
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *jsonReporter) Progress(ev engine.ProgressEvent) {
	pct := progressFraction(ev)
	elapsed := time.Since(r.startTime)
	r.writeJSON(progressLineMsg{
		Type:         "progress",
		Step:         ev.Step,
		TotalSteps:   ev.TotalSteps,
		CurrentDate:  ev.Date.Format("2006-01-02"),
		TargetDate:   ev.End.Format("2006-01-02"),
		Pct:          math.Round(pct*10000) / 100,
		ElapsedMS:    elapsed.Milliseconds(),
		EtaMS:        computeEtaMS(elapsed, pct),
		Measurements: ev.MeasurementsEvaluated,
	})
}

func (r *jsonReporter) writeJSON(v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintln(r.w, string(b))
}

// progressFraction computes completion as a fraction in [0,1] from the date span.
func progressFraction(ev engine.ProgressEvent) float64 {
	if ev.Date.IsZero() || !ev.End.After(ev.Start) {
		return 0
	}
	span := ev.End.Sub(ev.Start).Seconds()
	if span <= 0 {
		return 0
	}
	frac := ev.Date.Sub(ev.Start).Seconds() / span
	switch {
	case frac < 0:
		return 0
	case frac > 1:
		return 1
	default:
		return frac
	}
}

// computeEtaMS returns estimated milliseconds remaining, or 0 if pct is 0 or 1.
func computeEtaMS(elapsed time.Duration, pct float64) int64 {
	if pct <= 0 || pct >= 1 {
		return 0
	}
	return time.Duration(float64(elapsed) / pct * (1 - pct)).Milliseconds()
}
```

- [ ] **Step 2.2: Run the tests to confirm they pass**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
go test ./cli/... -run "progressFraction|computeEtaMS|jsonReporter" -v 2>&1 | tail -20
```

Expected: all `progressFraction`, `computeEtaMS`, and `jsonReporter` specs PASS.

- [ ] **Step 2.3: Commit**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
git add cli/json_reporter.go cli/json_reporter_test.go
git commit -m "feat(cli): add jsonReporter with progress, status, and log helpers"
```

---

## Task 3: Wire `--json` into `cli/backtest.go`

**Files:**
- Modify: `cli/backtest.go`

- [ ] **Step 3.1: Run the `--json` flag test to confirm it fails**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
go test ./cli/... -run "backtest.*json" -v 2>&1
```

Expected: FAIL — `Lookup("json")` returns nil.

- [ ] **Step 3.2: Register `--json` flag in `newBacktestCmd`**

In `newBacktestCmd`, after the `no-progress` flag registration, add:

```go
cmd.Flags().Bool("json", false, "Output JSON Lines to stdout (for programmatic consumers)")
```

The block becomes:

```go
cmd.Flags().String("start", fiveYearsAgo.Format("2006-01-02"), "Backtest start date (YYYY-MM-DD)")
cmd.Flags().String("end", now.Format("2006-01-02"), "Backtest end date (YYYY-MM-DD)")
cmd.Flags().Float64("cash", 100000, "Initial cash balance")
cmd.Flags().String("output", "", "Output file path (default: auto-generated)")
cmd.Flags().Bool("no-progress", false, "Disable the interactive progress bar (logs go straight to stderr)")
cmd.Flags().Bool("json", false, "Output JSON Lines to stdout (for programmatic consumers)")
```

- [ ] **Step 3.3: Run the flag test to confirm it passes**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
go test ./cli/... -run "backtest.*json" -v 2>&1
```

Expected: PASS.

- [ ] **Step 3.4: Wire the reporter into `runBacktest`**

Replace the body of `runBacktest` in `cli/backtest.go` with the version below. The changes are:
1. Read `--json` flag after `cash`
2. Configure zerolog for JSON mode before the first `log.Info()` call
3. Create reporter and emit `started` before the engine runs
4. `useProgress` now also checks `!jsonMode`
5. Add JSON progress callback block after the TUI block
6. Emit `reporter.Error` on engine failure
7. Replace `summary.Render()` with `reporter.Completed()` in JSON mode

```go
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
```

- [ ] **Step 3.5: Confirm the full CLI test suite passes**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
go test ./cli/... -v 2>&1 | tail -30
```

Expected: all specs PASS, no compile errors.

- [ ] **Step 3.6: Commit**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
git add cli/backtest.go
git commit -m "feat(cli): add --json flag to backtest for JSON Lines output"
```
