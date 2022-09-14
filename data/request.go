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
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

type DataRequest struct {
	securities []*Security
	frequency  Frequency
	metrics    map[Metric]int
}

func NewDataRequest(securities ...*Security) *DataRequest {
	req := &DataRequest{
		securities: securities,
		frequency:  FrequencyMonthly,
		metrics:    make(map[Metric]int, 1),
	}

	req.metrics[MetricAdjustedClose] = 2

	return req
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
	for metric, status := range req.metrics {
		if status != 2 {
			delete(req.metrics, metric)
		}
	}

	if len(metrics) == 0 {
		log.Warn().Msg("cannot set request metrics to an empty list; using adjusted close")
		req.metrics[MetricAdjustedClose] = 2
	} else {
		for _, m := range metrics {
			req.metrics[m] = 1
		}
	}

	return req
}

func (req *DataRequest) Between(ctx context.Context, a, b time.Time) (*dataframe.DataFrame, error) {
	manager := getManagerInstance()
	metricVals, dates, err := manager.GetMetrics(req.securities, req.metricsArray(), a, b)
	if err != nil {
		log.Error().Err(err).Msg("could not get data")
	}
	// filter down to requested frequency
	df := securityMetricMapToDataFrame(metricVals, dates)
	df = df.Frequency(dataframe.Monthly)
}

func (req *DataRequest) On(a time.Time) (map[SecurityMetric]float64, error) {
	return nil, nil
}

func (req *DataRequest) OnOrBefore(a time.Time) (map[SecurityMetric]float64, error) {
	return nil, nil
}

// private methods

func (req *DataRequest) metricsArray() []Metric {
	metrics := make([]Metric, 0, len(req.metrics))
	for k := range req.metrics {
		metrics = append(metrics, k)
	}
	return metrics
}