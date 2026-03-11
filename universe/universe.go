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

// Package universe defines collections of assets that strategies operate on.
package universe

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// Universe provides time-varying membership of tradeable instruments
// and data access for strategies.
type Universe interface {
	// Assets returns the members of the universe at time t.
	Assets(t time.Time) []asset.Asset

	// Prefetch tells the universe what time range the engine will
	// operate over so it can load membership data in bulk.
	Prefetch(ctx context.Context, start, end time.Time) error

	// Window returns a DataFrame covering [currentDate - lookback, currentDate]
	// for the requested metrics, using the universe's current membership.
	Window(lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)

	// At returns a single-row DataFrame at time t for the requested metrics.
	At(t time.Time, metrics ...data.Metric) (*data.DataFrame, error)
}
