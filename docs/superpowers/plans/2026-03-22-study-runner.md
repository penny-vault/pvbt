# Study Runner Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a study runner that orchestrates multiple engine runs with parameter sweeps, refactor the report package into compositional primitives, and implement the stress test study as the first concrete study type.

**Architecture:** Three independent subsystems built in sequence: (1) report primitives replace the monolithic Report struct with a Section interface and concrete types (Table, TimeSeries, MetricPairs, Text); (2) study package provides the Runner, Study interface, and parameter sweep cross-product; (3) stress test study slices a single portfolio's equity curve into named historical scenario windows. The engine gains an exported ApplyParams function that wraps existing reflection-based parameter and preset application.

**Tech Stack:** Go, Ginkgo v2 (tests), lipgloss (terminal rendering), cobra (CLI), zerolog (logging)

**Spec:** `docs/superpowers/specs/2026-03-22-study-runner-design.md`

---

### Task 1: Report primitives -- Format, Section interface, and concrete types

**Files:**
- Create: `report/format.go`
- Create: `report/section.go`
- Create: `report/table.go`
- Create: `report/time_series.go`
- Create: `report/metric_pairs.go`
- Create: `report/text.go`
- Create: `report/report_suite_test.go`
- Create: `report/section_test.go`

- [ ] **Step 1: Create Ginkgo test suite for report package**

The existing report tests use standard Go tests. Create `report/report_suite_test.go` to enable Ginkgo tests alongside:

```go
package report_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestReportSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = log.Output(GinkgoWriter)
	RunSpecs(t, "Report Suite")
}
```

- [ ] **Step 2: Write tests for Table rendering**

Create `report/section_test.go` with Ginkgo tests:

```go
package report_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/report"
)

var _ = Describe("Table", func() {
	It("renders text with aligned columns", func() {
		table := report.Table{
			SectionName: "Test Table",
			Columns: []report.Column{
				{Header: "Name", Format: "string", Align: "left"},
				{Header: "Value", Format: "number", Align: "right"},
			},
			Rows: [][]any{
				{"Alpha", 1.5},
				{"Beta", 2.3},
			},
		}

		var buf bytes.Buffer
		Expect(table.Render(report.FormatText, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("Name"))
		Expect(buf.String()).To(ContainSubstring("Alpha"))
	})

	It("returns type discriminator", func() {
		table := report.Table{SectionName: "T"}
		Expect(table.Type()).To(Equal("table"))
	})

	It("returns section name", func() {
		table := report.Table{SectionName: "My Table"}
		Expect(table.Name()).To(Equal("My Table"))
	})

	It("renders JSON with type discriminator", func() {
		table := report.Table{
			SectionName: "Test",
			Columns: []report.Column{
				{Header: "X", Format: "number", Align: "right"},
			},
			Rows: [][]any{{42}},
		}

		var buf bytes.Buffer
		Expect(table.Render(report.FormatJSON, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring(`"type":"table"`))
		Expect(buf.String()).To(ContainSubstring(`"name":"Test"`))
	})
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./report/ -run "Table" -v`
Expected: compilation failure -- types not defined

- [ ] **Step 4: Create format.go**

Create `report/format.go`:

```go
package report

// Format specifies the output format for report rendering.
type Format string

const (
	FormatText Format = "text"
	FormatHTML Format = "html"
	FormatJSON Format = "json"
)
```

- [ ] **Step 5: Create section.go**

Create `report/section.go`:

```go
package report

import "io"

// Section is a self-contained renderable unit within a report.
type Section interface {
	Type() string
	Name() string
	Render(format Format, w io.Writer) error
}
```

- [ ] **Step 6: Create table.go**

Create `report/table.go` implementing the Table section type with `Type()`, `Name()`, and `Render()` for FormatText and FormatJSON. FormatHTML can return an unsupported format error for now.

The Table struct:

```go
package report

type Table struct {
	SectionName string
	Columns     []Column
	Rows        [][]any
}

type Column struct {
	Header string
	Format string // "percent", "currency", "number", "string", "date"
	Align  string // "left", "right", "center"
}
```

`Render(FormatText, w)` should produce a simple text table with column headers and aligned rows. `Render(FormatJSON, w)` should emit `{"type":"table","name":"...","columns":[...],"rows":[...]}`.

- [ ] **Step 7: Run Table tests**

Run: `go test ./report/ -run "Table" -v`
Expected: PASS

- [ ] **Step 8: Write tests for TimeSeries, MetricPairs, Text**

Add to `report/section_test.go` (add `"time"` to the import block at the top of the file):

```go
var _ = Describe("TimeSeries", func() {
	It("returns type and name", func() {
		ts := report.TimeSeries{SectionName: "Equity Curve"}
		Expect(ts.Type()).To(Equal("time_series"))
		Expect(ts.Name()).To(Equal("Equity Curve"))
	})

	It("renders JSON with series data", func() {
		ts := report.TimeSeries{
			SectionName: "Curve",
			Series: []report.NamedSeries{
				{Name: "Strategy", Times: []time.Time{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}, Values: []float64{100.0}},
			},
		}

		var buf bytes.Buffer
		Expect(ts.Render(report.FormatJSON, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring(`"type":"time_series"`))
	})
})

var _ = Describe("MetricPairs", func() {
	It("returns type and name", func() {
		mp := report.MetricPairs{SectionName: "Risk"}
		Expect(mp.Type()).To(Equal("metric_pairs"))
		Expect(mp.Name()).To(Equal("Risk"))
	})

	It("handles nil comparison pointer", func() {
		mp := report.MetricPairs{
			SectionName: "Metrics",
			Metrics: []report.MetricPair{
				{Label: "Sharpe", Value: 1.5, Comparison: nil, Format: "ratio"},
			},
		}

		var buf bytes.Buffer
		Expect(mp.Render(report.FormatText, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("Sharpe"))
		Expect(buf.String()).NotTo(ContainSubstring("vs"))
	})

	It("renders comparison when present", func() {
		benchVal := 0.8
		mp := report.MetricPairs{
			SectionName: "Metrics",
			Metrics: []report.MetricPair{
				{Label: "Sharpe", Value: 1.5, Comparison: &benchVal, Format: "ratio"},
			},
		}

		var buf bytes.Buffer
		Expect(mp.Render(report.FormatText, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("1.5"))
		Expect(buf.String()).To(ContainSubstring("0.8"))
	})
})

var _ = Describe("Text", func() {
	It("returns type and name", func() {
		txt := report.Text{SectionName: "Warnings", Body: "Watch out"}
		Expect(txt.Type()).To(Equal("text"))
		Expect(txt.Name()).To(Equal("Warnings"))
	})

	It("renders body as-is for text format", func() {
		txt := report.Text{SectionName: "Notes", Body: "All good"}
		var buf bytes.Buffer
		Expect(txt.Render(report.FormatText, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("All good"))
	})
})
```

