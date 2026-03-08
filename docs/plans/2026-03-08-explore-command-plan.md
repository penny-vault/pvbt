# Explore Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a standalone `pvbt explore` CLI tool for querying and visualizing data from the PVDataProvider, with table and graph output modes.

**Architecture:** A new `cli/explore.go` file adds `RunExplore()` which builds a standalone cobra command (no strategy required). It parses positional args (tickers, metrics), creates a PVDataProvider, fetches a DataFrame, and either prints it as a table or launches a bubbletea TUI with a line chart. A `data/metric_registry.go` file provides a name-to-Metric lookup map. A `cmd/explore/main.go` provides the binary entry point.

**Tech Stack:** cobra, viper, zerolog, lipgloss, bubbletea, ntcharts, pgx/v5

---

### Task 1: Add metric name registry to data package

**Files:**
- Create: `data/metric_registry.go`
- Test: `data/metric_registry_test.go`

**Step 1: Write the failing test**

Create `data/metric_registry_test.go`:

```go
package data_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("MetricRegistry", func() {
	Describe("MetricByName", func() {
		It("finds AdjClose by exact name", func() {
			m, ok := data.MetricByName("AdjClose")
			Expect(ok).To(BeTrue())
			Expect(m).To(Equal(data.AdjClose))
		})

		It("finds MetricClose by exact name", func() {
			m, ok := data.MetricByName("MetricClose")
			Expect(ok).To(BeTrue())
			Expect(m).To(Equal(data.MetricClose))
		})

		It("returns false for unknown name", func() {
			_, ok := data.MetricByName("NotAMetric")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("AllMetricNames", func() {
		It("returns a non-empty sorted list", func() {
			names := data.AllMetricNames()
			Expect(len(names)).To(BeNumerically(">", 10))
			// verify sorted
			for i := 1; i < len(names); i++ {
				Expect(names[i] > names[i-1]).To(BeTrue())
			}
		})

		It("contains known metrics", func() {
			names := data.AllMetricNames()
			Expect(names).To(ContainElement("AdjClose"))
			Expect(names).To(ContainElement("Volume"))
			Expect(names).To(ContainElement("PE"))
			Expect(names).To(ContainElement("Revenue"))
		})
	})
})
```

**Step 2: Run test to verify it fails**

Run: `ginkgo --focus "MetricRegistry" ./data/...`
Expected: FAIL -- `MetricByName` and `AllMetricNames` not defined

**Step 3: Write minimal implementation**

Create `data/metric_registry.go`:

