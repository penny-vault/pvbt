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

type Momentum struct {
	Assets []string
}

func (m *Momentum) IndicatorForPeriod(start time.Time, end time.Time) *dataframe.DataFrame {
	ctx := context.Background()
	manager := data.NewManager()
	manager.Begin = start
	manager.End = end

	df, err := manager.GetDataFrame(ctx, data.MetricAdjustedClose, m.Assets...)
	if err != nil {
		log.Error().Err(err).Msg("could not load data for momentum indicator")
	}

	return df
}