- [ ] **Step 9: Implement TimeSeries, MetricPairs, Text**

Create `report/time_series.go`:

```go
type TimeSeries struct {
	SectionName string
	Series      []NamedSeries
}

type NamedSeries struct {
	Name   string
	Times  []time.Time
	Values []float64
}
```

Create `report/metric_pairs.go`:

```go
type MetricPairs struct {
	SectionName string
	Metrics     []MetricPair
}

type MetricPair struct {
	Label      string
	Value      float64
	Comparison *float64
	Format     string // "percent", "ratio", "days"
}
```

Create `report/text.go`:

```go
type Text struct {
	SectionName string
	Body        string
}
```

Each implements `Type()`, `Name()`, and `Render()` for FormatText and FormatJSON.

- [ ] **Step 10: Run all section tests**

Run: `go test ./report/ -v`
Expected: PASS for all section tests

- [ ] **Step 11: Commit**

```bash
git add report/format.go report/section.go report/table.go report/time_series.go report/metric_pairs.go report/text.go report/report_suite_test.go report/section_test.go
git commit -m "feat: add compositional report primitives (Table, TimeSeries, MetricPairs, Text)"
```

---

### Task 2: New Report struct with Render method

**Files:**
- Create: `report/composable_report.go`
- Modify: `report/section_test.go`

- [ ] **Step 1: Write test for Report.Render**

Add to `report/section_test.go`:

