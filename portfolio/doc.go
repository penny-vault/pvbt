// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package portfolio is where strategy decisions become trades. It has two
// responsibilities: construction (turning allocation decisions into orders)
// and measurement (computing performance metrics over the resulting equity
// curve). Strategy code receives the [Portfolio] interface during Compute
// calls. The engine uses the [PortfolioManager] interface for external
// events such as dividends and price updates. Both interfaces are
// implemented by [Account].
//
// # Allocation and PortfolioPlan
//
// An [Allocation] pairs a date with a map of assets to target weight
// percentages (summing to 1.0). A [PortfolioPlan] is a time-ordered slice
// of Allocations describing how the portfolio should be invested over
// time. Building a PortfolioPlan is a two-step pipeline: selection
// chooses which assets to hold, then weighting assigns each asset its
// target weight.
//
// # Selection
//
// The [Selector] interface has a single method, Select, which takes a
// DataFrame and inserts a [Selected] metric column. Values greater than
// zero mean the asset is chosen at that timestep; zero or NaN means it is
// not. The DataFrame is mutated in place and the same pointer is returned.
//
// [MaxAboveZero] is a built-in Selector that picks the asset with the
// highest positive value in a given metric column at each timestep. If no
// asset qualifies, optional fallback assets are inserted and marked as
// selected.
//
// [TopN] selects the N assets with the highest values in a given metric
// column at each timestep. [BottomN] selects the N assets with the lowest
// values. Both exclude NaN values from ranking and select fewer than N
// assets when not enough valid values exist.
//
// # Weighting
//
// [EqualWeight] builds a [PortfolioPlan] by assigning equal weights to all
// assets whose Selected value is greater than zero at each timestep. The
// magnitude of the Selected value is ignored; only its sign matters.
//
// [WeightedBySignal] builds a [PortfolioPlan] by weighting each selected
// asset proportionally to the values in a named metric column. Weights are
// normalized to sum to 1.0. Zero, NaN, and negative metric values are
// discarded. If all selected assets have non-positive metric values at a
// timestep, equal weight is assigned among the selected assets as a
// fallback.
//
// [InverseVolatility] weights each selected asset inversely proportional to
// its trailing volatility over a configurable lookback window (default 60
// calendar days). Assets with lower volatility receive larger weights. Falls
// back to equal weight when all volatilities are zero.
//
// [MarketCapWeighted] weights each selected asset proportionally to its
// market capitalization. If the MarketCap metric is not present in the
// DataFrame, it is fetched automatically via the DataFrame's DataSource.
// Falls back to equal weight when all market caps are zero or NaN.
//
// [RiskParityFast] approximates equal risk contribution using a single-pass
// adjustment: it starts with inverse volatility weights and divides each
// weight by its marginal risk contribution from the covariance matrix, then
// normalizes. This produces better risk balance than pure inverse volatility
// when assets are correlated, but does not guarantee exact equal risk
// contribution.
//
// [RiskParity] finds weights where each asset contributes equally to total
// portfolio risk using iterative multiplicative optimization. It runs up to
// 1000 iterations and returns the best result found, logging a warning if
// convergence is not reached.
//
// # Construction
//
// Two approaches are available for turning decisions into trades:
//
//   - [Portfolio.RebalanceTo] (declarative): compute a selection and
//     weighting, then pass the resulting [PortfolioPlan]. The engine diffs
//     current holdings against each target and generates the necessary
//     buy and sell orders.
//   - [Portfolio.Order] (imperative): place individual orders with fine-
//     grained control. Optional modifiers adjust order type and lifetime.
//
// The two approaches can be mixed freely within the same strategy.
//
// # Order Modifiers
//
// [OrderModifier] values adjust the behavior of an [Portfolio.Order] call.
// They fall into two categories.
//
// Order types control price conditions:
//
//   - Market (default when no modifier is specified): execute at the
//     current market price.
//   - [Limit]: set a maximum buy price or minimum sell price.
//   - [Stop]: trigger a market order when the price reaches a threshold.
//   - StopLimit (combine [Stop] and [Limit]): trigger a limit order when
//     the stop price is reached.
//
// Time in force controls how long the order remains active:
//
//   - [DayOrder] (default): cancel at market close if not executed.
//   - [GoodTilCancel]: keep open until filled or explicitly cancelled.
//   - [GoodTilDate]: keep open until a specified date.
//   - [FillOrKill]: fill entirely or cancel immediately.
//   - [ImmediateOrCancel]: fill as many shares as possible immediately
//     and cancel the remainder.
//   - [OnTheOpen]: fill only at the opening price.
//   - [OnTheClose]: fill only at the closing price.
//
// Bracket and OCO modifiers link orders for coordinated execution.
// These modifiers are only supported through batch submission
// (Batch.Order), not through direct Account.Order calls:
//
//   - [WithBracket]: attach stop-loss and take-profit exits to an entry
//     order. The exits activate as an OCO pair when the entry fills.
//     Exit targets are specified as absolute prices ([StopLossPrice],
//     [TakeProfitPrice]) or percentage offsets from the fill price
//     ([StopLossPercent], [TakeProfitPercent]).
//   - [OCO]: create two linked orders from a single Batch.Order call.
//     Each leg is defined by an [OCOLeg] built with [StopLeg] or
//     [LimitLeg]. When one leg fills, the other is cancelled.
//
// Trade annotation:
//
//   - [WithJustification]: attach a human-readable explanation to the
//     resulting transaction. The string is copied onto every
//     [Transaction] the order produces.
//
// # Justifications vs Annotations
//
// Strategies produce two kinds of explanatory output. They serve different
// audiences and answer different questions:
//
//   - A justification is a human-readable sentence explaining why a
//     particular trade was made. It appears on each [Transaction] and is
//     meant to be read by a person reviewing the trade log. Think of it
//     as the caption on a trade: "momentum crossover signal" or "rebalance
//     after earnings."
//   - An annotation is a step-level key-value entry that captures an
//     intermediate computation -- a momentum score, a signal strength, a
//     bond fraction. Annotations are meant for debugging and analysis: they
//     let you reconstruct what the strategy computed on a given date and
//     understand why it reached the decision it did.
//
// In short: justifications tell you what the strategy decided and why in
// plain language; annotations give you the raw numbers behind that decision.
//
// # Justifications
//
// Set the Justification field on an [Allocation] to label every transaction
// generated by [Portfolio.RebalanceTo], or pass [WithJustification] as an
// [OrderModifier] to [Portfolio.Order]. Justifications are stored on the
// [Transaction] and persisted in the SQLite output.
//
// # Annotations
//
// Call [Portfolio.Annotate] during Compute to record a key-value entry at the
// current timestep. Call [Portfolio.Annotations] to read the full log.
//
// To annotate an entire [data.DataFrame] at once, use
// [data.DataFrame.Annotate] which decomposes every non-NaN cell into a
// "TICKER/Metric" keyed entry:
//
//	momentumDF.Annotate(portfolio)
//
// The [Annotation] struct stores a Unix-seconds timestamp, a key, and a
// string value. Annotations are append-only and persisted in the SQLite
// output.
//
// # Reading Portfolio State
//
// The [Portfolio] interface exposes read-only access to the current state:
//
//   - [Portfolio.Cash]: current cash balance.
//   - [Portfolio.Value]: total portfolio value (cash plus holdings marked
//     to current prices).
//   - [Portfolio.Position]: quantity held of a specific asset.
//   - [Portfolio.PositionValue]: current market value of a position.
//   - [Portfolio.Holdings]: iterate over all current positions, calling a
//     function with each asset and its held quantity.
//
// # Transaction Log
//
// Every event that changes the portfolio produces a [Transaction]. The
// Type field identifies the kind of event using one of six
// [TransactionType] constants:
//
//   - [BuyTransaction]: purchase of an asset.
//   - [SellTransaction]: sale of an asset.
//   - [DividendTransaction]: dividend payment received.
//   - [FeeTransaction]: fee or commission charged.
//   - [DepositTransaction]: cash added to the portfolio.
//   - [WithdrawalTransaction]: cash removed from the portfolio.
//
// The Qualified flag on a [Transaction] indicates whether a dividend meets
// the IRS 60-day holding period requirement for preferential tax rates.
// It is set automatically by Record based on the position's holding
// duration.
//
// The Amount field represents the total cash impact of the transaction.
// Positive values are cash inflows (sells, dividends, deposits); negative
// values are cash outflows (buys, fees, withdrawals).
//
// # Risk Controls
//
// Risk controls are configured by the operator, not the strategy author.
// The strategy expresses its investment intent through RebalanceTo and
// Order calls; a separate risk layer enforces position limits, drawdown
// thresholds, and other constraints.
//
// # Performance Measurement
//
// Use the [Portfolio.PerformanceMetric] method to query any registered
// metric. It returns a [PerformanceMetricQuery] builder. Call Value to
// get a single scalar over the full history, or chain Window first to
// set a lookback period. Call Series to get a rolling time series.
//
//	sharpe, err := p.PerformanceMetric(Sharpe).Window(Months(36)).Value()
//
// Window helpers include [Days], [Months], and [Years].
//
// # Performance Metrics
//
// The following metrics are available. Each is a package-level variable
// implementing the [PerformanceMetric] interface.
//
// Return metrics:
//
//   - [TWRR]: time-weighted rate of return, measuring portfolio growth
//     independent of the timing and size of deposits and withdrawals.
//   - [MWRR]: money-weighted rate of return (also called internal rate of
//     return), measuring the actual return earned on invested capital
//     including the effect of cash flow timing.
//   - [CAGR]: compound annual growth rate.
//   - [ActiveReturn]: excess return over the benchmark.
//
// Risk-adjusted ratios:
//
//   - [Sharpe]: excess return per unit of total volatility.
//   - [Sortino]: excess return per unit of downside volatility.
//   - [SmartSharpe]: Sharpe ratio adjusted for autocorrelation.
//   - [SmartSortino]: Sortino ratio adjusted for autocorrelation.
//   - [ProbabilisticSharpe]: probability that the Sharpe ratio exceeds a threshold.
//   - [Calmar]: annualized return divided by maximum drawdown.
//   - [Treynor]: excess return per unit of beta.
//   - [InformationRatio]: active return per unit of tracking error.
//   - [OmegaRatio]: probability-weighted ratio of gains to losses.
//   - [RecoveryFactor]: cumulative return divided by maximum drawdown.
//   - [GainToPainRatio]: sum of returns divided by sum of absolute losses.
//   - [KRatio]: measures how consistently returns grow over time by
//     fitting a line to the cumulative return curve; higher values
//     indicate steadier growth.
//   - [KellerRatio]: measures the quality and persistence of an asset's
//     momentum, distinguishing smooth trends from noisy ones.
//
// Risk and volatility:
//
//   - [StdDev]: standard deviation of returns.
//   - [Beta]: sensitivity to benchmark movements.
//   - [Alpha]: excess return over CAPM prediction.
//   - [DownsideDeviation]: volatility of returns below the risk-free rate.
//   - [TrackingError]: standard deviation of return difference vs benchmark.
//   - [UlcerIndex]: downside risk that accounts for both the depth and
//     duration of drawdowns, penalizing long, deep declines more heavily.
//   - [ValueAtRisk]: maximum expected loss at a given confidence level
//     (e.g., "95% of the time, losses will not exceed X").
//   - [CVaR]: conditional value at risk -- the average loss in the worst
//     cases beyond the ValueAtRisk threshold (the expected shortfall).
//   - [TailRatio]: ratio of right-tail gains to left-tail losses.
//
// Drawdown:
//
//   - [MaxDrawdown]: largest peak-to-trough decline.
//   - [AvgDrawdown]: average drawdown depth.
//   - [AvgDrawdownDays]: average number of days in drawdown.
//
// Distribution shape:
//
//   - [ExcessKurtosis]: tail risk relative to a normal distribution.
//   - [Skewness]: asymmetry of the return distribution.
//
// Benchmark comparison:
//
//   - [RSquared]: how well returns are explained by the benchmark.
//   - [UpsideCaptureRatio]: percentage of benchmark gains captured.
//   - [DownsideCaptureRatio]: percentage of benchmark losses captured.
//
// Win/loss profile:
//
//   - [NPositivePeriods]: percentage of periods with positive returns.
//   - [GainLossRatio]: average gain divided by average loss.
//   - [KellyCriterion]: optimal fraction of capital to risk per trade,
//     derived from the win rate and average payoff ratio.
//   - [ConsecutiveWins]: longest streak of consecutive winning periods.
//   - [ConsecutiveLosses]: longest streak of consecutive losing periods.
//   - [Exposure]: fraction of time the portfolio is invested.
//
// Withdrawal rates:
//
//   - [SafeWithdrawalRate]: maximum annual rate where the balance never hits zero.
//   - [PerpetualWithdrawalRate]: maximum rate preserving inflation-adjusted balance.
//   - [DynamicWithdrawalRate]: maximum rate with dynamic spending adjustments.
//
// # Metric Bundles
//
// Convenience methods compute groups of related metrics in a single call:
//
//   - [Portfolio.Summary]: TWRR, MWRR, Sharpe, Sortino, Calmar,
//     MaxDrawdown, StdDev.
//   - [Portfolio.RiskMetrics]: Beta, Alpha, TrackingError,
//     DownsideDeviation, InformationRatio, Treynor, UlcerIndex,
//     ExcessKurtosis, Skewness, RSquared, ValueAtRisk,
//     UpsideCaptureRatio, DownsideCaptureRatio.
//   - [Portfolio.TaxMetrics]: LTCG, STCG, UnrealizedLTCG,
//     UnrealizedSTCG, QualifiedDividends, NonQualifiedIncome,
//     TaxCostRatio.
//   - [Portfolio.TradeMetrics]: WinRate, AverageWin, AverageLoss,
//     ProfitFactor, AverageHoldingPeriod, Turnover, NPositivePeriods,
//     GainLossRatio.
//   - [Portfolio.WithdrawalMetrics]: SafeWithdrawalRate,
//     PerpetualWithdrawalRate, DynamicWithdrawalRate.
//
// # Custom Metrics
//
// Implement the [PerformanceMetric] interface to define a custom metric.
// The interface has four methods: Name, Description, Compute, and
// ComputeSeries. The standard pattern uses an unexported struct with an
// exported package-level variable:
//
//	type myMetric struct{}
//
//	func (myMetric) Name() string                                         { return "MyMetric" }
//	func (myMetric) Description() string                                  { return "..." }
//	func (myMetric) Compute(a *Account, w *Period) (float64, error)       { /* ... */ }
//	func (myMetric) ComputeSeries(a *Account, w *Period) ([]float64, error) { /* ... */ }
//
//	var MyMetric PerformanceMetric = myMetric{}
//
// Register custom metrics at Account construction time with [WithMetric].
//
// # Factor Analysis
//
// Factor analysis (decomposing returns into exposure to known risk
// factors such as market, size, value, momentum, and quality) is planned
// but not yet implemented. When available it will allow strategies to
// understand how much of their return comes from systematic factor
// tilts versus genuine alpha.
package portfolio
