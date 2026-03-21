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

// Package risk provides portfolio middleware for enforcing risk constraints.
// Middleware sits between the strategy and the broker: after the strategy
// writes orders into a batch, the engine runs the batch through each
// registered middleware in order before submitting to the broker.
//
// Risk controls are configured by the operator (the person running the
// backtest or live system), not the strategy author. The strategy
// expresses intent through the batch; middleware enforces limits.
//
// # Built-in Middleware
//
// Four middleware implementations are provided:
//
//   - [MaxPositionSize] caps any single position at a fraction of total
//     portfolio value. Positions that exceed the limit are sold down;
//     excess allocation goes to cash.
//   - [MaxPositionCount] limits concurrent positions. When projected
//     holdings exceed the limit, the smallest positions by dollar value
//     are sold first.
//   - [DrawdownCircuitBreaker] force-liquidates all equity positions
//     when drawdown from peak exceeds a threshold (e.g., 0.15 for 15%).
//   - [VolatilityScaler] scales position sizes inversely to trailing
//     realized volatility. Higher-volatility assets receive smaller
//     allocations. Requires a [DataSource] for fetching price history.
//
// # Profiles
//
// Pre-built middleware chains for common configurations:
//
//   - [Conservative](ds): 20% max position, 10% drawdown breaker,
//     volatility scaling with 60-day lookback. Requires a DataSource.
//   - [Moderate](): 25% max position, 15% drawdown breaker.
//   - [Aggressive](): 35% max position, 25% drawdown breaker.
//
// # Usage
//
// Register middleware on the account before passing it to the engine:
//
//	acct := portfolio.New(portfolio.WithCash(100000, startDate))
//	acct.Use(risk.MaxPositionSize(0.25), risk.DrawdownCircuitBreaker(0.15))
//	eng := engine.New(strategy, engine.WithAccount(acct))
//
// Or use a profile:
//
//	acct.Use(risk.Moderate()...)
package risk