```go
var _ = Describe("Report", func() {
	It("renders all sections in order for text format", func() {
		rpt := report.ComposableReport{
			Title: "Test Report",
			Sections: []report.Section{
				&report.Text{SectionName: "Intro", Body: "Hello"},
				&report.Table{
					SectionName: "Data",
					Columns:     []report.Column{{Header: "X", Format: "number", Align: "right"}},
					Rows:        [][]any{{1}},
				},
			},
		}

		var buf bytes.Buffer
		Expect(rpt.Render(report.FormatText, &buf)).To(Succeed())
		output := buf.String()
		introIdx := strings.Index(output, "Hello")
		dataIdx := strings.Index(output, "Data")
		Expect(introIdx).To(BeNumerically("<", dataIdx))
	})

	It("renders JSON with title and sections array", func() {
		rpt := report.ComposableReport{
			Title: "JSON Test",
			Sections: []report.Section{
				&report.Text{SectionName: "Note", Body: "test"},
			},
		}

		var buf bytes.Buffer
		Expect(rpt.Render(report.FormatJSON, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring(`"title":"JSON Test"`))
		Expect(buf.String()).To(ContainSubstring(`"sections":[`))
	})
})
```

Note: The new composable report type is named `ComposableReport` to coexist with the existing `Report` struct during migration. Once migration is complete (Task 3), the old `Report` is removed and `ComposableReport` is renamed to `Report`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./report/ -run "Report" -v`
Expected: compilation failure

- [ ] **Step 3: Implement ComposableReport**

Create `report/composable_report.go`:

```go
package report

import (
	"encoding/json"
	"fmt"
	"io"
)

// ComposableReport is a titled collection of renderable sections.
// Named ComposableReport during migration to coexist with the legacy Report struct.
type ComposableReport struct {
	Title    string
	Sections []Section
}

func (r ComposableReport) Render(format Format, w io.Writer) error {
	switch format {
	case FormatJSON:
		return r.renderJSON(w)
	case FormatText:
		return r.renderText(w)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func (r ComposableReport) renderJSON(w io.Writer) error {
	// Collect each section's JSON into a buffer, then assemble the wrapper.
	_, err := fmt.Fprintf(w, `{"title":%s,"sections":[`, jsonString(r.Title))
	if err != nil {
		return err
	}

	for idx, section := range r.Sections {
		if idx > 0 {
			if _, err := w.Write([]byte(",")); err != nil {
				return err
			}
		}

		if err := section.Render(FormatJSON, w); err != nil {
			return fmt.Errorf("rendering section %q as JSON: %w", section.Name(), err)
		}
	}

	_, err = w.Write([]byte("]}"))
	return err
}

// jsonString returns a JSON-encoded string value.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (r ComposableReport) renderText(w io.Writer) error {
	fmt.Fprintf(w, "\n  %s\n", r.Title)
	fmt.Fprintln(w, strings.Repeat("─", len(r.Title)+4))

	for _, section := range r.Sections {
		fmt.Fprintf(w, "\n")
		if err := section.Render(FormatText, w); err != nil {
			return fmt.Errorf("rendering section %q: %w", section.Name(), err)
		}
	}

	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./report/ -run "Report" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add report/composable_report.go report/section_test.go
git commit -m "feat: add ComposableReport with multi-format Render method"
```

---

### Task 3: Migrate report.Summary builder and terminal rendering

**Files:**
- Create: `report/summary.go`
- Create: `report/summary_test.go`
- Modify: `report/terminal/renderer.go`
- Modify: `report/terminal/header.go`
- Modify: `report/terminal/returns.go`
- Modify: `report/terminal/risk.go`
- Modify: `report/terminal/trades.go`
- Modify: `report/terminal/drawdowns.go`
- Modify: `report/terminal/monthly.go`
- Modify: `report/terminal/chart.go`
- Modify: `report/report.go:49-62` (remove old Report struct)
- Modify: `cli/backtest.go:186-193`

This is the largest task. It replaces the old `Report` struct and `Build()` function with `Summary()` returning a `ComposableReport`, and updates all terminal rendering to work with sections.

- [ ] **Step 1: Write test for Summary builder**

Create `report/summary_test.go` with a test that calls `report.Summary()` with a mock `ReportablePortfolio` and verifies the returned `ComposableReport` contains the expected section types in the correct order:

```go
var _ = Describe("Summary", func() {
	It("produces a report with all expected sections", func() {
		// Use a test portfolio with known data
		// Verify sections include: header MetricPairs, equity curve TimeSeries,
		// returns Tables, risk MetricPairs, drawdowns Table, trades MetricPairs, etc.
	})
})
```

The test should use the existing test helpers in the portfolio package to construct a portfolio with known transactions and equity curve data.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./report/ -run "Summary" -v`
Expected: compilation failure -- Summary not defined

- [ ] **Step 3: Implement Summary builder**

Create `report/summary.go`. This function replaces `Build()` in `report/report.go:178-259`. It reads the same data from `ReportablePortfolio` but constructs sections instead of filling struct fields:

```go
func Summary(acct ReportablePortfolio) (ComposableReport, error) {
	rpt := ComposableReport{
		Title: acct.GetMetadata(portfolio.MetaStrategyName),
	}

	// Header metrics
	rpt.Sections = append(rpt.Sections, buildHeaderMetrics(acct))

	// Equity curve
	rpt.Sections = append(rpt.Sections, buildEquityCurve(acct))

	// Returns tables
	// Risk metrics
	// Drawdowns
	// Monthly returns
	// Trade statistics
	// Warnings

	return rpt, nil
}
```

Each `build*` helper creates the appropriate section type (Table, TimeSeries, MetricPairs, or Text) from portfolio data. The logic mirrors what's currently in `report.go:178-259` and the `report*` helpers in the `terminal/` sub-package files.

- [ ] **Step 4: Run Summary test**

Run: `go test ./report/ -run "Summary" -v`
Expected: PASS

- [ ] **Step 5: Update terminal.Render to accept ComposableReport**

Modify `report/terminal/renderer.go:26` to accept `ComposableReport` instead of the old `Report`. The new `Render` function walks sections and renders each one using lipgloss styling. The existing per-section render functions (`renderHeader`, `renderEquityCurve`, etc.) become the text renderers inside each section type's `Render(FormatText, w)` method.

Approach: Move the lipgloss formatting logic from `terminal/header.go`, `terminal/returns.go`, etc. into the `Render(FormatText, w)` methods of each section type. The terminal renderer becomes a thin wrapper that calls `rpt.Render(FormatText, w)`.

Note: This moves the lipgloss dependency from the `terminal` sub-package into the `report` package. The `report` package will gain an import of `github.com/charmbracelet/lipgloss`. This is acceptable since the section types own their rendering. Alternatively, the text rendering logic could stay in `terminal` with each section type delegating to a registered text renderer -- but the simpler approach of self-contained sections is preferred.

- [ ] **Step 6: Update cli/backtest.go**

Change `cli/backtest.go:186-193` to use `Summary()` and the new rendering:

```go
rpt, err := backtestReport.Summary(acct)
if err != nil {
    log.Warn().Err(err).Msg("some report metrics failed")
}

if err := rpt.Render(backtestReport.FormatText, os.Stdout); err != nil {
    return fmt.Errorf("rendering report: %w", err)
}
```

- [ ] **Step 7: Remove old Report struct and Build function**

Remove the old `Report` struct (`report/report.go:49-62`) and `Build()` function (`report/report.go:178-259`). Remove all the sub-types that were fields of the old struct (Header, EquityCurve, ReturnTable, AnnualReturns, Risk, RiskVsBenchmark, Drawdowns, MonthlyReturns, Trades).

Rename `ComposableReport` to `Report` now that the old type is gone.

Update all references: `report/summary.go`, `report/composable_report.go` (renamed to `report/report.go` or merged), `report/section_test.go`, `report/summary_test.go`, `cli/backtest.go`.

- [ ] **Step 8: Run all tests**

Run: `go test ./... -v`
Expected: PASS. All existing report tests may need updating since the old struct is gone. Fix any breakage.

- [ ] **Step 9: Run linter**

Run: `golangci-lint run ./...`
Expected: PASS. Fix any issues.

- [ ] **Step 10: Commit**

```bash
git add report/ cli/backtest.go
git commit -m "refactor: replace monolithic Report struct with composable Section-based reports"
```

---

### Task 4: Export engine.ApplyParams

**Files:**
- Create: `engine/apply_params.go`
- Create: `engine/apply_params_test.go`

- [ ] **Step 1: Write tests for ApplyParams**

Create `engine/apply_params_test.go`:

```go
var _ = Describe("ApplyParams", func() {
	It("applies simple string parameter", func() {
		strategy := &testStrategy{RiskOn: "SPY"}
		eng := engine.New(strategy, engine.WithAssetProvider(mockProvider))
		Expect(engine.ApplyParams(eng, "", map[string]string{"riskOn": "QQQ"})).To(Succeed())
		Expect(strategy.RiskOn).To(Equal("QQQ"))
	})

	It("applies float64 parameter", func() {
		strategy := &testStrategy{Lookback: 10.0}
		eng := engine.New(strategy, engine.WithAssetProvider(mockProvider))
		Expect(engine.ApplyParams(eng, "", map[string]string{"lookback": "20.5"})).To(Succeed())
		Expect(strategy.Lookback).To(Equal(20.5))
	})

	It("resolves preset and applies all preset values", func() {
		strategy := &presetStrategy{}
		eng := engine.New(strategy, engine.WithAssetProvider(mockProvider))
		Expect(engine.ApplyParams(eng, "Classic", nil)).To(Succeed())
		Expect(strategy.RiskOn).To(Equal("VFINX,PRIDX"))
		Expect(strategy.RiskOff).To(Equal("VUSTX"))
	})

	It("explicit params override preset values", func() {
		strategy := &presetStrategy{}
		eng := engine.New(strategy, engine.WithAssetProvider(mockProvider))
		Expect(engine.ApplyParams(eng, "Classic", map[string]string{"riskOff": "BND"})).To(Succeed())
		Expect(strategy.RiskOn).To(Equal("VFINX,PRIDX"))
		Expect(strategy.RiskOff).To(Equal("BND"))
	})

	It("returns error for unknown preset", func() {
		strategy := &presetStrategy{}
		eng := engine.New(strategy, engine.WithAssetProvider(mockProvider))
		err := engine.ApplyParams(eng, "DoesNotExist", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown preset"))
	})

	It("returns error for non-Descriptor strategy with preset", func() {
		strategy := &simpleStrategy{}
		eng := engine.New(strategy, engine.WithAssetProvider(mockProvider))
		err := engine.ApplyParams(eng, "Classic", nil)
		Expect(err).To(HaveOccurred())
	})

	It("handles asset.Asset fields via engine asset provider", func() {
		// Test that asset fields are resolved through the engine
		// This requires a mock asset provider that returns known assets
	})
})
```

Use existing test strategy types from `engine/exports_test.go` or define new ones in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run "ApplyParams" -v`
Expected: compilation failure

- [ ] **Step 3: Implement ApplyParams**

Create `engine/apply_params.go`:

```go
package engine

import "fmt"

// ApplyParams resolves a named preset and applies parameter overrides to the
// engine's strategy. It takes an *Engine because resolving asset.Asset and
// universe.Universe fields requires the engine's asset provider.
//
// Preset values are resolved via DescribeStrategy().Suggestions. If the
// strategy does not implement Descriptor and a non-empty preset is given,
// an error is returned. Explicit params override preset values.
func ApplyParams(eng *Engine, preset string, params map[string]string) error {
	strategy := eng.strategy

	// Resolve preset values.
	presetParams := make(map[string]string)
	if preset != "" {
		info := DescribeStrategy(strategy)
		if info.Suggestions == nil {
			return fmt.Errorf("strategy %q does not support presets (does not implement Descriptor)", strategy.Name())
		}

		presetValues, ok := info.Suggestions[preset]
		if !ok {
			available := make([]string, 0, len(info.Suggestions))
			for name := range info.Suggestions {
				available = append(available, name)
			}
			sort.Strings(available)
			return fmt.Errorf("unknown preset %q (available: %v)", preset, available)
		}

		for paramName, value := range presetValues {
			presetParams[paramName] = value
		}
	}

	// Merge: explicit params override preset values.
	for paramName, value := range params {
		presetParams[paramName] = value
	}

	// Apply all merged params via applyParamValue (handles string, int, float64, bool, duration).
	for paramName, value := range presetParams {
		if err := applyParamValue(strategy, paramName, value); err != nil {
			return fmt.Errorf("applying param %s=%s: %w", paramName, value, err)
		}
	}

	// Re-hydrate asset and universe fields that may have been set as strings.
	if err := hydrateFields(eng, strategy); err != nil {
		return fmt.Errorf("hydrating fields after param application: %w", err)
	}

	return nil
}
```

Key insight: `applyParamValue` handles simple types (string, int, float64, bool, duration) and silently skips asset/universe fields. Then `hydrateFields` picks up any asset/universe fields that now have non-zero string values and resolves them via the engine's asset provider. This is the same two-step process the engine already uses internally.

- [ ] **Step 4: Run tests**

Run: `go test ./engine/ -run "ApplyParams" -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./engine/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add engine/apply_params.go engine/apply_params_test.go
git commit -m "feat: export engine.ApplyParams for preset resolution and parameter application"
```

---

### Task 5: Study package -- core types

**Depends on:** Task 3 must be fully complete (including the rename of `ComposableReport` to `Report` and removal of the old `Report` struct) before starting this task, since the Study interface and Result type reference `report.Report`.

**Files:**
- Create: `study/study.go`
- Create: `study/doc.go`
- Create: `study/study_suite_test.go`

- [ ] **Step 1: Create study package with core types**

Create `study/doc.go`:

```go
// Package study provides a framework for running a strategy multiple times
// with different configurations and synthesizing the results into a report.
// Parameter sweeps are cross-producted with study configurations to produce
// the run matrix. Results are collected and passed to a study-specific
// Analyze function that composes a report from report primitives.
package study
```

Create `study/study.go` with all core types:

```go
package study

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/report"
)

// RunStatus represents the state of a single run within a study.
type RunStatus int

const (
	RunStarted   RunStatus = iota
	RunCompleted
	RunFailed
)

// RunConfig fully specifies what the engine should do for a single run.
type RunConfig struct {
	Name     string
	Start    time.Time
	End      time.Time
	Deposit  float64
	Preset   string
	Params   map[string]string
	Metadata map[string]string
}

// RunResult pairs a config with its outcome.
type RunResult struct {
	Config    RunConfig
	Portfolio report.ReportablePortfolio
	Err       error
}

// Progress is sent on a channel as runs execute.
type Progress struct {
	RunName   string
	RunIndex  int
	TotalRuns int
	Status    RunStatus
	Err       error
}

// Result is sent on a channel when the study completes.
type Result struct {
	Runs   []RunResult
	Report report.Report
	Err    error
}

// Study is the interface that each study type implements.
type Study interface {
	Name() string
	Description() string
	Configurations(ctx context.Context) ([]RunConfig, error)
	Analyze(results []RunResult) (report.Report, error)
}
```

Note: `report.Report` here refers to `report.ComposableReport` (or `report.Report` after the rename in Task 3).

Create `study/study_suite_test.go`:

```go
package study_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestStudy(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = log.Output(GinkgoWriter)
	RunSpecs(t, "Study Suite")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./study/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add study/
git commit -m "feat: add study package with core types (RunConfig, RunResult, Study interface)"
```

---

### Task 6: Parameter sweep types and cross-product

**Files:**
- Create: `study/sweep.go`
- Create: `study/sweep_test.go`

- [ ] **Step 1: Write tests for sweep constructors and cross-product**

Create `study/sweep_test.go`:

```go
var _ = Describe("ParamSweep", func() {
	Describe("SweepRange", func() {
		It("generates float values from min to max with step", func() {
			sweep := study.SweepRange("lookback", 1.0, 3.0, 1.0)
			Expect(sweep.Field()).To(Equal("lookback"))
			Expect(sweep.Values()).To(Equal([]string{"1", "2", "3"}))
		})

		It("generates int values", func() {
			sweep := study.SweepRange("window", 5, 15, 5)
			Expect(sweep.Values()).To(Equal([]string{"5", "10", "15"}))
		})

		It("handles non-divisible ranges by stopping before exceeding max", func() {
			sweep := study.SweepRange("x", 0.0, 1.0, 0.3)
			values := sweep.Values()
			Expect(len(values)).To(Equal(4)) // 0, 0.3, 0.6, 0.9
		})

		It("returns single value when min equals max", func() {
			sweep := study.SweepRange("x", 5.0, 5.0, 1.0)
			Expect(sweep.Values()).To(Equal([]string{"5"}))
		})

		It("returns empty when min exceeds max", func() {
			sweep := study.SweepRange("x", 10.0, 5.0, 1.0)
			Expect(sweep.Values()).To(BeEmpty())
		})
	})

	Describe("SweepDuration", func() {
		It("generates duration values", func() {
			sweep := study.SweepDuration("rebalance", 24*time.Hour, 72*time.Hour, 24*time.Hour)
			Expect(sweep.Values()).To(Equal([]string{"24h0m0s", "48h0m0s", "72h0m0s"}))
		})
	})

	Describe("SweepValues", func() {
		It("stores explicit string values", func() {
			sweep := study.SweepValues("universe", "SPY,TLT", "QQQ,SHY")
			Expect(sweep.Field()).To(Equal("universe"))
			Expect(sweep.Values()).To(Equal([]string{"SPY,TLT", "QQQ,SHY"}))
		})
	})

	Describe("SweepPresets", func() {
		It("stores preset names", func() {
			sweep := study.SweepPresets("Classic", "Modern")
			Expect(sweep.Field()).To(Equal(""))  // presets don't target a field
			Expect(sweep.Values()).To(Equal([]string{"Classic", "Modern"}))
			Expect(sweep.IsPreset()).To(BeTrue())
		})
	})
})

var _ = Describe("CrossProduct", func() {
	It("cross-products base configs with sweeps", func() {
		base := []study.RunConfig{
			{Name: "Base", Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)},
		}

		sweeps := []study.ParamSweep{
			study.SweepRange("lookback", 5.0, 10.0, 5.0),
			study.SweepValues("universe", "SPY", "QQQ"),
		}

		result := study.CrossProduct(base, sweeps)
		Expect(result).To(HaveLen(4)) // 1 base * 2 lookback * 2 universe

		// Verify each config has the right params
		names := make([]string, len(result))
		for idx, cfg := range result {
			names[idx] = cfg.Name
		}
		Expect(names).To(ContainElement(ContainSubstring("lookback=5")))
		Expect(names).To(ContainElement(ContainSubstring("universe=QQQ")))
	})

	It("handles preset sweeps by setting RunConfig.Preset", func() {
		base := []study.RunConfig{{Name: "Run"}}
		sweeps := []study.ParamSweep{study.SweepPresets("Classic", "Modern")}

		result := study.CrossProduct(base, sweeps)
		Expect(result).To(HaveLen(2))
		Expect(result[0].Preset).To(Equal("Classic"))
		Expect(result[1].Preset).To(Equal("Modern"))
	})

	It("returns base configs unchanged when no sweeps provided", func() {
		base := []study.RunConfig{{Name: "Only"}}
		result := study.CrossProduct(base, nil)
		Expect(result).To(HaveLen(1))
		Expect(result[0].Name).To(Equal("Only"))
	})

	It("sweep preset overrides study preset", func() {
		base := []study.RunConfig{{Name: "Run", Preset: "Original"}}
		sweeps := []study.ParamSweep{study.SweepPresets("Override")}

		result := study.CrossProduct(base, sweeps)
		Expect(result[0].Preset).To(Equal("Override"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./study/ -run "ParamSweep|CrossProduct" -v`
Expected: compilation failure

- [ ] **Step 3: Implement ParamSweep and constructors**

Create `study/sweep.go`:

```go
package study

import (
	"fmt"
	"time"
)

// Numeric constrains SweepRange to integer and floating-point types.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// ParamSweep describes how to vary a single strategy parameter across runs.
type ParamSweep struct {
	field    string
	values   []string
	isPreset bool
}

func (ps ParamSweep) Field() string    { return ps.field }
func (ps ParamSweep) Values() []string { return ps.values }
func (ps ParamSweep) IsPreset() bool   { return ps.isPreset }

// SweepRange generates values from min to max (inclusive) with the given step.
func SweepRange[T Numeric](field string, min, max, step T) ParamSweep {
	var values []string
	for val := min; val <= max; val += step {
		values = append(values, fmt.Sprintf("%v", val))
	}
	return ParamSweep{field: field, values: values}
}

// SweepDuration generates duration values from min to max with the given step.
func SweepDuration(field string, min, max, step time.Duration) ParamSweep {
	var values []string
	for val := min; val <= max; val += step {
		values = append(values, val.String())
	}
	return ParamSweep{field: field, values: values}
}

// SweepValues provides explicit string values for a field.
func SweepValues(field string, values ...string) ParamSweep {
	return ParamSweep{field: field, values: values}
}

// SweepPresets varies named parameter presets.
func SweepPresets(presets ...string) ParamSweep {
	return ParamSweep{field: "", values: presets, isPreset: true}
}

// CrossProduct combines base configs with sweeps to produce the full run matrix.
func CrossProduct(base []RunConfig, sweeps []ParamSweep) []RunConfig {
	if len(sweeps) == 0 {
		return base
	}

	result := make([]RunConfig, len(base))
	copy(result, base)

	for _, sweep := range sweeps {
		var expanded []RunConfig
		for _, cfg := range result {
			for _, val := range sweep.Values() {
				newCfg := cloneRunConfig(cfg)
				if sweep.IsPreset() {
					newCfg.Preset = val
					newCfg.Name = appendName(newCfg.Name, val)
				} else {
					if newCfg.Params == nil {
						newCfg.Params = make(map[string]string)
					}
					newCfg.Params[sweep.Field()] = val
					newCfg.Name = appendName(newCfg.Name, fmt.Sprintf("%s=%s", sweep.Field(), val))
				}
				expanded = append(expanded, newCfg)
			}
		}
		result = expanded
	}

	return result
}
```

Include helper functions `cloneRunConfig` (deep copy of maps) and `appendName` (joins with " / ").

- [ ] **Step 4: Run tests**

Run: `go test ./study/ -run "ParamSweep|CrossProduct" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add study/sweep.go study/sweep_test.go
git commit -m "feat: add parameter sweep types and cross-product logic"
```

---

### Task 7: Runner implementation

**Files:**
- Create: `study/runner.go`
- Create: `study/runner_test.go`

- [ ] **Step 1: Write tests for Runner**

Create `study/runner_test.go` with tests using a mock Study implementation:

```go
// mockStudy implements Study for testing.
type mockStudy struct {
	configs []study.RunConfig
	analyzeFn func([]study.RunResult) (report.Report, error)
}

func (m *mockStudy) Name() string        { return "mock" }
func (m *mockStudy) Description() string { return "mock study" }
func (m *mockStudy) Configurations(ctx context.Context) ([]study.RunConfig, error) {
	return m.configs, nil
}
func (m *mockStudy) Analyze(results []study.RunResult) (report.Report, error) {
	return m.analyzeFn(results)
}
```

Tests:

```go
var _ = Describe("Runner", func() {
	It("executes all configurations and returns results", func() {
		// Create a mock study with 2 configs
		// Create a simple strategy factory
		// Run with 2 workers
		// Verify: progress channel receives started/completed for each run
		// Verify: result contains both RunResults
		// Verify: result.Report is populated from Analyze
	})

	It("cross-products sweeps with study configurations", func() {
		// 1 base config + 2-value sweep = 2 runs
	})

	It("returns error synchronously when Configurations fails", func() {
		// Study whose Configurations returns error
		// Verify: Run returns nil channels and error
	})

	It("records individual run failures without aborting other runs", func() {
		// Strategy factory that fails for specific params
		// Verify: failed run has Err set, other runs succeed
	})

	It("respects context cancellation", func() {
		// Cancel context mid-run
		// Verify: remaining runs are not started or are cancelled
	})

	It("sends progress updates in order", func() {
		// Verify progress channel receives TotalRuns matching config count
		// Verify each run gets a started and completed/failed status
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./study/ -run "Runner" -v`
Expected: compilation failure

- [ ] **Step 3: Implement Runner**

Create `study/runner.go`:

```go
package study

import (
	"context"
	"fmt"
	"sync"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
	"github.com/rs/zerolog"
)

// Runner holds study configuration and executes the study.
type Runner struct {
	Study       Study
	NewStrategy func() engine.Strategy
	Options     []engine.Option
	Workers     int
	Sweeps      []ParamSweep
}

// Run executes the study and returns channels for progress and the final result.
func (r *Runner) Run(ctx context.Context) (<-chan Progress, <-chan Result, error) {
	configs, err := r.Study.Configurations(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("generating configurations: %w", err)
	}

	// Cross-product with sweeps.
	configs = CrossProduct(configs, r.Sweeps)

	workers := r.Workers
	if workers <= 0 {
		workers = 1
	}

	progressCh := make(chan Progress, len(configs))
	resultCh := make(chan Result, 1)

	go r.execute(ctx, configs, workers, progressCh, resultCh)

	return progressCh, resultCh, nil
}

func (r *Runner) execute(ctx context.Context, configs []RunConfig, workers int, progressCh chan<- Progress, resultCh chan<- Result) {
	defer close(progressCh)
	defer close(resultCh)

	results := make([]RunResult, len(configs))

	// Worker pool with semaphore.
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for idx, cfg := range configs {
		select {
		case <-ctx.Done():
			results[idx] = RunResult{Config: cfg, Err: ctx.Err()}
			progressCh <- Progress{RunName: cfg.Name, RunIndex: idx, TotalRuns: len(configs), Status: RunFailed, Err: ctx.Err()}
			continue
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(idx int, cfg RunConfig) {
			defer wg.Done()
			defer func() { <-sem }()

			progressCh <- Progress{RunName: cfg.Name, RunIndex: idx, TotalRuns: len(configs), Status: RunStarted}

			result := r.runSingle(ctx, cfg)
			results[idx] = result

			if result.Err != nil {
				progressCh <- Progress{RunName: cfg.Name, RunIndex: idx, TotalRuns: len(configs), Status: RunFailed, Err: result.Err}
			} else {
				progressCh <- Progress{RunName: cfg.Name, RunIndex: idx, TotalRuns: len(configs), Status: RunCompleted}
			}
		}(idx, cfg)
	}

	wg.Wait()

	// Analyze results.
	rpt, analyzeErr := r.Study.Analyze(results)
	resultCh <- Result{Runs: results, Report: rpt, Err: analyzeErr}
}

func (r *Runner) runSingle(ctx context.Context, cfg RunConfig) RunResult {
	strategy := r.NewStrategy()

	// Build engine options: base options + config-specific overrides.
	opts := make([]engine.Option, len(r.Options))
	copy(opts, r.Options)

	if cfg.Deposit > 0 {
		opts = append(opts, engine.WithInitialDeposit(cfg.Deposit))
	}

	eng := engine.New(strategy, opts...)

	// Apply preset and parameter overrides after engine construction
	// so that asset and universe fields can be resolved.
	if cfg.Preset != "" || len(cfg.Params) > 0 {
		if err := engine.ApplyParams(eng, cfg.Preset, cfg.Params); err != nil {
			return RunResult{Config: cfg, Err: fmt.Errorf("applying params: %w", err)}
		}
	}

	p, err := eng.Backtest(ctx, cfg.Start, cfg.End)
	if err != nil {
		return RunResult{Config: cfg, Err: err}
	}

	// Store metadata on portfolio.
	for key, val := range cfg.Metadata {
		p.SetMetadata(key, val)
	}

	// Type-assert to ReportablePortfolio.
	reportable, ok := p.(report.ReportablePortfolio)
	if !ok {
		return RunResult{Config: cfg, Err: fmt.Errorf("portfolio does not implement ReportablePortfolio")}
	}

	return RunResult{Config: cfg, Portfolio: reportable}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./study/ -run "Runner" -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./study/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add study/runner.go study/runner_test.go
git commit -m "feat: implement study Runner with concurrent worker pool"
```

---

### Task 8: Stress test study -- scenarios and configurations

**Files:**
- Create: `study/stress/stress.go`
- Create: `study/stress/scenarios.go`
- Create: `study/stress/stress_suite_test.go`
- Create: `study/stress/stress_test.go`

- [ ] **Step 1: Write tests for scenario definitions and Configurations**

Create `study/stress/stress_suite_test.go`:

```go
package stress_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestStress(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = log.Output(GinkgoWriter)
	RunSpecs(t, "Stress Suite")
}
```

Create `study/stress/stress_test.go`:

```go
var _ = Describe("StressTest", func() {
	Describe("Pre-defined scenarios", func() {
		It("includes all named scenarios", func() {
			scenarios := stress.DefaultScenarios()
			names := make([]string, len(scenarios))
			for idx, s := range scenarios {
				names[idx] = s.Name
			}
			Expect(names).To(ContainElement("2008 Financial Crisis"))
			Expect(names).To(ContainElement("COVID Crash"))
			Expect(names).To(ContainElement("2022 Rate Hiking Cycle"))
			Expect(names).To(ContainElement("Dot-com Bust"))
			Expect(names).To(ContainElement("2015-2017 Low-Volatility Grind"))
			Expect(names).To(ContainElement("2011 Debt Ceiling Crisis"))
		})
	})

	Describe("New", func() {
		It("defaults to all scenarios when nil", func() {
			st := stress.New(nil)
			Expect(st.Name()).To(Equal("Stress Test"))
			configs, err := st.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(1)) // single run covering all scenarios
		})

		It("uses provided scenarios", func() {
			custom := []stress.Scenario{
				{Name: "Custom", Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 6, 30, 0, 0, 0, 0, time.UTC)},
			}
			st := stress.New(custom)
			configs, err := st.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(1))
			Expect(configs[0].Start).To(Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))
			Expect(configs[0].End).To(Equal(time.Date(2020, 6, 30, 0, 0, 0, 0, time.UTC)))
		})
	})

	Describe("Configurations", func() {
		It("returns a single RunConfig spanning earliest to latest scenario", func() {
			scenarios := []stress.Scenario{
				{Name: "Early", Start: time.Date(2008, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2009, 3, 31, 0, 0, 0, 0, time.UTC)},
				{Name: "Late", Start: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 3, 31, 0, 0, 0, 0, time.UTC)},
			}
			st := stress.New(scenarios)
			configs, err := st.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs[0].Start).To(Equal(time.Date(2008, 9, 1, 0, 0, 0, 0, time.UTC)))
			Expect(configs[0].End).To(Equal(time.Date(2020, 3, 31, 0, 0, 0, 0, time.UTC)))
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./study/stress/ -v`
Expected: compilation failure

- [ ] **Step 3: Implement scenarios and StressTest type**

Create `study/stress/scenarios.go`:

```go
package stress

import "time"

// Scenario is a named historical market episode.
type Scenario struct {
	Name        string
	Description string
	Start       time.Time
	End         time.Time
}

// DefaultScenarios returns the pre-defined set of historical market scenarios.
func DefaultScenarios() []Scenario {
	return []Scenario{
		{
			Name:        "2008 Financial Crisis",
			Description: "Global financial crisis triggered by subprime mortgage collapse",
			Start:       time.Date(2008, 9, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2009, 3, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "COVID Crash",
			Description: "Rapid market decline due to COVID-19 pandemic",
			Start:       time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2020, 3, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2022 Rate Hiking Cycle",
			Description: "Federal Reserve aggressive rate hikes to combat inflation",
			Start:       time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2022, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "Dot-com Bust",
			Description: "Collapse of the dot-com bubble",
			Start:       time.Date(2000, 3, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2002, 10, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2015-2017 Low-Volatility Grind",
			Description: "Extended low-volatility period with no clear trends",
			Start:       time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2017, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:        "2011 Debt Ceiling Crisis",
			Description: "US debt ceiling standoff and S&P downgrade",
			Start:       time.Date(2011, 7, 1, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2011, 10, 31, 0, 0, 0, 0, time.UTC),
		},
	}
}
```

Create `study/stress/stress.go`:

```go
package stress

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

// StressTest runs a strategy against named historical market scenarios.
type StressTest struct {
	scenarios []Scenario
}

// New creates a stress test study. If scenarios is nil or empty,
// all default scenarios are used.
func New(scenarios []Scenario) *StressTest {
	if len(scenarios) == 0 {
		scenarios = DefaultScenarios()
	}
	return &StressTest{scenarios: scenarios}
}

func (st *StressTest) Name() string        { return "Stress Test" }
func (st *StressTest) Description() string { return "Run strategy against historical market stress scenarios" }

func (st *StressTest) Configurations(ctx context.Context) ([]study.RunConfig, error) {
	earliest := st.scenarios[0].Start
	latest := st.scenarios[0].End
	for _, s := range st.scenarios[1:] {
		if s.Start.Before(earliest) {
			earliest = s.Start
		}
		if s.End.After(latest) {
			latest = s.End
		}
	}

	return []study.RunConfig{
		{
			Name:  "Full Range",
			Start: earliest,
			End:   latest,
			Metadata: map[string]string{
				"study": "stress-test",
			},
		},
	}, nil
}

func (st *StressTest) Analyze(results []study.RunResult) (report.Report, error) {
	// Stub -- replaced with full implementation in Task 9 (analyze.go).
	return report.Report{Title: "Stress Test"}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./study/stress/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add study/stress/
git commit -m "feat: add stress test study type with named historical scenarios"
```

---

### Task 9: Stress test analysis

**Files:**
- Modify: `study/stress/stress.go`
- Create: `study/stress/analyze.go`
- Modify: `study/stress/stress_test.go`

- [ ] **Step 1: Write tests for Analyze**

Add to `study/stress/stress_test.go`:

```go
var _ = Describe("Analyze", func() {
	It("produces a report with scenario comparison table", func() {
		// Create a mock portfolio with known equity curve data
		// covering multiple scenario windows.
		// Call Analyze and verify the report contains:
		// - A Table section named "Scenario Comparison"
		// - MetricPairs sections for each scenario
		// - A TimeSeries section with equity curves
		// - A Text section with summary
	})

	It("handles failed runs by including error in table", func() {
		results := []study.RunResult{
			{Config: study.RunConfig{Name: "Failed"}, Err: fmt.Errorf("engine error")},
		}
		st := stress.New(nil)
		rpt, err := st.Analyze(results)
		Expect(err).NotTo(HaveOccurred())
		// Verify the report includes the failed run with error text
	})

	It("computes per-scenario metrics from sliced equity curve", func() {
		// Portfolio with known drawdown in 2008 scenario window
		// Verify max drawdown, recovery time, worst day are computed correctly
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./study/stress/ -run "Analyze" -v`
Expected: failure

- [ ] **Step 3: Remove Analyze stub and implement full version**

First, remove the stub `Analyze` method from `study/stress/stress.go` (the one that returns an empty report). Then create `study/stress/analyze.go` with the real implementation:

```go
package stress

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

func (st *StressTest) Analyze(results []study.RunResult) (report.Report, error) {
	rpt := report.Report{
		Title: "Stress Test Results",
	}

	// Build scenario comparison table.
	comparisonTable := buildComparisonTable(st.scenarios, results)
	rpt.Sections = append(rpt.Sections, &comparisonTable)

	// Per-scenario equity curves.
	equityCurves := buildEquityCurves(st.scenarios, results)
	if equityCurves != nil {
		rpt.Sections = append(rpt.Sections, equityCurves)
	}

	// Per-scenario metric pairs.
	for _, scenario := range st.scenarios {
		metrics := buildScenarioMetrics(scenario, results)
		if metrics != nil {
			rpt.Sections = append(rpt.Sections, metrics)
		}
	}

	// Summary text.
	rpt.Sections = append(rpt.Sections, &report.Text{
		SectionName: "Summary",
		Body:        buildSummaryText(st.scenarios, results),
	})

	return rpt, nil
}
```

Implement helper functions:

- `buildComparisonTable`: Creates a Table with one row per scenario, columns for max drawdown, drawdown velocity, recovery time, worst day, worst week, return, benchmark return, Sharpe. Uses `DataFrame.Between()` to slice equity curve data for each scenario window.

- `buildEquityCurves`: Creates a TimeSeries with one NamedSeries per scenario, normalized to starting value of 1.0.

- `buildScenarioMetrics`: Creates a MetricPairs section for a single scenario with strategy vs benchmark comparison.

- `buildSummaryText`: Generates narrative text summarizing worst-case scenario, most resilient scenario, etc.

For each scenario window, use `portfolio.PerfDataView()` to get the equity curve DataFrame, then `df.Between(scenario.Start, scenario.End)` to slice it. Compute metrics directly from the sliced data:

- Max drawdown: walk the sliced equity values, track running peak, compute max drop
- Drawdown velocity: max drawdown / trading days from peak to trough
- Recovery time: use `df.Between(trough, runEnd)` on the full (unsliced) equity curve to track forward beyond the scenario window until value exceeds previous peak. The full equity data is available since the engine ran over the entire date range, not just the scenario window.
- Worst day/week: compute daily returns from sliced equity, find minimum
- Turnover: sum absolute trade values from transactions within window, divide by average portfolio value
- Relative performance: strategy return vs benchmark return over window
- Sharpe: compute from daily returns within window (annualized)

- [ ] **Step 4: Run tests**

Run: `go test ./study/stress/ -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./study/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add study/stress/
git commit -m "feat: implement stress test Analyze with per-scenario metrics and report composition"
```

---

### Task 10: CLI study subcommand

**Files:**
- Create: `cli/study.go`
- Create: `cli/study_test.go`
- Modify: `cli/run.go:41`

- [ ] **Step 1: Write tests for study CLI command**

Create `cli/study_test.go`:

```go
var _ = Describe("study command", func() {
	It("registers stress-test subcommand", func() {
		strategy := &testStrategy{}
		cmd := newStudyCmd(strategy)
		subCmds := cmd.Commands()
		names := make([]string, len(subCmds))
		for idx, sub := range subCmds {
			names[idx] = sub.Name()
		}
		Expect(names).To(ContainElement("stress-test"))
	})

	It("stress-test accepts scenario names as positional args", func() {
		strategy := &testStrategy{}
		cmd := newStudyCmd(strategy)
		stressCmd, _, err := cmd.Find([]string{"stress-test"})
		Expect(err).NotTo(HaveOccurred())
		Expect(stressCmd.Args).NotTo(BeNil())
	})

	It("stress-test accepts 'all' as scenario selector", func() {
		// Verify that "all" selects all default scenarios
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cli/ -run "study" -v`
Expected: compilation failure

- [ ] **Step 3: Implement study command**

Create `cli/study.go`:

```go
package cli

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/stress"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newStudyCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "study",
		Short: "Run analysis studies on the strategy",
	}

	cmd.AddCommand(newStressTestCmd(strategy))

	return cmd
}

func newStressTestCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stress-test [scenario-names...|all]",
		Short: "Run strategy against historical market stress scenarios",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStressTest(cmd, strategy, args)
		},
	}

	cmd.Flags().Int("workers", runtime.GOMAXPROCS(0), "Number of concurrent workers")
	cmd.Flags().String("format", "text", "Output format (text, json, html)")

	return cmd
}

func runStressTest(cmd *cobra.Command, strategy engine.Strategy, args []string) error {
	ctx := log.Logger.WithContext(context.Background())

	// Resolve scenarios from args.
	scenarios := resolveScenarios(args)

	// Build study.
	st := stress.New(scenarios)

	// Build data provider.
	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	opts := []engine.Option{
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
	}

	workers, _ := cmd.Flags().GetInt("workers")

	runner := &study.Runner{
		Study:       st,
		NewStrategy: strategyFactory(strategy),
		Options:     opts,
		Workers:     workers,
	}

	progressCh, resultCh, err := runner.Run(ctx)
	if err != nil {
		return err
	}

	// Drain progress channel (print simple progress for now;
	// Bubble Tea integration is a follow-up).
	for prog := range progressCh {
		switch prog.Status {
		case study.RunStarted:
			log.Info().Str("run", prog.RunName).Int("index", prog.RunIndex).Int("total", prog.TotalRuns).Msg("started")
		case study.RunCompleted:
			log.Info().Str("run", prog.RunName).Msg("completed")
		case study.RunFailed:
			log.Warn().Str("run", prog.RunName).Err(prog.Err).Msg("failed")
		}
	}

	result := <-resultCh
	if result.Err != nil {
		return fmt.Errorf("study analysis failed: %w", result.Err)
	}

	formatStr, _ := cmd.Flags().GetString("format")
	return result.Report.Render(report.Format(formatStr), os.Stdout)
}

// strategyFactory returns a function that creates fresh copies of the strategy
// by reflecting over the original and creating a new zero-value instance of the
// same concrete type. This works because strategy state is populated by
// Setup() and hydrateFields during engine initialization, not at construction.
func strategyFactory(original engine.Strategy) func() engine.Strategy {
	originalType := reflect.TypeOf(original)
	if originalType.Kind() == reflect.Ptr {
		originalType = originalType.Elem()
	}

	return func() engine.Strategy {
		return reflect.New(originalType).Interface().(engine.Strategy)
	}
}

func resolveScenarios(args []string) []stress.Scenario {
	if len(args) == 0 || (len(args) == 1 && args[0] == "all") {
		return nil // nil triggers default scenarios
	}

	defaults := stress.DefaultScenarios()
	byName := make(map[string]stress.Scenario)
	for _, scenario := range defaults {
		byName[scenario.Name] = scenario
	}

	var selected []stress.Scenario
	for _, name := range args {
		if scenario, ok := byName[name]; ok {
			selected = append(selected, scenario)
		}
	}

	return selected
}
```

- [ ] **Step 4: Register study command in cli/run.go**

Add to `cli/run.go:41` after the existing AddCommand calls:

```go
rootCmd.AddCommand(newStudyCmd(strategy))
```

- [ ] **Step 5: Run tests**

Run: `go test ./cli/ -run "study" -v`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Run linter**

Run: `golangci-lint run ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add cli/study.go cli/study_test.go cli/run.go
git commit -m "feat: add CLI study subcommand with stress-test"
```

---

### Task 11: Integration test and final verification

**Files:**
- Create: `study/integration_test.go`

- [ ] **Step 1: Write integration test**

Create `study/integration_test.go` that wires together the full flow:

```go
var _ = Describe("Integration", func() {
	It("runs a stress test study end-to-end", func() {
		// Use a simple test strategy
		// Use mock data providers
		// Run the full Runner flow with stress.New()
		// Verify: progress updates received
		// Verify: result has non-empty report
		// Verify: report renders without error
	})
})
```

- [ ] **Step 2: Run integration test**

Run: `go test ./study/ -run "Integration" -v`
Expected: PASS

- [ ] **Step 3: Run full test suite and linter**

Run: `go test ./... && golangci-lint run ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add study/integration_test.go
git commit -m "test: add end-to-end integration test for study runner"
```
