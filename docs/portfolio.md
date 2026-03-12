# Portfolio

The portfolio is where strategy decisions become trades. It has two responsibilities: construction -- turning allocation decisions into orders -- and measurement -- computing performance metrics over the portfolio's history.

Strategy code interacts with the portfolio through the `Portfolio` interface -- it calls `RebalanceTo`, `Order`, reads state, and inspects the transaction log. The engine uses a separate `PortfolioManager` interface to inject external events like dividends and fees. Both interfaces are implemented by the concrete `Account` type.

```go
// create a portfolio with $10K
acct := portfolio.New(portfolio.WithCash(10_000))

eng := engine.New(&MyStrategy{},
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
defer eng.Close()

acct, err := eng.Backtest(ctx, acct, start, end)
```

For backtesting, the engine automatically attaches a simulated broker to the account. For live trading, attach a real broker via `portfolio.WithBroker(b)`. The portfolio delegates all order execution to the broker and never computes fill prices itself.

## Allocation and PortfolioPlan

An `Allocation` is a single rebalance target: a date and a map of assets to their target weight percentages (summing to 1.0).

```go
type Allocation struct {
    Date    time.Time
    Members map[asset.Asset]float64
}
```

A `PortfolioPlan` is a time-ordered series of allocations describing how the portfolio should be invested over time:

```go
type PortfolioPlan []Allocation
```

Portfolio construction is a two-step pipeline: **selection** filters a DataFrame to the assets you want, and **weighting** assigns target weights to produce a `PortfolioPlan`.

## Selection

A `Selector` filters a DataFrame to only the assets that should be held at each timestep:

```go
type Selector interface {
    Select(df *data.DataFrame) *data.DataFrame
}
```

The returned DataFrame has the same structure but with unselected assets removed. For example, `MaxAboveZero` picks the asset with the highest signal value above zero at each timestep, falling back to specified assets if nothing qualifies:

```go
selected := momentum.Select(portfolio.MaxAboveZero(s.riskOff))
```

## Weighting

Weighting functions take a filtered DataFrame and produce a `PortfolioPlan` with target weights at each timestep:

| Function | Description |
|----------|-------------|
| `EqualWeight(df)` | Assigns equal weights to all selected assets (3 assets = 1/3 each) |
| `WeightedBySignal(df, metric)` | Weights proportionally to a metric column, normalized to sum to 1.0 |

```go
// equal weight
plan := portfolio.EqualWeight(selected)

// weight by market cap
plan := portfolio.WeightedBySignal(selected, data.MarketCap)
```

## Construction

There are two ways to express allocation decisions, and they can be mixed freely within a strategy.

### Declarative: RebalanceTo

The most common approach. The strategy computes a selection and weighting, then tells the portfolio to match it:

```go
selected := momentum.Select(portfolio.MaxAboveZero(s.riskOff))
plan := portfolio.EqualWeight(selected)
p.RebalanceTo(plan...)
```

`RebalanceTo` accepts variadic `Allocation` arguments. Pass a single allocation for an immediate rebalance, or spread a `PortfolioPlan` to apply a series of rebalances in date order. The engine diffs current holdings against each target and generates the necessary buy/sell orders. Large orders may be filled in multiple lots at different prices, producing one transaction per fill.

A more involved example -- select the 50 cheapest stocks by P/E and weight by market cap:

```go
cheapest := pe.Sort(Ascending).Top(50)
plan := portfolio.WeightedBySignal(cheapest, data.MarketCap)
p.RebalanceTo(plan...)
```

### Imperative: Order

When a strategy needs more control, it can place individual orders:

```go
p.Order(asset, Buy, 100)
p.Order(asset, Sell, 50)
```

Order behavior is controlled by optional modifiers passed as variadic arguments. Modifiers fall into two categories: order type (price conditions) and time in force (lifetime).

#### Order types

A plain `Order` with no modifiers is a **market order**, filled at the next available price plus slippage.

**Limit** sets a maximum buy price or minimum sell price:

```go
p.Order(asset, Buy, 100, Limit(150.00))
```

**Stop** (stop loss) triggers a market order when the price reaches a threshold:

```go
p.Order(asset, Sell, 100, Stop(140.00))
```

Combining `Stop` and `Limit` creates a **stop-limit order** -- the order activates at the stop price, then fills only at the limit price or better:

