// Copyright 2021-2022
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

package data

import (
	"fmt"
	"time"

	"github.com/penny-vault/pv-api/dataframe"
)

func securityMetricMapToDataFrame(vals map[SecurityMetric][]float64, dates []time.Time) *dataframe.DataFrame {
	df := &dataframe.DataFrame{
		Dates:    dates,
		ColNames: make([]string, len(vals)),
		Vals:     make([][]float64, len(vals)),
	}
	idx := 0
	for k, v := range vals {
		df.ColNames[idx] = fmt.Sprintf("%s:%s", k.SecurityObject.CompositeFigi, k.MetricObject)
		df.Vals[idx] = v
		idx++
	}
	return df
}
