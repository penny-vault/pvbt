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

// Package universe defines the investable space for a strategy.
//
// A universe is a collection of assets that a strategy operates on. It
// determines which instruments a strategy can observe and trade. Universes
// change over time: the S&P 500 regularly adds and removes companies, for
// example. The engine resolves membership at each historical point so that
// backtests reflect the assets that were actually available on a given date,
// preventing survivorship bias (the distortion that occurs when backtests
// only consider companies that survived to the present, ignoring those that
// were delisted, acquired, or went bankrupt).
//
// # Universe Interface
//
// Every universe implements four methods:
//
//   - Assets(t time.Time) []asset.Asset -- returns the members at time t.
//   - Window(ctx, lookback, metrics...) (*data.DataFrame, error) -- returns a
//     DataFrame covering the lookback period ending at the current simulation
//     date.
//   - At(ctx, metrics...) (*data.DataFrame, error) -- returns a single-row
//     DataFrame at the current simulation date.
//   - CurrentDate() time.Time -- returns the current simulation date.
//
// # Creating Universes
//
// There are three ways to create a universe.
//
// From configuration: declare a universe.Universe field in your strategy
// struct with struct tags. The engine auto-populates the field before calling
// any strategy methods.
//
//	type MyStrategy struct {
//	    Universe universe.Universe `mapstructure:"universe"`
//	}
//
// From explicit tickers: use NewStatic to list the instruments directly.
// Prefix a ticker with a namespace to select a non-default data source.
//
//	u := universe.NewStatic("GLD", "TLT", "FRED:DGS3MO")
//
// From predefined indexes: use SP500 or Nasdaq100 with an IndexProvider.
// These universes resolve time-varying membership on every call to Assets and
// cache the results so repeated lookups for the same date are fast.
//
//	u := universe.SP500(indexProvider)
//	u := universe.Nasdaq100(indexProvider)
//
// # Getting Data
//
// Strategies retrieve market data through the universe rather than querying a
// data source directly. The Window method returns a DataFrame spanning a
// lookback period for one or more metrics, which is the primary way signals
// consume historical data. The At method returns a single-row DataFrame for a
// point-in-time lookup, useful for getting the latest price or indicator value.
//
//	df, err := u.Window(ctx, portfolio.Months(6), data.Close, data.Volume)
//	row, err := u.At(ctx, data.Close)
//
// # Membership and Time
//
// Universe membership is resolved at each computation step, not at setup time.
// Index-based universes return different members depending on the date passed
// to Assets. Static universes always return the same set of assets, but the
// engine still checks data validity for each date. This per-step resolution
// ensures that backtests do not look ahead into future index compositions.
package universe
