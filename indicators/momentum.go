// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package indicators

import (
	"context"
	"time"

	"github.com/jdfergason/dataframe-go"
	"github.com/penny-vault/pv-api/data"
	"github.com/rs/zerolog/log"
)

// Momentum calculates the average momentum for each asset listed in `Assets` for each
// lookback period in `Periods` and calculates an indicator based on the weighted
// momentum if each momentum is positive then indicator = 1; otherwise indicator = 0
type Momentum struct {
	Assets  []string
	Periods map[int]float64
	Manager *data.Manager
}

func (m *Momentum) IndicatorForPeriod(ctx context.Context, start time.Time, end time.Time) (*dataframe.DataFrame, error) {
	origStart := m.Manager.Begin
	origEnd := m.Manager.End

	defer func() {
		m.Manager.Begin = origStart
		m.Manager.End = origEnd
	}()

	m.Manager.Begin = start
	m.Manager.End = end

	// Add the risk free asset
	assets := append(m.Assets, "DGS3MO")
	df, err := m.Manager.GetDataFrame(ctx, data.MetricAdjustedClose, assets...)
	if err != nil {
		log.Error().Err(err).Msg("could not load data for momentum indicator")
		return nil, err
	}

	return df, nil
}