```go
package data

import "sort"

// metricRegistry maps the string representation of each Metric constant
// back to its typed value. Used by CLI tools that accept metric names
// as user input.
var metricRegistry = map[string]Metric{
	// EOD
	"MetricOpen":  MetricOpen,
	"MetricHigh":  MetricHigh,
	"MetricLow":   MetricLow,
	"MetricClose": MetricClose,
	"AdjClose":    AdjClose,
	"Volume":      Volume,
	"Dividend":    Dividend,
	"SplitFactor": SplitFactor,

	// Live
	"Price": Price,
	"Bid":   Bid,
	"Ask":   Ask,

	// Valuation
	"MarketCap":       MarketCap,
	"EnterpriseValue": EnterpriseValue,
	"PE":              PE,
	"PB":              PB,
	"PS":              PS,
	"EVtoEBIT":        EVtoEBIT,
	"EVtoEBITDA":      EVtoEBITDA,
	"SP500":           SP500,

	// Income statement
	"Revenue":                             Revenue,
	"CostOfRevenue":                       CostOfRevenue,
	"GrossProfit":                         GrossProfit,
	"OperatingExpenses":                   OperatingExpenses,
	"OperatingIncome":                     OperatingIncome,
	"EBIT":                                EBIT,
	"EBITDA":                              EBITDA,
	"EBT":                                 EBT,
	"ConsolidatedIncome":                  ConsolidatedIncome,
	"NetIncome":                           NetIncome,
	"NetIncomeCommonStock":                NetIncomeCommonStock,
	"EarningsPerShare":                    EarningsPerShare,
	"EPSDiluted":                          EPSDiluted,
	"InterestExpense":                     InterestExpense,
	"IncomeTaxExpense":                    IncomeTaxExpense,
	"RandDExpenses":                       RandDExpenses,
	"SGAExpense":                          SGAExpense,
	"ShareBasedCompensation":              ShareBasedCompensation,
	"DividendsPerShare":                   DividendsPerShare,
	"NetLossIncomeDiscontinuedOperations": NetLossIncomeDiscontinuedOperations,
	"NetIncomeToNonControllingInterests":  NetIncomeToNonControllingInterests,
	"PreferredDividendsImpact":            PreferredDividendsImpact,

	// Balance sheet
	"TotalAssets":                         TotalAssets,
	"CurrentAssets":                       CurrentAssets,
	"AssetsNonCurrent":                    AssetsNonCurrent,
	"AverageAssets":                       AverageAssets,
	"CashAndEquivalents":                  CashAndEquivalents,
	"Inventory":                           Inventory,
	"Receivables":                         Receivables,
	"Investments":                         Investments,
	"InvestmentsCurrent":                  InvestmentsCurrent,
	"InvestmentsNonCurrent":               InvestmentsNonCur,
	"Intangibles":                         Intangibles,
	"PPENet":                              PPENet,
	"TaxAssets":                           TaxAssets,
	"TotalLiabilities":                    TotalLiabilities,
	"CurrentLiabilities":                  CurrentLiabilities,
	"LiabilitiesNonCurrent":               LiabilitiesNonCurrent,
	"TotalDebt":                           TotalDebt,
	"DebtCurrent":                         DebtCurrent,
	"DebtNonCurrent":                      DebtNonCurrent,
	"Payables":                            Payables,
	"DeferredRevenue":                     DeferredRevenue,
	"Deposits":                            Deposits,
	"TaxLiabilities":                      TaxLiabilities,
	"Equity":                              Equity,
	"EquityAvg":                           EquityAvg,
	"AccumulatedOtherComprehensiveIncome": AccumulatedOtherComprehensiveIncome,
	"AccumulatedRetainedEarningsDeficit":  AccumulatedRetainedEarningsDeficit,

	// Cash flow
	"FreeCashFlow":             FreeCashFlow,
	"NetCashFlow":              NetCashFlow,
	"NetCashFlowFromOperations": NetCashFlowFromOperations,
	"NetCashFlowFromInvesting":  NetCashFlowFromInvesting,
	"NetCashFlowFromFinancing":  NetCashFlowFromFinancing,
	"NetCashFlowBusiness":      NetCashFlowBusiness,
	"NetCashFlowCommon":        NetCashFlowCommon,
	"NetCashFlowDebt":          NetCashFlowDebt,
	"NetCashFlowDividend":      NetCashFlowDividend,
	"NetCashFlowInvest":        NetCashFlowInvest,
	"NetCashFlowFx":            NetCashFlowFx,
	"CapitalExpenditure":       CapitalExpenditure,
	"DepreciationAmortization": DepreciationAmortization,

	// Per-share and ratios
	"BookValue":                       BookValue,
	"FreeCashFlowPerShare":            FreeCashFlowPerShare,
	"SalesPerShare":                   SalesPerShare,
	"TangibleAssetsBookValuePerShare": TangibleAssetsBookValuePerShare,
	"ShareFactor":                     ShareFactor,
	"SharesBasic":                     SharesBasic,
	"WeightedAverageShares":           WeightedAverageShares,
	"WeightedAverageSharesDiluted":    WeightedAverageSharesDiluted,
	"FundamentalPrice":                FundamentalPrice,
	"PE1":                             PE1,
	"PS1":                             PS1,
	"FxUSD":                           FxUSD,

	// Margin and return ratios
	"GrossMargin":   GrossMargin,
	"EBITDAMargin":  EBITDAMargin,
	"ProfitMargin":  ProfitMargin,
	"ROA":           ROA,
	"ROE":           ROE,
	"ROIC":          ROIC,
	"ReturnOnSales": ReturnOnSales,
	"AssetTurnover": AssetTurnover,
	"CurrentRatio":  CurrentRatio,
	"DebtToEquity":  DebtToEquity,
	"DividendYield": DividendYield,
	"PayoutRatio":   PayoutRatio,

	// Invested capital
	"InvestedCapital":      InvestedCapital,
	"InvestedCapitalAvg":   InvestedCapitalAvg,
	"TangibleAssetValue":   TangibleAssetValue,
	"WorkingCapital":       WorkingCapital,
	"MarketCapFundamental": MarketCapFundamental,

	// Economic
	"Unemployment": Unemployment,
}

// MetricByName looks up a Metric by its constant name string.
// Returns the Metric and true if found, or zero value and false if not.
func MetricByName(name string) (Metric, bool) {
	m, ok := metricRegistry[name]
	return m, ok
}

// AllMetricNames returns all registered metric names in sorted order.
func AllMetricNames() []string {
	names := make([]string, 0, len(metricRegistry))
	for name := range metricRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
```

**Step 4: Run tests**

