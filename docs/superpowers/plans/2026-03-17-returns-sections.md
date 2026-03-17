# Returns Sections Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single "Trailing Returns" table with "Recent Returns" (1D, 1W, 1M, WTD, MTD, YTD -- non-annualized TWRR) and "Returns" (1Y, 3Y, 5Y, 10Y, Since Inception -- annualized TWRR), with wider columns.

**Architecture:** The `TrailingReturns` struct becomes a generic `ReturnTable` used by both sections. Two new builder functions in `report.go` produce the tables. The renderer gets a shared `renderReturnTable` helper called with different section titles. Column width increases from 12 to 16.

**Tech Stack:** Go, lipgloss (terminal styling), Ginkgo/Gomega (engine/portfolio tests), standard testing (renderer tests)

**Spec:** `docs/superpowers/specs/2026-03-17-returns-sections-design.md`

---

### File Map

| File | Action | Responsibility |
|---|---|---|
| `report/report.go` | Modify | Replace `TrailingReturns` struct with `ReturnTable`, replace `buildTrailingReturns` with `buildRecentReturns` + `buildReturns` |
| `report/terminal/returns.go` | Modify | Replace `renderTrailingReturns` with `renderReturnTable` helper, update `colWidth` |
| `report/terminal/renderer.go` | Modify | Call new render functions |
| `report/terminal/renderer_test.go` | Modify | Update fixtures and section assertions |
| `report/report_test.go` | Create | Test annualization math and N/A detection |

---

### Task 1: Replace TrailingReturns with ReturnTable and update Report struct

**Files:**
- Modify: `report/report.go:82-87` (TrailingReturns struct)
- Modify: `report/report.go:45` (Report struct field)
- Modify: `report/terminal/renderer.go:44` (render call)
- Modify: `report/terminal/returns.go:26,29-76` (colWidth, renderTrailingReturns)
- Modify: `report/terminal/renderer_test.go:76-80,169-182` (fixtures, assertions)

- [ ] **Step 1: Replace the struct and update all references**

In `report/report.go`, replace lines 82-87:
```go
// ReturnTable holds return figures for named periods.
type ReturnTable struct {
	Periods   []string
	Strategy  []float64
	Benchmark []float64
}
```

Update the `Report` struct (line 45), replacing `TrailingReturns TrailingReturns` with:
```go
	RecentReturns ReturnTable
	Returns       ReturnTable
```

- [ ] **Step 2: Update the renderer to use ReturnTable**

In `report/terminal/returns.go`, change `colWidth` (line 26) from 12 to 16:
```go
const colWidth = 16
```

Replace `renderTrailingReturns` (lines 29-76) with a shared helper and two thin wrappers:
```go
func renderRecentReturns(builder *strings.Builder, table report.ReturnTable, hasBenchmark bool) {
	renderReturnTable("Recent Returns", builder, table, hasBenchmark)
}

func renderReturns(builder *strings.Builder, table report.ReturnTable, hasBenchmark bool) {
	renderReturnTable("Returns", builder, table, hasBenchmark)
}

func renderReturnTable(title string, builder *strings.Builder, table report.ReturnTable, hasBenchmark bool) {
	if len(table.Periods) == 0 {
		return
	}

	builder.WriteString(sectionTitleStyle.Render(title))
	builder.WriteString("\n")

	// Header row.
	header := padRight(labelStyle.Render(""), colWidth)
	for _, period := range table.Periods {
		header += padLeft(tableHeaderStyle.Render(period), colWidth)
	}

	builder.WriteString("  " + header + "\n")

	// Strategy row.
	stratRow := padRight(labelStyle.Render("Strategy"), colWidth)
	for _, val := range table.Strategy {
		stratRow += padLeft(fmtPct(val), colWidth)
	}

	builder.WriteString("  " + stratRow + "\n")

	// Benchmark row (if present).
	if hasBenchmark {
		benchRow := padRight(labelStyle.Render("Benchmark"), colWidth)
		for _, val := range table.Benchmark {
			benchRow += padLeft(fmtPct(val), colWidth)
		}

		builder.WriteString("  " + benchRow + "\n")

		// Diff row.
		diffRow := padRight(labelStyle.Render("+/-"), colWidth)

		for idx := range table.Strategy {
			diff := table.Strategy[idx] - table.Benchmark[idx]
			if math.IsNaN(table.Strategy[idx]) || math.IsNaN(table.Benchmark[idx]) {
				diff = math.NaN()
			}

			diffRow += padLeft(fmtPctDiff(diff), colWidth)
		}

		builder.WriteString("  " + diffRow + "\n")
	}
}
```

In `report/terminal/renderer.go`, replace line 44:
```go
	renderTrailingReturns(&builder, rpt.TrailingReturns, rpt.HasBenchmark)
```
with:
```go
	renderRecentReturns(&builder, rpt.RecentReturns, rpt.HasBenchmark)
	renderReturns(&builder, rpt.Returns, rpt.HasBenchmark)
```

- [ ] **Step 3: Update the renderer test**

