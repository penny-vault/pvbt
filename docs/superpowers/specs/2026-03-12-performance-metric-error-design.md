# PerformanceMetric.Compute Error Returns

## Summary

Change the `PerformanceMetric` interface so `Compute` returns `(float64, error)` and `ComputeSeries` returns `([]float64, error)`. This lets metrics like Sharpe return an error when required configuration (risk-free rate, benchmark) is missing, instead of silently returning meaningless zeros.

## Interface Change

```go
type PerformanceMetric interface {
    Name() string
    Description() string
    Compute(a *Account, window *Period) (float64, error)
    ComputeSeries(a *Account, window *Period) ([]float64, error)
}
```

Sentinel errors:

```go
var (
    ErrNoRiskFreeRate = errors.New("risk-free rate not configured")
    ErrNoBenchmark    = errors.New("benchmark not configured")
)
```

Detection: `len(a.RiskFreePrices()) == 0` / `len(a.BenchmarkPrices()) == 0`.

## Metric Categories

**Return `ErrNoRiskFreeRate`:** Sharpe, Sortino, SmartSharpe, SmartSortino, Treynor, Alpha, ProbabilisticSharpe, KellerRatio.

**Return `ErrNoBenchmark`:** Beta, Alpha, TrackingError, InformationRatio, RSquared, UpsideCaptureRatio, DownsideCaptureRatio.

**Compose other metrics (propagate errors):** Alpha calls Beta.Compute, Treynor calls Beta.Compute.

**All others (~45):** Add `return x, nil`. No new error conditions. Insufficient data / division-by-zero continues to return `0, nil`.

## Query Builder

```go
func (q PerformanceMetricQuery) Value() (float64, error)
func (q PerformanceMetricQuery) Series() ([]float64, error)
```

## Aggregate Methods

`Summary()`, `RiskMetrics()`, `TaxMetrics()`, `TradeMetrics()`, `WithdrawalMetrics()` change to return `(Struct, error)`. Use `errors.Join` to collect all errors. Populate each field that succeeds; leave failed fields at zero. Return partial results with combined error.

## Portfolio Interface

```go
Summary() (Summary, error)
RiskMetrics() (RiskMetrics, error)
TaxMetrics() (TaxMetrics, error)
TradeMetrics() (TradeMetrics, error)
WithdrawalMetrics() (WithdrawalMetrics, error)
```

## CLI Callers

Log warnings for metric errors, continue with partial results.

## Tests

- Existing tests add `err` return: `v, err := ...; Expect(err).NotTo(HaveOccurred())`
- New error-path tests assert the sentinel error and zero value.

## Design Rules

- Configuration errors (no risk-free, no benchmark) return errors.
- Insufficient data / division by zero returns `0, nil` (existing behavior preserved).