Run: `ginkgo --focus "MetricRegistry" ./data/...`
Expected: PASS

**Step 5: Commit**

```bash
git add data/metric_registry.go data/metric_registry_test.go
git commit -m "feat(data): add metric name registry for CLI lookup"
```

---

### Task 2: Create explore command with table output

**Files:**
- Create: `cli/explore.go`

**Step 1: Create cli/explore.go**

This file contains `RunExplore()` (standalone entry point), the cobra command, argument parsing, data fetching, and table display. It reuses `DataFrame.Table()` for the actual table rendering.

```go
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RunExplore is the entry point for the standalone explore tool.
// It does not require a strategy.
func RunExplore() {
	cmd := newExploreCmd()

	cmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	viper.BindPFlag("log-level", cmd.PersistentFlags().Lookup("log-level"))

	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		level, err := zerolog.ParseLevel(viper.GetString("log-level"))
		if err != nil {
			level = zerolog.InfoLevel
		}
		zerolog.SetGlobalLevel(level)
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			With().Timestamp().Logger()
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newExploreCmd() *cobra.Command {
	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)

	cmd := &cobra.Command{
		Use:   "explore <tickers> <metrics> [flags]",
		Short: "Query and visualize data from the PVDataProvider",
		Long: `Explore fetches data for the given tickers and metrics from the
PVDataProvider database and displays it as a table or graph.

  explore AAPL,MSFT AdjClose,Volume
  explore AAPL AdjClose --graph
  explore --list-metrics`,
		Args: func(cmd *cobra.Command, args []string) error {
			if listMetrics, _ := cmd.Flags().GetBool("list-metrics"); listMetrics {
				return nil
			}
			if len(args) < 2 {
				return fmt.Errorf("requires 2 arguments: <tickers> <metrics>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if listMetrics, _ := cmd.Flags().GetBool("list-metrics"); listMetrics {
				return runListMetrics()
			}
			return runExplore(args[0], args[1])
		},
	}

	cmd.Flags().String("start", oneYearAgo.Format("2006-01-02"), "Start date (YYYY-MM-DD)")
	cmd.Flags().String("end", now.Format("2006-01-02"), "End date (YYYY-MM-DD)")
	cmd.Flags().Bool("graph", false, "Show TUI graph instead of table")
	cmd.Flags().Bool("list-metrics", false, "List all available metric names and exit")

	viper.BindPFlags(cmd.Flags())

	return cmd
}

func runListMetrics() error {
	names := data.AllMetricNames()
	for _, name := range names {
		fmt.Println(name)
	}
	return nil
}

func runExplore(tickersArg, metricsArg string) error {
	ctx := context.Background()

	// parse args
	tickers := strings.Split(tickersArg, ",")
	metricNames := strings.Split(metricsArg, ",")

	// resolve metrics
	metrics := make([]data.Metric, 0, len(metricNames))
	for _, name := range metricNames {
		m, ok := data.MetricByName(strings.TrimSpace(name))
		if !ok {
			return fmt.Errorf("unknown metric %q (use --list-metrics to see available names)", name)
		}
		metrics = append(metrics, m)
	}

	// parse dates
	start, err := time.Parse("2006-01-02", viper.GetString("start"))
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}
	end, err := time.Parse("2006-01-02", viper.GetString("end"))
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	// create provider
	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}
	defer provider.Close()

	// resolve tickers to assets
	var assets []asset.Asset
	for _, ticker := range tickers {
		ticker = strings.TrimSpace(ticker)
		a, err := provider.LookupAsset(ctx, ticker)
		if err != nil {
			return fmt.Errorf("lookup ticker %q: %w", ticker, err)
		}
		assets = append(assets, a)
	}

	// fetch data
	req := data.DataRequest{
		Assets:  assets,
		Metrics: metrics,
		Start:   start,
		End:     end,
	}

	log.Info().
		Strs("tickers", tickers).
		Int("metrics", len(metrics)).
		Time("start", start).
		Time("end", end).
		Msg("fetching data")

	df, err := provider.Fetch(ctx, req)
	if err != nil {
		return fmt.Errorf("fetch data: %w", err)
	}

	if df.Len() == 0 {
		fmt.Println("No data returned.")
		return nil
	}

	// display
	if viper.GetBool("graph") {
		return runExploreGraph(df)
	}

	fmt.Print(df.Table())
	fmt.Printf("\n%d rows\n", df.Len())
	return nil
}
```

Note: This file references `asset.Asset` -- add `"github.com/penny-vault/pvbt/asset"` to imports. It also references `runExploreGraph` which is created in Task 3.