In `report/terminal/renderer_test.go`, replace lines 76-80:
```go
		RecentReturns: report.ReturnTable{
			Periods:   []string{"1D", "1W", "1M", "WTD", "MTD", "YTD"},
			Strategy:  []float64{0.001, 0.005, 0.01, 0.008, 0.009, 0.10},
			Benchmark: []float64{0.0005, 0.003, 0.005, 0.004, 0.005, 0.08},
		},
		Returns: report.ReturnTable{
			Periods:   []string{"1Y", "3Y", "5Y", "10Y", "Since Inception"},
			Strategy:  []float64{0.20, math.NaN(), math.NaN(), math.NaN(), 0.20},
			Benchmark: []float64{0.10, math.NaN(), math.NaN(), math.NaN(), 0.10},
		},
```

Add `"math"` to the imports.

Replace lines 169-182, changing `"Trailing Returns"` to `"Recent Returns"` and `"Returns"`:
```go
	for _, section := range []string{
		"Performance",
		"Recent Returns",
		"Returns",
		"Annual Returns",
		"Risk Metrics",
		"Risk vs Benchmark",
		"Top Drawdowns",
		"Monthly Returns",
		"Trade Summary",
	} {
```

- [ ] **Step 4: Verify compilation and tests**

Run: `go build github.com/penny-vault/pvbt/report/...`
Expected: compiles (buildTrailingReturns still exists but is now unused -- will be removed in Task 2)

Run: `go test github.com/penny-vault/pvbt/report/terminal -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add report/report.go report/terminal/returns.go report/terminal/renderer.go report/terminal/renderer_test.go
git commit -m "refactor: replace TrailingReturns with ReturnTable, widen columns to 16"
```

---

### Task 2: Implement buildRecentReturns and buildReturns

**Files:**
- Modify: `report/report.go:285-333` (replace buildTrailingReturns)
- Modify: `report/report.go:224` (Build function call)
- Create: `report/report_test.go`

- [ ] **Step 1: Write the test for buildRecentReturns and buildReturns**

Create `report/report_test.go`. This tests the annualization math and N/A
detection. The builder functions are not exported, so test through `Build()`.
However, since `Build()` requires a full `portfolio.Portfolio`, test the
annualization helper directly. Extract it as a package-level function
`annualizeTWRR(twrr float64, years float64) float64`.

```go
package report

import (
	"math"
	"testing"
)

func TestAnnualizeTWRR(t *testing.T) {
	tests := []struct {
		name     string
		twrr     float64
		years    float64
		expected float64
	}{
		{
			name:     "10% over 2 years",
			twrr:     0.10,
			years:    2.0,
			expected: math.Pow(1.10, 1.0/2.0) - 1, // ~4.88%
		},
		{
			name:     "100% over 5 years",
			twrr:     1.0,
			years:    5.0,
			expected: math.Pow(2.0, 1.0/5.0) - 1, // ~14.87%
		},
		{
			name:     "exactly 1 year is identity",
			twrr:     0.25,
			years:    1.0,
			expected: 0.25,
		},
		{
			name:     "negative return",
			twrr:     -0.20,
			years:    3.0,
			expected: math.Pow(0.80, 1.0/3.0) - 1,
		},
		{
			name:     "NaN input produces NaN",
			twrr:     math.NaN(),
			years:    1.0,
			expected: math.NaN(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := annualizeTWRR(tc.twrr, tc.years)
			if math.IsNaN(tc.expected) {
				if !math.IsNaN(got) {
					t.Errorf("expected NaN, got %f", got)
				}

				return
			}

			if math.Abs(got-tc.expected) > 1e-10 {
				t.Errorf("annualizeTWRR(%f, %f) = %f, want %f", tc.twrr, tc.years, got, tc.expected)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test github.com/penny-vault/pvbt/report -run TestAnnualizeTWRR -v`
Expected: FAIL with "undefined: annualizeTWRR"

- [ ] **Step 3: Implement annualizeTWRR and the two builder functions**

In `report/report.go`, add the helper function (near the metric helpers at the bottom):
```go
// annualizeTWRR converts a cumulative TWRR to an annualized rate.
// For periods under one year, returns the raw TWRR unchanged.
func annualizeTWRR(twrr float64, years float64) float64 {
	if math.IsNaN(twrr) || years <= 0 {
		return math.NaN()
	}

	return math.Pow(1+twrr, 1.0/years) - 1
}
```

