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

// Package asset defines the [Asset] type representing a tradeable instrument
// identified by a CompositeFigi and a human-readable Ticker.
//
// # Asset Type
//
// The Asset struct has two fields: CompositeFigi (the canonical identifier from
// the Financial Instrument Global Identifier system) and Ticker (a
// human-readable symbol like "SPY" or "AAPL"). Assets are resolved from
// tickers via an AssetProvider registered with the engine.
//
// # Economic Indicators
//
// The [EconomicIndicator] sentinel value represents data that is not tied to a
// specific asset, such as unemployment rates or CPI. It is used in data
// requests and DataFrames to keep the layout uniform -- from the DataFrame's
// perspective, an economic indicator looks like any other asset.
//
// # Ticker Resolution
//
// Tickers can include a namespace prefix to specify the data source (e.g.,
// "FRED:DGS3MO"). The engine's Asset method resolves tickers to full Asset
// values using the registered AssetProvider.
package asset
