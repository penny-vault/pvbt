# Signal Reference

Signals are reusable computations that derive new time series from market data. They live in the `signal` package as plain functions and return `*data.DataFrame` values — one column per asset — that can be used directly in weighting functions or composed through DataFrame arithmetic.

## Overview

A signal takes a universe and returns a new DataFrame with one column per asset containing the computed score at the current date. Because signals operate on market data rather than portfolio state, a signal like `signal.Momentum(ctx, u, portfolio.Months(12))` produces the same output regardless of what the portfolio currently holds or how it has traded. This distinguishes signals from portfolio performance metrics, which depend on a portfolio's specific trading history.

Signals are plain functions. There is no registration, no interface to implement, and no special type. Any function with a compatible signature works as a signal (see [Custom signals](#custom-signals)).

## Built-in signals

| Signal | Signature | Description |
|--------|-----------|-------------|
| `Momentum` | `(ctx, u, period, metrics...)` | Percent change over a lookback period |
| `EarningsYield` | `(ctx, u, t...)` | Earnings per share divided by price |
| `Volatility` | `(ctx, u, period, metrics...)` | Rolling standard deviation of returns |
| `RSI` | `(ctx, u, period, metrics...)` | Relative Strength Index (Wilder smoothing, 0--100) |
| `MACD` | `(ctx, u, fast, slow, signalPeriod, metrics...)` | MACD line, signal line, and histogram |
| `BollingerBands` | `(ctx, u, period, numStdDev, metrics...)` | Upper, middle, and lower Bollinger Bands |
| `Crossover` | `(ctx, u, fastPeriod, slowPeriod, metrics...)` | Fast/slow SMA crossover indicator (+1/--1) |
| `ATR` | `(ctx, u, period)` | Average True Range (Wilder smoothing) |
| `StochasticFast` | `(ctx, u, period)` | Fast Stochastic Oscillator (%K and %D, 0--100) |
| `StochasticSlow` | `(ctx, u, period, smoothing)` | Slow Stochastic Oscillator (smoothed %K and %D, 0--100) |
| `WilliamsR` | `(ctx, u, period)` | Williams %R momentum oscillator (-100 to 0) |
| `CCI` | `(ctx, u, period)` | Commodity Channel Index (unbounded) |
| `OBV` | `(ctx, u, period)` | On-Balance Volume (cumulative) |
| `VWMA` | `(ctx, u, period)` | Volume-Weighted Moving Average |
| `AccumulationDistribution` | `(ctx, u, period)` | Accumulation/Distribution line (cumulative) |
| `CMF` | `(ctx, u, period)` | Chaikin Money Flow (-1 to 1) |
| `MFI` | `(ctx, u, period)` | Money Flow Index (0 to 100) |
| `ZScore` | `(ctx, u, period, metrics...)` | Z-score of current value relative to rolling mean (unbounded) |
| `HurstRS` | `(ctx, u, period)` | Hurst exponent via Rescaled Range analysis (0 to 1) |
| `HurstDFA` | `(ctx, u, period)` | Hurst exponent via Detrended Fluctuation Analysis (0 to 1) |
| `PairsResidual` | `(ctx, u, period, refUniverse)` | Z-score of OLS regression residuals vs reference assets |
| `PairsRatio` | `(ctx, u, period, refUniverse)` | Z-score of price ratio vs reference assets |

## Signal reference

### Trend and momentum

#### Momentum

Computes the percent change of each asset over a lookback period. This is the simplest momentum signal: how much did the price change from the start of the window to the end?

```go
df := signal.Momentum(ctx, u, portfolio.Months(12))
```

**Signature:** `Momentum(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame`

**Parameters:**
- `period` — lookback window (e.g. `portfolio.Months(12)` for 12-month momentum)
- `metrics` — optional; defaults to `data.MetricClose`

**Output metric:** `MomentumSignal`

**Value range:** Unbounded; positive values indicate price appreciation, negative indicate decline.

---

#### Crossover

Computes a moving average crossover signal. Returns +1 when the fast SMA is above the slow SMA and -1 when it is below. This indicates trend direction based on two moving average periods.

```go
df := signal.Crossover(ctx, u, portfolio.Days(50), portfolio.Days(200))
```

**Signature:** `Crossover(ctx context.Context, u universe.Universe, fastPeriod, slowPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame`

**Parameters:**
- `fastPeriod` — lookback for the fast (shorter) SMA
- `slowPeriod` — lookback for the slow (longer) SMA
- `metrics` — optional; defaults to `data.MetricClose`

**Output metrics:** `CrossoverFastSignal`, `CrossoverSlowSignal`, `CrossoverSignal` (+1 or -1)

**Value range:** `CrossoverSignal` is +1 or -1. Fast and slow SMA values are in price units.

---

#### MACD

Computes Moving Average Convergence Divergence. The MACD line is the difference between a fast and slow EMA. The signal line is an EMA of the MACD line. The histogram is the difference between the MACD line and the signal line.

```go
df := signal.MACD(ctx, u, portfolio.Days(12), portfolio.Days(26), portfolio.Days(9))
```

**Signature:** `MACD(ctx context.Context, u universe.Universe, fast, slow, signalPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame`

**Parameters:**
- `fast` — fast EMA period (typically 12 days)
- `slow` — slow EMA period (typically 26 days)
- `signalPeriod` — signal line EMA period (typically 9 days)
- `metrics` — optional; defaults to `data.MetricClose`

**Output metrics:** `MACDLineSignal`, `MACDSignalLineSignal`, `MACDHistogramSignal`

**Value range:** Unbounded; values are in the same units as the input price.

---

### Volatility

#### Volatility

Computes the rolling standard deviation of returns for each asset. Higher values indicate more volatile assets.

```go
df := signal.Volatility(ctx, u, portfolio.Days(20))
```

**Signature:** `Volatility(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame`

**Parameters:**
- `period` — lookback window for the rolling standard deviation
- `metrics` — optional; defaults to `data.MetricClose`

**Output metric:** `VolatilitySignal`

**Value range:** Non-negative; expressed as a fraction (e.g. 0.02 means 2% daily volatility).

---

#### BollingerBands

Computes upper, middle, and lower Bollinger Bands. The middle band is the SMA of price over the period. The upper and lower bands are placed `numStdDev` standard deviations above and below the middle band.

```go
df := signal.BollingerBands(ctx, u, portfolio.Days(20), 2.0)
```

**Signature:** `BollingerBands(ctx context.Context, u universe.Universe, period portfolio.Period, numStdDev float64, metrics ...data.Metric) *data.DataFrame`

**Parameters:**
- `period` — SMA and standard deviation lookback window
- `numStdDev` — number of standard deviations for the band width (typically 2.0)
- `metrics` — optional; defaults to `data.MetricClose`

**Output metrics:** `BollingerUpperSignal`, `BollingerMiddleSignal`, `BollingerLowerSignal`

**Value range:** Values are in the same units as the input price.

---

#### ATR

Computes the Average True Range using Wilder's smoothing method. ATR measures volatility as the average distance between high and low prices, accounting for gaps from the prior close.

```go
df := signal.ATR(ctx, u, portfolio.Days(14))
```

**Signature:** `ATR(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — smoothing window (Wilder's method)

**Output metric:** `ATRSignal`

**Value range:** Non-negative; expressed in price units.

---

### Oscillators

#### RSI

Computes the Relative Strength Index using Wilder's smoothing method. RSI measures the magnitude of recent gains relative to recent losses.

```go
df := signal.RSI(ctx, u, portfolio.Days(14))
```

**Signature:** `RSI(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame`

**Parameters:**
- `period` — lookback window; `period.N - 1` price changes are used
- `metrics` — optional; defaults to `data.MetricClose`

**Output metric:** `RSISignal`

**Value range:** 0 to 100. Values above 70 are conventionally considered overbought; values below 30 are considered oversold.

---

#### StochasticFast

Computes the Fast Stochastic Oscillator. %K measures where the current close sits within the high-low range over the period. %D is a 3-period SMA of %K.

```go
df := signal.StochasticFast(ctx, u, portfolio.Days(14))
k := df.Metrics(signal.StochasticKSignal)
d := df.Metrics(signal.StochasticDSignal)
```

**Signature:** `StochasticFast(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window for the %K calculation

**Output metrics:** `StochasticKSignal` (raw %K), `StochasticDSignal` (3-period SMA of %K)

**Value range:** 0 to 100. Values above 80 are conventionally overbought; values below 20 are oversold.

---

#### StochasticSlow

Computes the Slow Stochastic Oscillator. Slow %K is an SMA of the raw Fast %K over the smoothing period, which reduces noise compared to the fast variant. Slow %D is a 3-period SMA of Slow %K.

```go
df := signal.StochasticSlow(ctx, u, portfolio.Days(14), portfolio.Days(3))
slowK := df.Metrics(signal.StochasticSlowKSignal)
slowD := df.Metrics(signal.StochasticSlowDSignal)
```

**Signature:** `StochasticSlow(ctx context.Context, u universe.Universe, period, smoothing portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window for the raw %K calculation
- `smoothing` — SMA window applied to raw %K to produce Slow %K (typically 3 days)

**Output metrics:** `StochasticSlowKSignal` (smoothed %K), `StochasticSlowDSignal` (3-period SMA of Slow %K)

**Value range:** 0 to 100. Values above 80 are conventionally overbought; values below 20 are oversold.

---

#### WilliamsR

Computes Williams %R, which measures where the current close sits relative to the high-low range over the lookback period. It is the inverse of the Stochastic %K, scaled to -100..0.

```go
df := signal.WilliamsR(ctx, u, portfolio.Days(14))
```

**Signature:** `WilliamsR(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window

**Output metric:** `WilliamsRSignal`

**Value range:** -100 to 0. A value of 0 means the close is at the highest high of the period; -100 means the close is at the lowest low. Values above -20 are conventionally overbought; values below -80 are oversold.

---

#### CCI

Computes the Commodity Channel Index. CCI measures how far the typical price (average of high, low, close) has deviated from its simple moving average, scaled by the mean absolute deviation. Lambert's constant (0.015) is applied so that roughly 70--80% of values fall between -100 and +100 under normal conditions.

```go
df := signal.CCI(ctx, u, portfolio.Days(20))
```

**Signature:** `CCI(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window for the SMA and mean absolute deviation

**Output metric:** `CCISignal`

**Value range:** Unbounded. Values above +100 suggest overbought conditions; values below -100 suggest oversold conditions.

---

### Volume

#### OBV

Computes On-Balance Volume, a cumulative indicator that adds volume on up-close bars and subtracts volume on down-close bars. Rising OBV confirms an uptrend; divergence between OBV and price warns of potential reversal.

```go
df := signal.OBV(ctx, u, portfolio.Days(50))
```

**Signature:** `OBV(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window

**Output metric:** `OBVSignal`

**Value range:** Unbounded; the absolute level is less meaningful than the direction and trend of the series.

---

#### VWMA

Computes the Volume-Weighted Moving Average. Unlike a simple moving average, VWMA gives more weight to bars with higher volume. Price trading above the VWMA suggests buying pressure dominates over the period.

```go
df := signal.VWMA(ctx, u, portfolio.Days(20))
```

**Signature:** `VWMA(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window

**Output metric:** `VWMASignal`

**Value range:** Values are in price units.

---

#### AccumulationDistribution

Computes the Accumulation/Distribution line, a cumulative volume indicator that uses the close position within the high-low range to weight volume. A close near the high adds volume (accumulation); a close near the low subtracts volume (distribution).

```go
df := signal.AccumulationDistribution(ctx, u, portfolio.Days(50))
```

**Signature:** `AccumulationDistribution(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window

**Output metric:** `AccumulationDistributionSignal`

**Value range:** Unbounded; the direction and trend of the series matter more than the absolute level.

---

#### CMF

Computes Chaikin Money Flow. CMF is the ratio of cumulative Money Flow Volume to cumulative volume over a fixed window, measuring buying and selling pressure over the period.

```go
df := signal.CMF(ctx, u, portfolio.Days(21))
```

**Signature:** `CMF(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window (conventionally 20 or 21 bars)

**Output metric:** `CMFSignal`

**Value range:** -1 to +1. Positive values indicate buying pressure; negative values indicate selling pressure.

---

#### MFI

Computes the Money Flow Index, a volume-weighted RSI. MFI classifies money flow as positive or negative based on the direction of the typical price relative to the prior period.

```go
df := signal.MFI(ctx, u, portfolio.Days(14))
```

**Signature:** `MFI(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window (conventionally 14 bars)

**Output metric:** `MFISignal`

**Value range:** 0 to 100. Values above 80 are conventionally considered overbought; values below 20 are considered oversold.

---

### Mean reversion

#### ZScore

Computes the z-score of each asset's current value relative to its rolling mean over the lookback window. A z-score measures how many standard deviations the current observation is above or below the mean: z = (current - mean) / stddev.

```go
df := signal.ZScore(ctx, u, portfolio.Days(20))
```

**Signature:** `ZScore(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame`

**Parameters:**
- `period` — lookback window for computing the rolling mean and standard deviation
- `metrics` — optional; defaults to `data.MetricClose`

**Output metric:** `ZScoreSignal`

**Value range:** Unbounded. Positive values indicate the current price is above the rolling mean; negative values indicate it is below. Returns an error on a constant series (zero standard deviation) or when there is insufficient data.

---

#### HurstRS

Computes the Hurst exponent using Rescaled Range (R/S) analysis. The R/S statistic is computed at multiple sub-period sizes and regressed against log(n) to estimate the Hurst exponent.

```go
df := signal.HurstRS(ctx, u, portfolio.Days(60))
```

**Signature:** `HurstRS(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window; at least ~20 data points are required for a reliable estimate

**Output metric:** `HurstRSSignal`

**Value range:** 0 to 1. H < 0.5 indicates mean-reverting behavior, H = 0.5 indicates a random walk, and H > 0.5 indicates trending behavior.

---

#### HurstDFA

Computes the Hurst exponent using Detrended Fluctuation Analysis (DFA). DFA builds a cumulative deviation profile, fits linear trends per segment, and computes the RMS fluctuation as a function of segment size. This method is more robust to short-term correlations than R/S analysis.

```go
df := signal.HurstDFA(ctx, u, portfolio.Days(60))
```

**Signature:** `HurstDFA(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window; at least ~20 data points are required for a reliable estimate

**Output metric:** `HurstDFASignal`

**Value range:** 0 to 1. Interpretation is the same as `HurstRS`: H < 0.5 is mean-reverting, H = 0.5 is a random walk, H > 0.5 is trending.

---

#### PairsResidual

Computes the z-score of OLS regression residuals between each primary asset and each reference asset over the lookback window. For each (primary, reference) pair, the signal runs a linear regression of returns and z-scores the residuals to identify divergence.

```go
df := signal.PairsResidual(ctx, u, portfolio.Days(60), refUniverse)
residuals := df.Metrics(signal.PairsResidualSignal("SPY"))
```

**Signature:** `PairsResidual(ctx context.Context, u universe.Universe, period portfolio.Period, refUniverse universe.Universe) *data.DataFrame`

**Parameters:**
- `period` — lookback window for the regression and z-score calculation
- `refUniverse` — universe of reference assets to regress against

**Output metrics:** One metric per reference asset, named `PairsResidual_{Ticker}` (e.g. `PairsResidual_SPY`).

**Value range:** Unbounded. Positive values indicate the primary asset is rich relative to the reference; negative values indicate it is cheap.

---

#### PairsRatio

Computes the z-score of the price ratio between each primary asset and each reference asset over the lookback window. This is a simpler alternative to `PairsResidual` that does not require a regression.

```go
df := signal.PairsRatio(ctx, u, portfolio.Days(60), refUniverse)
ratio := df.Metrics(signal.PairsRatioSignal("SPY"))
```

**Signature:** `PairsRatio(ctx context.Context, u universe.Universe, period portfolio.Period, refUniverse universe.Universe) *data.DataFrame`

**Parameters:**
- `period` — lookback window for the z-score calculation
- `refUniverse` — universe of reference assets to compare against

**Output metrics:** One metric per reference asset, named `PairsRatio_{Ticker}` (e.g. `PairsRatio_SPY`).

**Value range:** Unbounded. Positive values indicate the primary asset is rich relative to the reference; negative values indicate it is cheap. Interpretation is the same as `PairsResidual` but uses a simpler ratio rather than regression residuals.

---

### Fundamental

#### EarningsYield

Computes earnings yield as earnings per share divided by price. Higher values indicate cheaper valuations relative to earnings.

```go
df := signal.EarningsYield(ctx, u)
```

**Signature:** `EarningsYield(ctx context.Context, u universe.Universe, t ...time.Time) *data.DataFrame`

**Parameters:**
- `t` — optional point-in-time override; defaults to the universe's current date

**Output metric:** `EarningsYieldSignal`

**Value range:** Non-negative for profitable companies. Expressed as a fraction (e.g. 0.05 means 5% earnings yield).

---

## Composing signals

Because signals return DataFrames, they compose naturally through DataFrame arithmetic. For example, a composite momentum signal averaging three lookback periods:

```go
mom3 := signal.Momentum(ctx, u, portfolio.Months(3))
mom6 := signal.Momentum(ctx, u, portfolio.Months(6))
mom12 := signal.Momentum(ctx, u, portfolio.Months(12))
composite := mom3.Add(mom6).Add(mom12).DivScalar(3)
```

Signals from different categories can also be combined. For example, a rank-based approach that combines momentum with low volatility:

```go
mom := signal.Momentum(ctx, u, portfolio.Months(12))
vol := signal.Volatility(ctx, u, portfolio.Days(60))

// Negate volatility so lower vol = higher score, then average with momentum.
score := mom.Sub(vol).DivScalar(2)
```

## Custom signals

A signal is any function that takes a context and universe and returns a `*data.DataFrame`. There is no interface to implement:

```go
func BookToPrice(ctx context.Context, u universe.Universe) *data.DataFrame {
    df, err := u.At(ctx, u.CurrentDate(), data.BookValue, data.MetricClose)
    if err != nil {
        return data.WithErr(err)
    }
    book := df.Metrics(data.BookValue)
    price := df.Metrics(data.MetricClose)
    return book.Div(price)
}
```

Use `data.WithErr` to return an error-carrying DataFrame when the signal cannot be computed. This lets callers use `df.Err()` consistently regardless of whether the error occurred in a built-in or custom signal.

## Error handling

Every signal returns a `*data.DataFrame`. When a required metric is missing or insufficient data is available, the returned DataFrame carries an error. Always check before using the result:

```go
df := signal.RSI(ctx, u, portfolio.Days(14))
if df.Err() != nil {
    return df.Err()
}
```

Errors propagate through chained DataFrame operations, so it is safe to chain operations and check once at the end:

```go
result := signal.Momentum(ctx, u, portfolio.Months(12)).Add(
    signal.Momentum(ctx, u, portfolio.Months(3)),
).DivScalar(2)
if result.Err() != nil {
    return result.Err()
}
```