Replace `buildTrailingReturns` (lines 285-333) with:
```go
func buildRecentReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) ReturnTable {
	oneDay := portfolio.Days(1)
	oneWeek := portfolio.Days(7)
	oneMonth := portfolio.Months(1)
	wtd := portfolio.WTD()
	mtd := portfolio.MTD()
	ytd := portfolio.YTD()

	type periodDef struct {
		label  string
		window portfolio.Period
	}

	defs := []periodDef{
		{"1D", oneDay},
		{"1W", oneWeek},
		{"1M", oneMonth},
		{"WTD", wtd},
		{"MTD", mtd},
		{"YTD", ytd},
	}

	result := ReturnTable{
		Periods:   make([]string, len(defs)),
		Strategy:  make([]float64, len(defs)),
		Benchmark: make([]float64, len(defs)),
	}

	for idx, def := range defs {
		result.Periods[idx] = def.label
		result.Strategy[idx] = metricValWindow(acct, portfolio.TWRR, def.window, warnings)

		if hasBenchmark {
			result.Benchmark[idx] = metricValBenchmarkWindow(acct, portfolio.TWRR, def.window, warnings)
		} else {
			result.Benchmark[idx] = math.NaN()
		}
	}

	return result
}

func buildReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) ReturnTable {
	pd := acct.PerfData()

	var backtestStart, backtestEnd time.Time

	if pd != nil && pd.Len() > 0 {
		backtestStart = pd.Start()
		backtestEnd = pd.End()
	}

	backtestYears := backtestEnd.Sub(backtestStart).Hours() / 24 / 365.25

	type periodDef struct {
		label        string
		window       *portfolio.Period // nil = Since Inception
		nominalYears float64           // 0 = use actual duration
	}

	oneYear := portfolio.Years(1)
	threeYears := portfolio.Years(3)
	fiveYears := portfolio.Years(5)
	tenYears := portfolio.Years(10)

	defs := []periodDef{
		{"1Y", &oneYear, 1},
		{"3Y", &threeYears, 3},
		{"5Y", &fiveYears, 5},
		{"10Y", &tenYears, 10},
		{"Since Inception", nil, 0},
	}

	result := ReturnTable{
		Periods:   make([]string, len(defs)),
		Strategy:  make([]float64, len(defs)),
		Benchmark: make([]float64, len(defs)),
	}

	for idx, def := range defs {
		result.Periods[idx] = def.label

		// N/A detection: check if the backtest covers the requested period.
		if def.window != nil {
			windowStart := def.window.Before(backtestEnd)
			if windowStart.Before(backtestStart) {
				result.Strategy[idx] = math.NaN()
				result.Benchmark[idx] = math.NaN()

				continue
			}
		}

		// Compute TWRR.
		var stratTWRR, benchTWRR float64

		if def.window != nil {
			stratTWRR = metricValWindow(acct, portfolio.TWRR, *def.window, warnings)
		} else {
			stratTWRR = metricVal(acct, portfolio.TWRR, warnings)
		}

		if hasBenchmark {
			if def.window != nil {
				benchTWRR = metricValBenchmarkWindow(acct, portfolio.TWRR, *def.window, warnings)
			} else {
				benchTWRR = metricValBenchmark(acct, portfolio.TWRR, warnings)
			}
		} else {
			benchTWRR = math.NaN()
		}

		// Annualize.
		years := def.nominalYears
		if years == 0 {
			years = backtestYears
		}

		// For backtests shorter than 1 year, Since Inception shows raw TWRR.
		if def.window == nil && backtestYears < 1.0 {
			result.Strategy[idx] = stratTWRR
			result.Benchmark[idx] = benchTWRR
		} else {
			result.Strategy[idx] = annualizeTWRR(stratTWRR, years)
			result.Benchmark[idx] = annualizeTWRR(benchTWRR, years)
		}
	}

	return result
}
```

Update the `Build` function (line 224), replacing:
```go
	report.TrailingReturns = buildTrailingReturns(acct, hasBenchmark, &warnings)
```
with:
```go
	report.RecentReturns = buildRecentReturns(acct, hasBenchmark, &warnings)
	report.Returns = buildReturns(acct, hasBenchmark, &warnings)
```

Delete the old `buildTrailingReturns` function entirely.

- [ ] **Step 4: Run tests**

Run: `go test github.com/penny-vault/pvbt/report -run TestAnnualizeTWRR -v`
Expected: PASS

Run: `go test github.com/penny-vault/pvbt/report/terminal -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./report/... --fix`
Expected: 0 issues

- [ ] **Step 6: Commit**

```
git add report/report.go report/report_test.go
git commit -m "feat: split trailing returns into recent returns and annualized returns"
```

---

### Task 3: Build and verify end-to-end

- [ ] **Step 1: Build the momentum-rotation example and check output**

```
cd examples/momentum-rotation && go build -o /tmp/momrot .
/tmp/momrot backtest --start 2020-01-01 --end 2025-01-01 2>&1 | head -60
```

Verify:
- "Recent Returns" section appears with 1D, 1W, 1M, WTD, MTD, YTD columns
- "Returns" section appears with 1Y, 3Y, 5Y, 10Y, Since Inception columns
- Columns are visibly wider (16 chars)
- N/A appears for periods exceeding backtest duration
- No "Trailing Returns" section

- [ ] **Step 2: Run the full test suite**

```
go test ./report/... ./cli/... ./engine/... -count=1
```

Expected: all pass

- [ ] **Step 3: Run linter on all changed packages**

```
golangci-lint run ./report/... --fix
```

Expected: 0 issues