```go
p.Order(asset, Sell, 100, Stop(140.00), Limit(135.00))
```

#### Time in force

Time in force controls how long the order stays active. The default is `DayOrder`.

| Modifier | Behavior |
|----------|----------|
| `DayOrder` | Cancels at market close if not executed (default) |
| `GoodTilCancel` | Stays open until filled or cancelled (up to 180 days) |
| `GoodTilDate(t)` | Stays open until a specified date |
| `FillOrKill` | Fill entirely or cancel immediately |
| `ImmediateOrCancel` | Fill as many shares as possible, cancel the rest |
| `OnTheOpen` | Fill only at the opening price |
| `OnTheClose` | Fill only at the closing price |

Order type and time in force modifiers can be combined freely:

```go
p.Order(asset, Buy, 100, Limit(150.00), GoodTilCancel)
p.Order(asset, Sell, 50, Stop(140.00), FillOrKill)
p.Order(asset, Buy, 200, OnTheOpen)
```

### Mixing approaches

A strategy can use `RebalanceTo` for its core allocation and `Order` for specific adjustments:

```go
func (s *MyStrategy) Compute(ctx context.Context, e *engine.Engine, p Portfolio) {
    // Core allocation
    cheapest := pe.Sort(Ascending).Top(50)
    plan := portfolio.EqualWeight(cheapest)
    p.RebalanceTo(plan...)

    // Tactical overlay
    if vix.Last() > 30 {
        p.Order(spy, Buy, 100, Limit(150.00))
    }
}
```

Both paths go through the same execution pipeline. Both are subject to the same risk controls.

## Reading portfolio state

During computation the strategy can inspect the current portfolio:

```go
p.Cash()                // current cash balance
p.Value()               // total portfolio value (cash + holdings at mark)
p.Position(asset)       // quantity held of a specific asset
p.PositionValue(asset)  // market value of the position in an asset
```

To iterate over all current holdings:

```go
p.Holdings(func(a Asset, qty float64) {
    log.Info().
        Str("symbol", a.Symbol()).
        Float64("qty", qty).
        Msg("holding")
})
```

## Transaction log

Every operation that changes the portfolio's state appends to the transaction log. The log is the source of truth for performance measurement, tax calculations, and trade analysis.

Trades produce transactions automatically via `RebalanceTo` and `Order`. External events like dividends, fees, deposits, and withdrawals are injected by the engine via the `PortfolioManager.Record` method -- strategy code does not call `Record` directly.

```go
type Transaction struct {
    Date   time.Time
    Asset  asset.Asset
    Type   TransactionType
    Qty    float64
    Price  float64
    Amount float64
}
```

`Amount` is the total cash impact: positive for inflows (sells, dividends, deposits), negative for outflows (buys, fees, withdrawals).

Transaction types:

| Type | Recorded when |
|------|---------------|
| `BuyTransaction` | An asset is purchased (via `RebalanceTo` or `Order`) |
| `SellTransaction` | An asset is sold |
| `DividendTransaction` | A dividend payment is received |
| `FeeTransaction` | A commission or fee is charged |
| `DepositTransaction` | Cash is added to the account |
| `WithdrawalTransaction` | Cash is removed from the account |

To access the full log:

```go
for _, tx := range p.Transactions() {
    log.Info().
        Time("date", tx.Date).
        Str("type", tx.Type.String()).
        Float64("amount", tx.Amount).
        Msg("transaction")
}
```

The transaction log feeds many of the performance and tax signals. Trade signals like `WinRate`, `ProfitFactor`, and `AverageHoldingPeriod` are derived from buy/sell transactions. Tax signals like `LTCG`, `STCG`, and `QualifiedDividends` are derived from the same log combined with tax lot tracking.

## Risk controls

Risk controls constrain what the portfolio can do. They are configured by the operator — the person running the backtest or the live system — not by the strategy author. This is a deliberate separation: the strategy expresses intent, and the risk layer enforces limits.

Risk controls apply uniformly to all operations, whether they come from `RebalanceTo` or `Order`. If a strategy tries to allocate 50% to a single stock but the operator has set a 10% position limit, the portfolio will cap the position and distribute the excess according to its rules.

The specifics of risk control configuration are outside the strategy API. Strategy authors should write their logic without worrying about position limits, turnover constraints, or other guardrails — those are someone else's responsibility.

