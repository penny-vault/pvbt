# Go Unit Test Writer - Agent Memory

## Project Structure
- Tests use **Ginkgo v2 BDD framework** with **Gomega matchers** (NOT standard testing + testify)
- Run tests with `go test ./portfolio/ -v -count=1 -run "TestPortfolio"` or `ginkgo run -race ./data`
- Test files are in `_test` package (black-box): `package data_test`, `package portfolio_test`
- Suite bootstrap files: `data/data_suite_test.go`, `portfolio/portfolio_suite_test.go`
- Coverage: `go test -coverprofile=cover.out ./data/...` works for coverage reports

## Testing Patterns in This Project
- Ginkgo `Describe`/`Context`/`It` blocks, NOT table-driven `[]struct` pattern
- `BeforeEach` for shared fixture setup per Describe block
- Gomega matchers: `Equal`, `BeNumerically("~", val, delta)`, `BeTrue`, `BeNil`, `Panic`, `HaveLen`, `Succeed`
- NaN checks: `Expect(math.IsNaN(v)).To(BeTrue())`
- Panic checks: `Expect(func() { ... }).To(Panic())`
- Float comparison: `BeNumerically("~", expected, 1e-10)`

## Key Test Fixtures (data package)
- Standard 2-asset (AAPL/GOOG), 2-metric (Price/Volume), 5-timestamp DataFrame
- Assets constructed as `asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}`
- Base date: `time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)` with daily increments

## Key Test Fixtures (portfolio package)
- `buildDF()` in testutil_test.go: single-timestamp DF with MetricClose + AdjClose
- `buildMultiDF()`: multi-timestamp version; `daySeq()`: weekday timestamp generator
- `mockBroker` in order_test.go: configurable with `defaultFill`, `fills`, `fillsByAsset`, `submitErr`
- Summary fixture: SPY=[100,105,98,103,97,110], BIL=[100,100.01,...,100.05], 5 shares
- Equity curve=[500,525,490,515,485,550], Times: daySeq(2025-01-02, 6)

## Metric Computation Details
- All metrics implement `PerformanceMetric` with `Compute(a *Account, window *Period) float64`
- Query builder: `acct.PerformanceMetric(portfolio.Sharpe).Window(portfolio.Days(10)).Value()`
- `annualizationFactor()`: 252 for daily (avgDays<=20), 12 for monthly
- `variance()` uses sample (N-1) denominator; `drawdownSeries()` returns negatives (0 at peaks)
- UpdatePrices uses AdjClose for benchmark/risk-free, MetricClose for holdings
- MWRR: deposit from WithCash has zero Date, mapped to startDate=times[0]
- VaR: uses floor(0.05*len(sorted)) as index into sorted returns
- Skewness/Kurtosis use equity returns (NOT excess returns)
- DownsideDeviation uses negative excess returns only
- CVaR: cutoff = max(floor(0.05*n), 1); averages worst returns up to cutoff
- SmartSharpe/SmartSortino: autocorrelationPenalty defaults to 1.0 when inner <= 0 (negative autocorrelation can cause this)
- ProbabilisticSharpe: requires n >= 4; returns 0 when inner <= 0 (e.g., uniform returns with extreme kurtosis). Use varied return series for "near 1.0" tests.
- OmegaRatio/GainToPainRatio: both return 0 when losses/negativeSum = 0 (all positive or flat)
- KellyCriterion: returns 0 when wins=0 OR losses=0 (needs both)

## Coverage Status (as of 2026-03-10)
- data package: 100.0% statement coverage, 154 tests
- portfolio package: 465 tests (including 92 for new metrics in new_metrics_test.go)

## TWRR/MWRR Implementation Details
- TWRR just compounds equity curve ratios: product(equity[i+1]/equity[i]) - 1. Does NOT adjust for cash flows.
- Deposits/withdrawals between UpdatePrices calls cause equity jumps that TWRR treats as returns.
- For buy-and-hold with no mid-stream cash flows, TWRR correctly matches price returns.
- MWRR (XIRR) is annualized. Short-period high returns annualize to very large numbers.
- When comparing MWRR across scenarios, annualization effect dominates: late deposits with short-term high returns can have higher annualized MWRR than early deposits with longer-term gains, even if early timing was "better" in absolute terms.
- For MWRR timing tests: deposit-before-decline produces lower MWRR than deposit-before-rally (because terminal value is lower with same total invested).
- Record(BuyTransaction) with Amount=-(price*qty) keeps total value constant (cash down, holdings up).

## Gotchas
- Max()/Min() aggregation creates synthetic asset with empty CompositeFigi
- Column() returns a slice sharing underlying data
- gonum stat.Quantile with LinInterp uses N*p indexing, NOT (N-1)*p
- NewDataFrame(nil, []asset.Asset{a}, []data.Metric{m}, nil) is valid (T=0, A=1, M=1)
- BeforeEach rebuilds fixtures per It block; no need to restore mutated data
- Assets(aapl, aapl) does NOT deduplicate
- test_provider.go has compile-time check: var _ BatchProvider = (*TestProvider)(nil)
