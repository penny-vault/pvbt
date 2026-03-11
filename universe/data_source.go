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

package universe

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// DataSource provides data fetching capabilities to universe implementations.
// The engine implements this interface; universes hold a reference to it.
// This breaks the circular dependency between engine and universe.
type DataSource interface {
	// Fetch returns a DataFrame covering [currentDate - lookback, currentDate]
	// for the given assets and metrics.
	Fetch(ctx context.Context, assets []asset.Asset, lookback portfolio.Period,
		metrics []data.Metric) (*data.DataFrame, error)

	// FetchAt returns a single-row DataFrame at the given time for the given
	// assets and metrics.
	FetchAt(ctx context.Context, assets []asset.Asset, t time.Time,
		metrics []data.Metric) (*data.DataFrame, error)

	// CurrentDate returns the current simulation date.
	CurrentDate() time.Time
}
