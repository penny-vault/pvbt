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

package signal

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// MomentumSignal is the metric name for momentum signal output.
const MomentumSignal data.Metric = "Momentum"

// Momentum computes the percent change over the given period for each
// asset in the universe. Returns a single-row DataFrame with one column
// per asset containing the momentum score.
func Momentum(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("Momentum: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("Momentum: need at least 2 data points, got %d", df.Len()))
	}

	return df.Pct(df.Len()-1).Last().RenameMetric(metric, MomentumSignal)
}
