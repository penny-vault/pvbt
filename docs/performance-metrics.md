# Performance Metrics

The portfolio tracks its own history and can compute performance metrics over any window. Each metric is a type that implements the `PerformanceMetric` interface, and new metrics can be defined by anyone.

Portfolio metrics operate on the returns and transactions of a specific portfolio. The same metric (e.g. `Sharpe` or `MaxDrawdown`) will produce different values for different portfolios because each portfolio has its own equity curve shaped by its particular trades, cash flows, and timing. This is the key distinction from signals (see [Data - Signals](data.md#signals)), which operate on market data like prices and economic indicators and produce the same output regardless of portfolio state.

## Evaluating a portfolio

After a backtest, the portfolio gives you several ways to assess results, from quick headline numbers to deep trade-level analysis.

### Quick summary

`Summary()` returns the metrics most investors check first:

```go
result, _ := eng.Backtest(ctx, start, end)
summary, _ := result.Summary()

fmt.Printf("Return: %.2f%%  Sharpe: %.2f  MaxDD: %.2f%%\n",
    summary.TWRR*100, summary.Sharpe, summary.MaxDrawdown*100)
```

If Sharpe is above 1.0, returns are attractive relative to risk. If MaxDrawdown exceeds what you could stomach in practice, the strategy needs tighter risk controls regardless of returns.

### Digging deeper

Use the metric bundles to answer specific questions:

| Question | Bundle | Key fields |
|----------|--------|------------|
| "Is the return real or just market beta?" | `RiskMetrics()` | Alpha, Beta, RSquared |
| "How bad can it get?" | `RiskMetrics()` | ValueAtRisk, CVaR, UlcerIndex |
| "Is it beating the benchmark consistently?" | `RiskMetrics()` | InformationRatio, TrackingError, UpsideCaptureRatio, DownsideCaptureRatio |
| "Are the trades good?" | `TradeMetrics()` | WinRate, ProfitFactor, EdgeRatio, TradeCaptureRatio |
| "Is it tax-efficient?" | `TaxMetrics()` | TaxCostRatio, TaxDrag, LTCG vs STCG |
| "Can I spend from it?" | `WithdrawalMetrics()` | SafeWithdrawalRate, PerpetualWithdrawalRate |

### Windowed analysis

Headline numbers can hide regime changes. Use windowed queries to see how metrics evolve:

```go
// Compare Sharpe across time horizons
sharpe1y, _ := result.PerformanceMetric(portfolio.Sharpe).Window(portfolio.Years(1)).Value()
sharpe3y, _ := result.PerformanceMetric(portfolio.Sharpe).Window(portfolio.Years(3)).Value()
sharpeAll, _ := result.PerformanceMetric(portfolio.Sharpe).Value()

// Rolling 12-month return series for charting
series, _ := result.PerformanceMetric(portfolio.TWRR).Window(portfolio.Months(12)).Series()
```

A strategy with a strong full-history Sharpe but a weak trailing-1Y Sharpe may be decaying. A strategy whose rolling series oscillates wildly may not be robust.

### Trade-level analysis

When aggregate metrics look fine but something feels off, drill into individual trades:

```go
for _, trade := range result.TradeDetails() {
    fmt.Printf("%s: entry=%.2f exit=%.2f PnL=%.2f MFE=%.4f MAE=%.4f held=%v days\n",
        trade.Asset.Ticker, trade.EntryPrice, trade.ExitPrice,
        trade.PnL, trade.MFE, trade.MAE, trade.HoldDays)
}
```

High MFE with low realized profit means exits are too early. Deep MAE with losses means stops are too loose. See [Trade quality: MFE and MAE](#trade-quality-mfe-and-mae) for interpretation guidance.

### Comparing strategies

Run multiple strategies over the same period and compare their bundles. Key comparisons:

- **Risk-adjusted return**: Sharpe, Sortino, Calmar (higher is better)
- **Drawdown profile**: MaxDrawdown, AvgDrawdown, AvgDrawdownDays (lower is better)
- **Consistency**: KRatio, ProbabilisticSharpe (higher means more reliable)
- **Tax efficiency**: TaxCostRatio, ratio of LTCG to STCG (lower cost, more long-term gains is better)

## Individual metrics

Use `PerformanceMetric` to query a single metric. It returns a `PerformanceMetricQuery` builder with optional `Window` and then `Value` (scalar) or `Series` (rolling `[]float64`):

```go
// full history
sharpe, err := p.PerformanceMetric(Sharpe).Value()

// trailing 36-month window
sharpe36, err := p.PerformanceMetric(Sharpe).Window(Months(36)).Value()

// rolling series as []float64, compatible with gonum
series, err := p.PerformanceMetric(Sharpe).Series()
```

Windows are specified with `Days(n)`, `Months(n)`, or `Years(n)`. These handle calendar-aware durations correctly (months and years have variable lengths).

## Available metrics

Every metric below is a package-level variable implementing `PerformanceMetric`. Each can be used with `p.PerformanceMetric(metric).Value()` or `.Series()`.

**Return metrics:**

| Name | Description |
|------|-------------|
| `TWRR` | Time-weighted rate of return. Eliminates the effect of cash flows by compounding sub-period returns, showing pure investment performance independent of deposit/withdrawal timing. |
| `MWRR` | Money-weighted rate of return. Accounts for the timing and size of cash flows using XIRR. Unlike TWRR, this reflects the investor's actual dollar-weighted experience. |
| `CAGR` | Compound Annual Growth Rate. The annualized return that accounts for compounding. The standard way to compare returns across different time horizons. |
| `ActiveReturn` | Difference between portfolio return and benchmark return (strategy - benchmark). Measures the value added by active management. Highly dependent on appropriate benchmark selection. |

**Risk-adjusted ratios:**

| Name | Description |
|------|-------------|
| `Sharpe` | Risk-adjusted return: excess return over the risk-free rate divided by annualized standard deviation of returns. Higher is better. Uses monthly returns annualized to match Morningstar convention. |
| `Sortino` | Like Sharpe but uses downside deviation instead of total standard deviation. Only penalizes volatility from negative excess returns, treating upside volatility as desirable. |
| `SmartSharpe` | Sharpe ratio penalized for autocorrelation in returns using Lo (2002). When returns are serially correlated, the standard Sharpe overstates performance. Lower than Sharpe when returns exhibit positive autocorrelation. |
| `SmartSortino` | Sortino ratio penalized for autocorrelation using the same Lo (2002) correction as SmartSharpe. |
| `ProbabilisticSharpe` | Probability that the true Sharpe ratio exceeds zero, accounting for skewness and kurtosis. Based on Bailey and Lopez de Prado (2012). Values near 1.0 indicate high statistical confidence. |
| `Calmar` | Annualized return (CAGR) divided by maximum drawdown. Measures how much return compensates for the worst peak-to-trough decline. Typically computed over 36 months. |
| `Treynor` | Excess return per unit of systematic risk (beta). Similar to Sharpe but uses beta instead of standard deviation, isolating the reward for market risk rather than total risk. |
| `InformationRatio` | Mean active return divided by tracking error, annualized. Measures how consistently the portfolio outperforms the benchmark per unit of divergence from it. |
| `OmegaRatio` | Probability-weighted ratio of gains over losses. Captures the entire return distribution including higher moments, unlike Sharpe which uses only mean and variance. Above 1.0 means gains outweigh losses. |
| `RecoveryFactor` | Total compounded return divided by maximum drawdown. Measures how many times over the strategy recovered from its worst decline. A value of 3.0 means the strategy earned 3x its worst drawdown. |
| `GainToPainRatio` | Jack Schwager's metric: sum of all returns divided by absolute sum of negative returns. Intuitive measure of total profit relative to total pain. Values above 1.0 are good; above 2.0 is excellent. |
| `KRatio` | Consistency of returns over time. Computed as the slope of the log-VAMI regression line divided by the standard error of the slope. Higher values indicate steadier, more linear growth. |
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
| `CVaR` | Conditional Value at Risk (Expected Shortfall) at 95% confidence. The average loss in the worst 5% of periods. Superior to VaR because it captures the magnitude of extreme losses, not just their threshold. |
| `TailRatio` | Ratio of the 95th percentile return to the absolute 5th percentile return. Measures asymmetry between upside and downside tails. Above 1.0 means the upside tail is fatter. |

**Drawdown:**

| Name | Description |
|------|-------------|
| `MaxDrawdown` | Largest peak-to-trough decline in portfolio value. A max drawdown of 20% means the portfolio fell 20% from its high before recovering. The single most important risk metric for many investors. |
| `AvgDrawdown` | Mean drawdown across all points in the equity curve. Lower values indicate the portfolio spends less time in decline. |
| `AvgDrawdownDays` | Mean duration of drawdown episodes in trading days. Measures how long the strategy typically spends recovering from a decline before reaching a new peak. Complements AvgDrawdown (magnitude). |

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
| `KellyCriterion` | Optimal fraction of capital to risk per period based on historical win rate and payoff ratio. Computed as W - (1-W)/R where W is win rate and R is avg win / avg loss. Positive values suggest edge. |
| `ConsecutiveWins` | Longest streak of consecutive periods with positive returns. Useful for understanding momentum characteristics. |
| `ConsecutiveLosses` | Longest streak of consecutive periods with negative returns. Important for assessing the psychological difficulty of running a strategy. |
| `Exposure` | Fraction of periods with non-zero returns, indicating time invested in the market. A value of 1.0 means always invested. Essential for strategies that hold cash between signals. |

**Withdrawal rates:**

| Name | Description |
|------|-------------|
| `SafeWithdrawalRate` | Maximum constant annual withdrawal rate (as a percentage of initial balance) where the portfolio balance never reaches zero. Computed via circular bootstrap Monte Carlo simulation of historical returns. |
| `PerpetualWithdrawalRate` | Maximum constant annual withdrawal rate where the ending balance equals or exceeds the inflation-adjusted starting balance. Ensures the portfolio maintains real purchasing power indefinitely. |
| `DynamicWithdrawalRate` | Maximum annual withdrawal rate with dynamic adjustments: each year's withdrawal is the lesser of the inflation-adjusted initial withdrawal and the current balance times the rate. Adapts spending to portfolio performance. |

## Trade quality: MFE and MAE

Maximum Favorable Excursion (MFE) measures how far the price moved in a trade's favor before the position was closed. Maximum Adverse Excursion (MAE) measures how far the price moved against the position before exit. Both are expressed as fractions of the entry price: MFE is always non-negative and MAE is always non-positive.

Together they answer the question traditional metrics cannot: "Did the strategy exit well?" A strategy can have a positive win rate and still leave significant profit on the table if it consistently exits too early, or it can suffer unnecessary losses by holding too long through adverse moves. MFE and MAE make these exit quality problems visible.

### Interpreting results

- **High MFE with low realized profit** means exits are too early. The price moved significantly in the trade's favor, but the strategy gave back most of the gain before closing the position.
- **Deep MAE with realized loss** means stops are too loose. The position moved sharply against the trade before exit, suggesting tighter stop losses could reduce damage.
- **Edge ratio above 1.0** indicates favorable excursions typically exceed adverse excursions across all trades, suggesting the strategy has a positive trading edge. Below 1.0 means adverse moves dominate.
- **Capture ratio near 1.0** means the strategy captures most of the available favorable excursion as realized profit. Values near zero indicate premature exits that fail to harvest the move.

### Per-trade details

Call `TradeDetails()` on the portfolio to get a slice of `TradeDetail` structs, one per completed round-trip trade. Each struct includes entry/exit prices, dates, PnL, holding period, and the MFE/MAE values:

```go
for _, trade := range p.TradeDetails() {
    fmt.Printf("%s: PnL=%.2f  MFE=%.4f  MAE=%.4f  held=%v days\n",
        trade.Asset.Symbol(), trade.PnL, trade.MFE, trade.MAE, trade.HoldDays)
}
```

### MFE/MAE metrics

These six metrics summarize excursion data across all round-trip trades. They are included in the `TradeMetrics` bundle.

| Name | Description |
|------|-------------|
| `AverageMFE` | Mean Maximum Favorable Excursion across all round-trip trades, expressed as a fraction of entry price. Higher values indicate the strategy captures larger upside moves on average. |
| `AverageMAE` | Mean Maximum Adverse Excursion across all round-trip trades, expressed as a fraction of entry price. Values are typically negative; closer to zero indicates tighter risk control. |
| `MedianMFE` | Median Maximum Favorable Excursion across all round-trip trades. More robust to outliers than the mean, giving a better sense of typical upside potential. |
| `MedianMAE` | Median Maximum Adverse Excursion across all round-trip trades. Values are typically negative; the median is more robust to outliers than the mean. |
| `EdgeRatio` | Ratio of average MFE to the absolute value of average MAE. Values above 1.0 indicate favorable excursions typically exceed adverse excursions, suggesting a positive trading edge. |
| `TradeCaptureRatio` | Ratio of mean realized return percentage to mean MFE. Values close to 1.0 indicate the strategy captures most of the available favorable excursion; values near 0 indicate premature exits. |

## Metric bundles

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
| `TaxDrag` | Percentage of pre-tax return lost to trading-related taxes (excludes dividends) |

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
| `AverageMFE` | Mean Maximum Favorable Excursion across all trades |
| `AverageMAE` | Mean Maximum Adverse Excursion across all trades |
| `MedianMFE` | Median Maximum Favorable Excursion across all trades |
| `MedianMAE` | Median Maximum Adverse Excursion across all trades |
| `EdgeRatio` | Ratio of average MFE to absolute average MAE |
| `TradeCaptureRatio` | Ratio of mean realized return to mean MFE |

**WithdrawalMetrics** -- sustainable spending rates derived from Monte Carlo simulation of historical returns (`p.WithdrawalMetrics()`):

| Name | Description |
|------|-------------|
| `SafeWithdrawalRate` | Maximum withdrawal rate where balance never hits zero |
| `PerpetualWithdrawalRate` | Maximum withdrawal rate preserving inflation-adjusted balance |
| `DynamicWithdrawalRate` | Maximum withdrawal rate with dynamic spending adjustments |

## Custom metrics

Anyone can define a custom performance metric by implementing the `PerformanceMetric` interface:

```go
type PerformanceMetric interface {
    Name() string
    Description() string
    Compute(a *Account, window *Period) (float64, error)
    ComputeSeries(a *Account, window *Period) ([]float64, error)
}
```

`Compute` receives the full `*Account` so it can access the equity curve, benchmark data, risk-free rate, and transaction history -- not just raw transactions. This is what allows metrics like `Alpha` and `Beta` to reference the benchmark, and `Sharpe` to use the risk-free rate. Both methods return errors for cases like insufficient data or missing benchmark prices.

Each built-in metric is an unexported struct with an exported package-level variable (e.g., `var Sharpe = sharpe{}`). Custom metrics follow the same pattern:

```go
type myMetric struct{}
func (myMetric) Name() string { return "MyMetric" }
func (myMetric) Description() string { return "What this metric measures." }
func (myMetric) Compute(a *Account, window *Period) (float64, error) { ... }
func (myMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) { ... }

var MyMetric = myMetric{}

// use it
val := p.PerformanceMetric(MyMetric).Window(Years(3)).Value()
```

## Using metrics during computation

While performance metrics are most commonly examined after a backtest completes, they are available during computation. A strategy might use them to adjust its behavior:

```go
func (s *Adaptive) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    dd, err := port.PerformanceMetric(MaxDrawdown).Value()
    if err == nil && dd > 0.15 {
        // drawdown exceeds 15%, reduce exposure
        return nil
    }

    // normal allocation logic
    // ...
    return nil
}
```

This is less common but occasionally useful for strategies that adapt to their own performance.