## Performance measurement

The portfolio tracks its own history and can compute performance metrics over any window. Each metric is a type that implements the `PerformanceMetric` interface, and new metrics can be defined by anyone.

### Individual metrics

Use `PerformanceMetric` to query a single metric. It returns a `PerformanceMetricQuery` builder with optional `Window` and then `Value` (scalar) or `Series` (rolling `[]float64`):

```go
// full history
sharpe := p.PerformanceMetric(Sharpe).Value()

// trailing 36-month window
sharpe36 := p.PerformanceMetric(Sharpe).Window(Months(36)).Value()

// rolling series as []float64, compatible with gonum
series := p.PerformanceMetric(Sharpe).Series()
```

Windows are specified with `Days(n)`, `Months(n)`, or `Years(n)`. These handle calendar-aware durations correctly (months and years have variable lengths).

### Available metrics

Every metric below is a package-level variable implementing `PerformanceMetric`. Each can be used with `p.PerformanceMetric(metric).Value()` or `.Series()`.

**Return metrics:**

| Name | Description |
|------|-------------|
| `TWRR` | Time-weighted rate of return. Eliminates the effect of cash flows by compounding sub-period returns, showing pure investment performance independent of deposit/withdrawal timing. |
| `MWRR` | Money-weighted rate of return. Accounts for the timing and size of cash flows using XIRR. Unlike TWRR, this reflects the investor's actual dollar-weighted experience. |
| `ActiveReturn` | Difference between portfolio return and benchmark return (strategy - benchmark). Measures the value added by active management. Highly dependent on appropriate benchmark selection. |

**Risk-adjusted ratios:**

| Name | Description |
|------|-------------|
| `Sharpe` | Risk-adjusted return: excess return over the risk-free rate divided by annualized standard deviation of returns. Higher is better. Uses monthly returns annualized to match Morningstar convention. |
| `Sortino` | Like Sharpe but uses downside deviation instead of total standard deviation. Only penalizes volatility from negative excess returns, treating upside volatility as desirable. |
| `Calmar` | Annualized return (CAGR) divided by maximum drawdown. Measures how much return compensates for the worst peak-to-trough decline. Typically computed over 36 months. |
| `Treynor` | Excess return per unit of systematic risk (beta). Similar to Sharpe but uses beta instead of standard deviation, isolating the reward for market risk rather than total risk. |
| `InformationRatio` | Mean active return divided by tracking error, annualized. Measures how consistently the portfolio outperforms the benchmark per unit of divergence from it. |
| `KRatio` | Consistency of returns over time. Computed as the slope of the log-VAMI regression line divided by N times the standard error of the slope. Higher values indicate steadier, more linear growth. |
| `KellerRatio` | Drawdown-adjusted return: K = R * (1 - D/(1-D)) when R >= 0 and D <= 50%, else 0. Small drawdowns have limited impact on the adjustment; large drawdowns amplify the penalty dramatically. |

**Risk and volatility:**

| Name | Description |
|------|-------------|
| `StdDev` | Annualized standard deviation of monthly returns. The most basic measure of portfolio volatility. Uses monthly returns multiplied by sqrt(12) to annualize, consistent with Morningstar. |
| `Beta` | Sensitivity of portfolio returns to benchmark movements (covariance / benchmark variance). A beta of 1.0 means the portfolio moves with the market; above 1.0 means more volatile. |
| `Alpha` | Jensen's alpha: the portfolio's excess return beyond what CAPM would predict given its beta. Positive alpha means the portfolio outperformed its risk-adjusted expectation. Formula: Rp - [Rf + (Rm - Rf) * beta]. |
| `DownsideDeviation` | Standard deviation of negative excess returns (returns below the risk-free rate), annualized. Measures harmful volatility only, ignoring upside moves. Used as the denominator in the Sortino ratio. |
| `TrackingError` | Standard deviation of the difference between portfolio and benchmark returns (active returns). Measures how much the portfolio's returns deviate from the benchmark day-to-day. |
| `UlcerIndex` | Downside risk measure using both depth and duration of drawdowns. Computed as sqrt(mean of squared percentage drawdowns) over a 14-day lookback. Higher values mean more painful, prolonged drawdowns. |
| `ValueAtRisk` | Maximum expected loss over a given time horizon at a specified confidence level (e.g., 95%). Answers: "What is the worst I can expect to lose with 95% confidence?" |

