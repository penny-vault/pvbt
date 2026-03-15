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

package portfolio

import "time"

// MetricRow represents a single computed performance metric value at a
// specific date and window. The engine appends these at each step.
type MetricRow struct {
	Date   time.Time
	Name   string // e.g. "sharpe", "beta"
	Window string // e.g. "5yr", "3yr", "1yr", "ytd", "mtd", "wtd", "since_inception"
	Value  float64
}

// Metrics returns the accumulated metric rows.
func (a *Account) Metrics() []MetricRow {
	return a.metrics
}

// AppendMetric appends a MetricRow to the account's metric storage.
// Called by the engine at each step after computing metrics.
func (a *Account) AppendMetric(row MetricRow) {
	a.metrics = append(a.metrics, row)
}
