# Go Unit Test Writer - Agent Memory

## Project Structure
- Tests use **Ginkgo v2 BDD framework** with **Gomega matchers** (NOT standard testing + testify)
- Run tests with `ginkgo run -race ./data` (not `go test`)
- Test files are in `_test` package (black-box): `package data_test`
- Suite bootstrap: `data/data_suite_test.go` with `TestData` entry point
- Coverage: `go test -coverprofile=cover.out ./data/...` works for coverage reports

## Testing Patterns in This Project
- Ginkgo `Describe`/`Context`/`It` blocks, NOT table-driven `[]struct` pattern
- `BeforeEach` for shared fixture setup per Describe block
- Gomega matchers: `Equal`, `BeNumerically("~", val, delta)`, `BeTrue`, `BeNil`, `Panic`, `HaveLen`, `Succeed`
- NaN checks: `Expect(math.IsNaN(v)).To(BeTrue())`
- Panic checks: `Expect(func() { ... }).To(Panic())`
- Float comparison: `BeNumerically("~", expected, 1e-10)`

## Key Test Fixtures
- Standard 2-asset (AAPL/GOOG), 2-metric (Price/Volume), 5-timestamp DataFrame
- Assets constructed as `asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}`
- Base date: `time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)` with daily increments

## Coverage Status (as of 2026-03-07)
- Overall: 100.0% statement coverage, 154 tests
- All functions including Aggregation.String(), Frequency.String(), periodChanged default, aggregate default: 100%

## Bug Fix Applied
- `Diff()` panicked on frames with assets/metrics but zero timestamps (T=0, A>0, M>0)
- Fixed by adding `if len(col) == 0 { return out }` guard in the Apply closure

## Gotchas
- Max()/Min() aggregation creates synthetic asset with empty CompositeFigi
  - Looked up in tests via `asset.Asset{Ticker: "MAX"}` (CompositeFigi="")
- Column() returns a slice sharing underlying data (documented, tested)
- gonum stat.Quantile with LinInterp uses N*p indexing, NOT (N-1)*p
  - For [1,2,3] at p=0.5: returns 1.5, not the textbook median 2.0
- NewDataFrame(nil, []asset.Asset{a}, []data.Metric{m}, nil) is valid (T=0, A=1, M=1)
- BeforeEach rebuilds fixtures per It block; no need to restore mutated data
- Assets(aapl, aapl) does NOT deduplicate -- AAPL appears twice in result
- test_provider.go already has compile-time check: var _ BatchProvider = (*TestProvider)(nil)
