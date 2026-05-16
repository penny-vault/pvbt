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

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// IntradayProvider is implemented by data providers capable of serving
// 1-minute bars from the pv-data ClickHouse store. The engine routes
// intraday Fetch calls to whichever registered provider satisfies this
// interface.
type IntradayProvider interface {
	IntradayFetch(
		ctx context.Context,
		assets []asset.Asset,
		metrics []data.Metric,
		start, end time.Time,
		timesOfDay []data.TimeOfDay,
	) (*data.DataFrame, error)
}

// fetchIntraday handles a request whose lookback is one of the intraday
// units (MinuteBars or DailyAtTime). It computes the window bounds from
// the current simulation time, finds an IntradayProvider, and invokes
// IntradayFetch with the time-of-day predicate from the lookback.
//
// Adjustment is requested implicitly by the metric naming -- AdjOpen,
// AdjClose, etc. The provider applies adjustment factors at decode time
// over raw ClickHouse rows.
func (e *Engine) fetchIntraday(
	ctx context.Context,
	assets []asset.Asset,
	lookback portfolio.Period,
	metrics []data.Metric,
) (*data.DataFrame, error) {
	provider := e.findIntradayProvider()
	if provider == nil {
		return nil, fmt.Errorf(
			"engine: intraday lookback requested but no registered provider " +
				"implements IntradayProvider")
	}

	// Intraday lookbacks anchor on the firing moment (engine.Now()), not
	// the trading-day boundary. A daily-scheduled strategy querying
	// MinuteBars(60) at the close gets the last 60 minutes; an intraday
	// strategy firing at 10:00 ET gets minutes ending at 10:00.
	end := e.Now()
	start := lookback.Before(end)

	df, err := provider.IntradayFetch(ctx, assets, metrics, start, end, lookback.TimeOfDay)
	if err != nil {
		return nil, fmt.Errorf("engine: intraday fetch: %w", err)
	}

	df.SetSource(e)

	return df, nil
}

func (e *Engine) findIntradayProvider() IntradayProvider {
	for _, provider := range e.providers {
		if intraday, ok := provider.(IntradayProvider); ok {
			return intraday
		}
	}

	return nil
}
