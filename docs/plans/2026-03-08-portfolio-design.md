# Portfolio Package Design

## Overview

The portfolio package turns strategy decisions into trades and measures performance. The `Account` type implements both `Portfolio` (strategy-facing) and `PortfolioManager` (engine-facing) interfaces. A broker is always required for order execution -- for backtesting the engine provides a simulated broker, for live trading a real one.

## Account Internals

```go
type Account struct {
    cash            float64
    holdings        map[asset.Asset]float64
    transactions    []Transaction
    broker          broker.Broker
    prices          *data.DataFrame
    equityCurve     []float64
    equityTimes     []time.Time
    benchmarkPrices []float64
    riskFreePrices  []float64
    benchmark       asset.Asset
    riskFree        asset.Asset
    taxLots         map[asset.Asset][]taxLot
}

type taxLot struct {
    Date  time.Time
    Qty   float64
    Price float64
}
```

## Construction Options

| Option | Description |
|--------|-------------|
| `WithCash(amount)` | Sets initial cash balance, records a DepositTransaction |
| `WithBroker(b)` | Sets the broker for order execution (required) |
| `WithBenchmark(a)` | Identifies which asset in the DataFrame is the benchmark |
| `WithRiskFree(a)` | Identifies which asset in the DataFrame is the risk-free rate |

## Portfolio Interface (strategy-facing)

### RebalanceTo

1. For each Allocation (in date order):
   - Compute total portfolio value (cash + holdings at current prices)
   - For each target asset: compute target shares = (weight * totalValue) / currentPrice
   - Diff against current holdings to get required buys/sells
   - Build `broker.Order` for each diff
   - Submit each order via broker
   - For each fill in the returned `[]Fill`: record a BuyTransaction or SellTransaction, update cash, update holdings, update tax lots

### Order

1. Translate OrderModifiers into `broker.Order` fields:
   - No modifier = Market order, Day TIF
   - Limit(price) = Limit order
   - Stop(price) = Stop order
   - Both Limit + Stop = StopLimit order
   - Time-in-force modifiers map directly to broker.TimeInForce values
2. Submit via broker
3. For each fill: record transaction, update cash, holdings, tax lots

### Read methods

- `Cash()` returns `a.cash`
- `Value()` returns `a.cash` + sum of (qty * currentPrice) for all holdings
- `Position(asset)` returns `a.holdings[asset]`
- `PositionValue(asset)` returns `a.holdings[asset] * currentPrice`
- `Holdings(fn)` iterates `a.holdings` calling fn for each entry
- `Transactions()` returns `a.transactions`

### Metric bundles

- `Summary()`, `RiskMetrics()`, `TaxMetrics()`, `TradeMetrics()`, `WithdrawalMetrics()` each call `PerformanceMetric(m).Value()` for their constituent metrics and return the bundle struct (already implemented in stubs)

## PortfolioManager Interface (engine-facing)

### Record(tx)

Appends tx to the transaction log. Updates cash and holdings based on transaction type:
- Buy: cash -= amount, holdings[asset] += qty, add tax lot
- Sell: cash += amount, holdings[asset] -= qty, consume tax lots (FIFO)
- Dividend: cash += amount
- Fee: cash -= abs(amount)
- Deposit: cash += amount
- Withdrawal: cash -= abs(amount)

### UpdatePrices(df)

1. Store `df` as latest prices
2. Compute total portfolio value using `MetricClose` for held assets
3. Append value to equity curve, timestamp to equityTimes
4. Append benchmark `AdjClose` to benchmarkPrices
5. Append risk-free `AdjClose` to riskFreePrices

### SetBroker(b)

Replaces `a.broker`.

## Tax Lot Tracking

FIFO order. Each buy creates a tax lot `{Date, Qty, Price}`. Each sell consumes lots oldest-first, splitting a lot if partially consumed. TaxMetrics uses lot dates to classify gains as long-term (held > 1 year) or short-term.