**Step 2: Verify it compiles (will fail on missing runExploreGraph)**

Run: `go build ./cli/...`
Expected: fails -- `runExploreGraph` not defined. That's fine, created in Task 3.

**Step 3: Commit**

```bash
git add cli/explore.go
git commit -m "feat(cli): add explore command with table output and --list-metrics"
```

---

### Task 3: Add graph mode with bubbletea TUI

**Files:**
- Create: `cli/explore_graph.go`

**Step 1: Create cli/explore_graph.go**

A bubbletea TUI showing a line chart for the DataFrame's time series data. Uses ntcharts `timeserieslinechart` for the chart.

```go
package cli

import (
	"fmt"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/penny-vault/pvbt/data"
)

type exploreGraphModel struct {
	df     *data.DataFrame
	chart  timeserieslinechart.Model
	width  int
	height int
	ready  bool
}

func runExploreGraph(df *data.DataFrame) error {
	m := exploreGraphModel{df: df}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("graph TUI error: %w", err)
	}
	return nil
}

func (m exploreGraphModel) Init() tea.Cmd {
	return nil
}

func (m exploreGraphModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" || msg.String() == "esc" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chart = m.buildChart()
		m.ready = true
	}
	return m, nil
}

func (m exploreGraphModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Data Explorer")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Press q to quit")

	return fmt.Sprintf("%s\n%s\n%s", title, m.chart.View(), help)
}

func (m exploreGraphModel) buildChart() timeserieslinechart.Model {
	chartWidth := m.width - 2
	chartHeight := m.height - 4
	if chartWidth < 20 {
		chartWidth = 20
	}
	if chartHeight < 5 {
		chartHeight = 5
	}

	chart := timeserieslinechart.New(chartWidth, chartHeight)

	// determine time range from the DataFrame
	startTime := m.df.Start()
	endTime := m.df.End()
	chart.SetXRange(startTime, endTime)

	// determine Y range across all series
	minY := math.Inf(1)
	maxY := math.Inf(-1)

	// add each (asset, metric) column as a series
	// We need to iterate assets and metrics from the DataFrame.
	// Use the DataFrame's Column method to get each series.
	// Since DataFrame doesn't expose its asset/metric lists directly,
	// we need to track them from the original request.
	// For now, use the full data slab approach.

	// The DataFrame stores times, assets, metrics internally.
	// We can access columns via Column(asset, metric).
	// But we need the asset and metric lists. These are not exported.
	// We'll pass them separately or add accessor methods.

	// For the plan: we need to add Times(), AssetList(), MetricList()
	// accessors to DataFrame (Task 4).

	return chart
}
```

**STOP** -- the DataFrame doesn't export its `assets` or `metrics` slices, and we need them to iterate columns for the graph. We need accessor methods.

This means Task 3 depends on Task 4 (add DataFrame accessors). Reorder:

---

### Task 3: Add DataFrame accessor methods

**Files:**
- Modify: `data/data_frame.go`
- Test: `data/data_frame_test.go` (add tests)

**Step 1: Write failing tests**

Add to `data/data_frame_test.go` (in an existing Describe block or new one):

```go
var _ = Describe("DataFrame Accessors", func() {
	var df *data.DataFrame

	BeforeEach(func() {
		times := []time.Time{
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		}
		assets := []asset.Asset{
			{Ticker: "AAPL", CompositeFigi: "BBG000B9XRY4"},
		}
		metrics := []data.Metric{data.AdjClose}
		vals := []float64{100.0, 101.0}
		df = data.NewDataFrame(times, assets, metrics, vals)
	})

	It("Times returns the time axis", func() {
		times := df.Times()
		Expect(times).To(HaveLen(2))
		Expect(times[0]).To(Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))
	})

	It("AssetList returns the assets", func() {
		assets := df.AssetList()
		Expect(assets).To(HaveLen(1))
		Expect(assets[0].Ticker).To(Equal("AAPL"))
	})

	It("MetricList returns the metrics", func() {
		metrics := df.MetricList()
		Expect(metrics).To(HaveLen(1))
		Expect(metrics[0]).To(Equal(data.AdjClose))
	})
})
```

**Step 2: Run tests to verify they fail**

Run: `ginkgo --focus "DataFrame Accessors" ./data/...`
Expected: FAIL

**Step 3: Add accessor methods to data/data_frame.go**

Add after the existing `ColCount()` method (around line 144):