**Drawdown:**

| Name | Description |
|------|-------------|
| `MaxDrawdown` | Largest peak-to-trough decline in portfolio value. A max drawdown of 20% means the portfolio fell 20% from its high before recovering. The single most important risk metric for many investors. |
| `AvgDrawdown` | Mean loss percentage across all drawdown periods. A drawdown is the decline from a peak to a subsequent trough. Lower average drawdowns indicate the portfolio tends to recover quickly from losses. |

**Distribution shape:**

| Name | Description |
|------|-------------|
| `ExcessKurtosis` | How much fatter the tails of the return distribution are compared to a normal distribution. Positive values mean more extreme outcomes (both gains and losses) than a normal distribution predicts. Important for understanding tail risk. |
| `Skewness` | Asymmetry of the return distribution. Negative skew means the left tail is longer (more large losses than large gains). Positive skew means the right tail is longer. Most equity strategies exhibit negative skew. |

**Benchmark comparison:**

| Name | Description |
|------|-------------|
| `RSquared` | Coefficient of determination: how well portfolio returns are explained by benchmark returns. Ranges 0 to 1. Near 1 means the portfolio closely tracks the benchmark; low values mean returns are driven by other factors. |
| `UpsideCaptureRatio` | Portfolio return divided by benchmark return during periods when the benchmark is up. Above 100% means the portfolio outperforms in rising markets. |
| `DownsideCaptureRatio` | Portfolio return divided by benchmark return during periods when the benchmark is down. Below 100% means the portfolio loses less than the benchmark in falling markets. Ideally low. |

**Win/loss profile:**

| Name | Description |
|------|-------------|
| `NPositivePeriods` | Percentage of periods with positive returns. Higher values mean the portfolio gains more often, though this says nothing about the magnitude of gains vs losses. |
| `GainLossRatio` | Average gain on winning periods divided by average loss on losing periods. Above 1.0 means wins are larger than losses on average. Combined with NPositivePeriods, gives a complete win/loss picture. |

**Withdrawal rates:**

| Name | Description |
|------|-------------|
| `SafeWithdrawalRate` | Maximum constant annual withdrawal rate (as a percentage of initial balance) where the portfolio balance never reaches zero. Computed via circular bootstrap Monte Carlo simulation of historical returns. |
| `PerpetualWithdrawalRate` | Maximum constant annual withdrawal rate where the ending balance equals or exceeds the inflation-adjusted starting balance. Ensures the portfolio maintains real purchasing power indefinitely. |
| `DynamicWithdrawalRate` | Maximum annual withdrawal rate with dynamic adjustments: each year's withdrawal is the lesser of the inflation-adjusted initial withdrawal and the current balance times the rate. Adapts spending to portfolio performance. |

### Metric bundles

For convenience, several bundles return a struct with commonly grouped metrics computed over the full history.

**Summary** -- headline performance numbers (`p.Summary()`):

| Name | Description |
|------|-------------|
| `TWRR` | Time-weighted rate of return, eliminating the effect of cash flows |
| `MWRR` | Money-weighted rate of return, reflecting actual investor experience |
| `Sharpe` | Risk-adjusted return using standard deviation as the risk measure |
| `Sortino` | Like Sharpe but penalizes only downside volatility |
| `Calmar` | Annualized return divided by maximum drawdown |
| `MaxDrawdown` | Largest peak-to-trough decline in portfolio value |
| `StdDev` | Standard deviation of monthly returns |

**RiskMetrics** -- risk-related measurements (`p.RiskMetrics()`):

| Name | Description |
|------|-------------|
| `Beta` | Sensitivity of portfolio returns to benchmark movements |
| `Alpha` | Jensen's alpha: excess return over CAPM prediction given beta |
| `TrackingError` | Standard deviation of the difference between portfolio and benchmark returns |
| `DownsideDeviation` | Volatility of returns below the risk-free rate |
| `InformationRatio` | Active return per unit of tracking error |
| `Treynor` | Excess return per unit of systematic risk (beta) |
| `UlcerIndex` | Downside risk based on drawdown depth and duration |
| `ExcessKurtosis` | Tail risk relative to a normal distribution |
| `Skewness` | Asymmetry of the return distribution |
| `RSquared` | How well returns are explained by benchmark returns |
| `ValueAtRisk` | Maximum expected loss at a given confidence level |
| `UpsideCaptureRatio` | Percentage of benchmark gains captured |
| `DownsideCaptureRatio` | Percentage of benchmark losses captured |

