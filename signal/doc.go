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

// Package signal provides reusable computations that derive new time series
// from market data (prices, volume, fundamentals, economic indicators).
// Signals live as plain functions. Each takes a [universe.Universe] and
// returns a new [data.DataFrame] with one column per asset containing the
// computed score.
//
// Signals operate on market data, not portfolio state. A signal like
// [Momentum] produces the same output regardless of current portfolio
// holdings. This distinguishes signals from portfolio performance metrics,
// which depend on the portfolio's trading history.
//
// # Built-in Signals
//
//   - [Momentum](ctx, u, period, metrics...): Percent change over a lookback period.
//   - [EarningsYield](ctx, u, t...): Earnings per share divided by price.
//   - [Volatility](ctx, u, period, metrics...): Rolling standard deviation of returns.
//   - [RSI](ctx, u, period, metrics...): Relative Strength Index with Wilder smoothing.
//   - [MACD](ctx, u, fast, slow, signalPeriod, metrics...): Moving average convergence divergence (line, signal, histogram).
//   - [BollingerBands](ctx, u, period, numStdDev, metrics...): Upper, middle, and lower Bollinger Bands.
//   - [Crossover](ctx, u, fastPeriod, slowPeriod, metrics...): Moving average crossover signal with fast/slow SMAs.
//   - [ATR](ctx, u, period): Average True Range with Wilder smoothing.
//
// # Custom Signals
//
// Any function that takes a universe and returns a [data.DataFrame] can serve
// as a signal. For example, a book-to-price signal:
//
//	func BookToPrice(ctx context.Context, u universe.Universe) *data.DataFrame {
//		book := u.DataFrame(ctx, data.BookValuePerShare)
//		price := u.DataFrame(ctx, data.Close)
//		return book.Div(price)
//	}
//
// # Composing Signals
//
// Because signals return DataFrames, they compose through DataFrame arithmetic.
// For example, a composite momentum signal averaging three lookback periods:
//
//	mom1 := signal.Momentum(ctx, u, portfolio.Month)
//	mom3 := signal.Momentum(ctx, u, portfolio.Quarter)
//	mom6 := signal.Momentum(ctx, u, portfolio.HalfYear)
//	composite := mom1.Add(mom3).Add(mom6).DivScalar(3)
//
// # Error Handling
//
// If a required metric is missing, the signal returns a [data.DataFrame] with
// [data.DataFrame.Err] set. Always check with df.Err() before using the result:
//
//	df := signal.Momentum(ctx, u, portfolio.Month)
//	if df.Err() != nil {
//		return df.Err()
//	}
package signal