## Selection

### MaxAboveZero(fallback)

For each timestamp in the DataFrame:
- Find the asset with the highest value above zero
- If none qualify, select the fallback assets
- Return a new DataFrame containing only the selected assets

## Weighting

### EqualWeight(df)

For each timestamp in the DataFrame:
- Count assets present (N)
- Create an Allocation with weight 1/N for each asset
- Return PortfolioPlan (slice of Allocations)

### WeightedBySignal(df, metric)

For each timestamp in the DataFrame:
- Read the named metric column for each asset
- Normalize values to sum to 1.0
- Create an Allocation with the normalized weights
- Return PortfolioPlan

## Performance Metrics

All metrics receive `*Account` and `*Period`. They derive return series from the equity curve, benchmark prices, and risk-free prices as needed.

### Data derivation helpers (unexported)

These are helper methods on Account or free functions used by metrics:

- `returns(prices []float64) []float64` -- period-over-period returns from a price series
- `excessReturns(returns, riskFreeReturns []float64) []float64` -- returns minus risk-free
- `windowSlice(series []float64, times []time.Time, window *Period) []float64` -- trim series to window

### Metric implementations

**Return metrics:**
- TWRR: compound sub-period returns from equity curve
- MWRR: XIRR on cash flows (deposits, withdrawals) and ending value
- ActiveReturn: portfolio return minus benchmark return

**Risk-adjusted ratios:**
- Sharpe: mean excess return / std dev of excess returns, annualized
- Sortino: mean excess return / downside deviation, annualized
- Calmar: CAGR / max drawdown
- Treynor: excess return / beta
- InformationRatio: mean active return / tracking error, annualized
- KRatio: slope of log-VAMI regression / (N * std error of slope)
- KellerRatio: R * (1 - D/(1-D)) when R >= 0 and D <= 50%, else 0

**Risk and volatility:**
- StdDev: std dev of monthly returns * sqrt(12)
- Beta: cov(portfolio, benchmark) / var(benchmark)
- Alpha: Rp - [Rf + (Rm - Rf) * beta]
- DownsideDeviation: std dev of negative excess returns, annualized
- TrackingError: std dev of active returns
- UlcerIndex: sqrt(mean of squared percentage drawdowns), 14-day lookback
- ValueAtRisk: percentile-based at 95% confidence

**Drawdown:**
- MaxDrawdown: largest peak-to-trough decline
- AvgDrawdown: mean of all drawdown magnitudes

**Distribution:**
- ExcessKurtosis: kurtosis - 3
- Skewness: third standardized moment

**Benchmark comparison:**
- RSquared: correlation^2 of portfolio vs benchmark returns
- UpsideCaptureRatio: portfolio return / benchmark return when benchmark is up
- DownsideCaptureRatio: portfolio return / benchmark return when benchmark is down

**Win/loss:**
- NPositivePeriods: percentage of periods with positive returns
- GainLossRatio: average gain / average loss

**Withdrawal rates:**
- SafeWithdrawalRate: Monte Carlo, max rate where balance never hits zero
- PerpetualWithdrawalRate: Monte Carlo, max rate preserving inflation-adjusted balance
- DynamicWithdrawalRate: Monte Carlo with dynamic spending adjustments

**Trade metrics (transaction-derived):**
- WinRate: profitable round-trip trades / total round-trip trades
- AverageWin: mean profit on winners
- AverageLoss: mean loss on losers
- ProfitFactor: gross profit / gross loss
- AverageHoldingPeriod: mean days between buy and sell
- Turnover: annual turnover rate

**Tax metrics (lot-derived):**
- LTCG/STCG: realized gains classified by holding period
- UnrealizedLTCG/UnrealizedSTCG: unrealized gains classified by holding period
- QualifiedDividends/NonQualifiedIncome: from dividend transactions
- TaxCostRatio: percentage of return lost to taxes

## TransactionType

Add `String()` method returning the name for each iota value.
