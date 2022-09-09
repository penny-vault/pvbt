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

	"github.com/jdfergason/dataframe-go"
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
	// if metric is open, high, low, close, or adjusted close also pre-fetch splits
	// and dividends
	_, hasOpen := req.metrics[MetricOpen]
	_, hasHigh := req.metrics[MetricHigh]
	_, hasLow := req.metrics[MetricLow]
	_, hasClose := req.metrics[MetricClose]
	_, hasAdjustedClose := req.metrics[MetricAdjustedClose]

	if hasOpen || hasHigh || hasLow || hasClose || hasAdjustedClose {
		req.metrics[MetricSplitFactor] = 3
		req.metrics[MetricDividendCash] = 3
	}

	metrics := make([]Metric, 0, len(req.metrics))
	metricsStr := make([]string, 0, len(req.metrics))
	for k := range req.metrics {
		metrics = append(metrics, k)
		metricsStr = append(metricsStr, string(k))
	}

	subLog := log.With().Time("Begin", a).Time("End", b).Strs("Metrics", metricsStr).Str("Frequency", string(req.frequency)).Logger()

	manager := getManagerInstance()

	// check if the data is in the cache
	type securityMetric struct {
		security *Security
		metric   Metric
	}

	toPull := make([]securityMetric, 0)
	for _, security := range req.securities {
		for metric := range req.metrics {
			data, err := manager.cache.Get(security, metric, a, b)
			if err != nil {
				toPull[security] = metric
			}
		}
	}

	if res, err := manager.pvdb.Get(ctx, req.securities, metrics, req.frequency, a, b); err == nil {
		df := dataFrameFromMap(res, a, b)
		return df, nil
	} else {
		subLog.Error().Err(err).Msg("could not fetch data")
		return nil, err
	}

	return nil, ErrNotFound
}

func (req *DataRequest) On(a time.Time) (map[Security]float64, error) {
	return nil, nil
}

// private methods

func dataFrameFromMap(vals map[Security][]float64, begin, end time.Time) *dataframe.DataFrame
