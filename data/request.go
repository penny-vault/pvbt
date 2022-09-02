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
	"math"
	"time"

	"github.com/jdfergason/dataframe-go"
)

type DataRequest struct {
	securities []*Security
	frequency  Frequency
	metrics    []Metric
}

func NewDataRequest(securities ...*Security) *DataRequest {
	return &DataRequest{
		securities: securities,
		frequency:  FrequencyMonthly,
		metrics:    []Metric{MetricAdjustedClose},
	}
}

func (req *DataRequest) Securities(securities ...*Security) *DataRequest {
	req.securities = securities
	return req
}

func (req *DataRequest) Frequency(frequency Frequency) *DataRequest {
	req.frequency = frequency
	return req
}

func (req *DataRequest) Metrics(metrics ...Metric) *DataRequest {
	req.metrics = metrics
	return req
}

func (req *DataRequest) Between(a, b time.Time) *dataframe.DataFrame {
	return nil
}

func (req *DataRequest) On(a time.Time) float64 {
	return math.NaN()
}