```go
// Times returns a copy of the timestamp axis.
func (df *DataFrame) Times() []time.Time {
	out := make([]time.Time, len(df.times))
	copy(out, df.times)
	return out
}

// AssetList returns a copy of the asset list.
func (df *DataFrame) AssetList() []asset.Asset {
	out := make([]asset.Asset, len(df.assets))
	copy(out, df.assets)
	return out
}

// MetricList returns a copy of the metric list.
func (df *DataFrame) MetricList() []Metric {
	out := make([]Metric, len(df.metrics))
	copy(out, df.metrics)
	return out
}
```

**Step 4: Run tests**

Run: `ginkgo --focus "DataFrame Accessors" ./data/...`
Expected: PASS

**Step 5: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add Times, AssetList, MetricList accessors to DataFrame"
```

---

### Task 4: Add graph mode with bubbletea TUI

**Files:**
- Create: `cli/explore_graph.go`

**Step 1: Create cli/explore_graph.go**

```go
package cli

import (
	"fmt"
	"math"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/penny-vault/pvbt/data"
)

type exploreGraphModel struct {
	df     *data.DataFrame
	width  int
	height int
	ready  bool
}

func runExploreGraph(df *data.DataFrame) error {
	m := exploreGraphModel{df: df}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("graph TUI error: %w", err)
	}
	return nil
}

func (m exploreGraphModel) Init() tea.Cmd {
	return nil
}

func (m exploreGraphModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" || msg.String() == "esc" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	}
	return m, nil
}

func (m exploreGraphModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	chartWidth := m.width - 2
	chartHeight := m.height - 4
	if chartWidth < 20 {
		chartWidth = 20
	}
	if chartHeight < 5 {
		chartHeight = 5
	}

	chart := timeserieslinechart.New(chartWidth, chartHeight)

	times := m.df.Times()
	assets := m.df.AssetList()
	metrics := m.df.MetricList()

	if len(times) < 2 {
		return "Not enough data points to graph."
	}

	chart.SetXRange(times[0], times[len(times)-1])

	// determine Y range
	minY := math.Inf(1)
	maxY := math.Inf(-1)
	for _, a := range assets {
		for _, met := range metrics {
			col := m.df.Column(a, met)
			for _, v := range col {
				if !math.IsNaN(v) {
					if v < minY {
						minY = v
					}
					if v > maxY {
						maxY = v
					}
				}
			}
		}
	}

	if math.IsInf(minY, 1) {
		return "No numeric data to graph."
	}

	padding := (maxY - minY) * 0.05
	if padding == 0 {
		padding = 1
	}
	chart.SetYRange(minY-padding, maxY+padding)

	// add each series
	for _, a := range assets {
		for _, met := range metrics {
			col := m.df.Column(a, met)
			for i, t := range times {
				if !math.IsNaN(col[i]) {
					chart.Push(timeserieslinechart.TimePoint{Time: t, Value: col[i]})
				}
			}
		}
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("Data Explorer")

	// build legend
	var legend string
	for _, a := range assets {
		for _, met := range metrics {
			legend += fmt.Sprintf("  %s/%s", a.Ticker, string(met))
		}
	}
	legendLine := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(legend)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Press q to quit")

	return fmt.Sprintf("%s%s\n%s\n%s", title, legendLine, chart.View(), help)
}
```

Note: The ntcharts API may need adjustment based on actual package version. The subagent should check the ntcharts API by looking at go doc or the module source. The key pattern is: `timeserieslinechart.New(width, height)`, `SetXRange`, `SetYRange`, `Push(TimePoint{Time, Value})`, `View()`.

**Step 2: Verify build**

Run: `go build ./cli/...`
Expected: clean build. If ntcharts API differs, adjust accordingly.

**Step 3: Commit**

```bash
git add cli/explore_graph.go
git commit -m "feat(cli): add TUI graph mode for explore command using ntcharts"
```

---

### Task 5: Create standalone binary entry point

**Files:**
- Create: `cmd/explore/main.go`

**Step 1: Create cmd/explore/main.go**

```go
package main

import "github.com/penny-vault/pvbt/cli"

func main() {
	cli.RunExplore()
}
```

**Step 2: Verify build**

Run: `go build ./cmd/explore/...`
Expected: produces binary

**Step 3: Commit**

```bash
git add cmd/explore/main.go
git commit -m "feat(cmd): add standalone explore binary entry point"
```

---

### Task 6: Final build, vet, and test

**Step 1: Full build**

Run: `go build ./...`
Expected: clean

**Step 2: Run all tests**

Run: `ginkgo ./...`
Expected: all pass

**Step 3: Run vet**

Run: `go vet ./...`
Expected: no issues

**Step 4: Commit any cleanup**

```bash
git add -A
git commit -m "chore: final cleanup for explore command"
```