**TaxMetrics** -- tax efficiency measurements derived from the transaction log and tax lot tracking (`p.TaxMetrics()`):

| Name | Description |
|------|-------------|
| `LTCG` | Realized long-term capital gains |
| `STCG` | Realized short-term capital gains |
| `UnrealizedLTCG` | Unrealized long-term capital gains at current prices |
| `UnrealizedSTCG` | Unrealized short-term capital gains at current prices |
| `QualifiedDividends` | Qualified dividend income received |
| `NonQualifiedIncome` | Non-qualified dividend and interest income |
| `TaxCostRatio` | Percentage of return lost to taxes |

**TradeMetrics** -- trade analysis derived from the transaction log (`p.TradeMetrics()`):

| Name | Description |
|------|-------------|
| `WinRate` | Percentage of trades that were profitable |
| `AverageWin` | Average profit on winning trades |
| `AverageLoss` | Average loss on losing trades |
| `ProfitFactor` | Gross profit divided by gross loss |
| `AverageHoldingPeriod` | Average number of days a position is held |
| `Turnover` | Annual portfolio turnover rate |
| `NPositivePeriods` | Percentage of periods with positive returns |
| `GainLossRatio` | Average gain divided by average loss |

**WithdrawalMetrics** -- sustainable spending rates derived from Monte Carlo simulation of historical returns (`p.WithdrawalMetrics()`):

| Name | Description |
|------|-------------|
| `SafeWithdrawalRate` | Maximum withdrawal rate where balance never hits zero |
| `PerpetualWithdrawalRate` | Maximum withdrawal rate preserving inflation-adjusted balance |
| `DynamicWithdrawalRate` | Maximum withdrawal rate with dynamic spending adjustments |

### Custom metrics

Anyone can define a custom performance metric by implementing the `PerformanceMetric` interface:

```go
type PerformanceMetric interface {
    Name() string
    Compute(a *Account, window *Period) float64
    ComputeSeries(a *Account, window *Period) []float64
}
```

`Compute` receives the full `*Account` so it can access the equity curve, benchmark data, risk-free rate, and transaction history -- not just raw transactions. This is what allows metrics like `Alpha` and `Beta` to reference the benchmark, and `Sharpe` to use the risk-free rate.

Each built-in metric is an unexported struct with an exported package-level variable (e.g., `var Sharpe = sharpe{}`). Custom metrics follow the same pattern:

```go
type myMetric struct{}
func (myMetric) Name() string { return "MyMetric" }
func (myMetric) Compute(a *Account, window *Period) float64 { ... }
func (myMetric) ComputeSeries(a *Account, window *Period) []float64 { ... }

var MyMetric = myMetric{}

// use it
val := p.PerformanceMetric(MyMetric).Window(Years(3)).Value()
```

### Using metrics during computation

While performance metrics are most commonly examined after a backtest completes, they are available during computation. A strategy might use them to adjust its behavior:

```go
func (s *Adaptive) Compute(ctx context.Context, e *engine.Engine, p Portfolio) {
    dd := p.PerformanceMetric(MaxDrawdown).Value()
    if dd > 0.15 {
        // drawdown exceeds 15%, reduce exposure
        p.RebalanceTo(EqualWeight(s.riskOff))
        return
    }

    // normal allocation logic
    // ...
}
```

This is less common but occasionally useful for strategies that adapt to their own performance.

## Broker integration

For backtesting, the engine automatically attaches a `SimulatedBroker` that fills all orders at the close price. For live trading, attach a real broker at construction:

```go
// backtesting -- engine sets the broker automatically
acct := portfolio.New(portfolio.WithCash(10_000))

// live trading -- attach a broker explicitly
acct := portfolio.New(portfolio.WithCash(10_000), portfolio.WithBroker(liveBroker))
```

Strategy code never knows or cares which broker is attached -- it calls `RebalanceTo` and `Order` the same way in both cases. The portfolio translates its internal order representation into `broker.Order` values and submits them. Large orders may be filled in multiple lots at different prices, producing one transaction per fill.

See [Broker](broker.md) for the full broker interface and supported order types.
