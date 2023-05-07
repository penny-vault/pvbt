// Copyright 2021-2023
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
	"strings"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/rs/zerolog/log"
)

type Request struct {
	securities []*Security
	metrics    map[Metric]int
}

// NewDataRequest creates a new data request object for the requested securities. The frequency
// is defaulted to Monthly and the metric defaults to Adjusted Close
func NewDataRequest(securities ...*Security) *Request {
	req := &Request{
		securities: securities,
		metrics:    make(map[Metric]int, 1),
	}

	req.metrics[MetricAdjustedClose] = 2

	return req
}

func (req *Request) Securities(securities ...*Security) *Request {
	req.securities = securities
	return req
}

func (req *Request) Metrics(metrics ...Metric) *Request {
	req.metrics = make(map[Metric]int, len(metrics))

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

// Between queries the metric data store and returns a dataframe with metrics between the specified dates
// columns are named `CompositeFigi:MetricName`
func (req *Request) Between(a, b time.Time) (dataframe.Map, error) {
	manager := GetManagerInstance()
	dfMap, err := manager.GetMetrics(req.securities, req.metricsArray(), a, b)
	if err != nil {
		return nil, err
	}
	return dfMap, nil
}

// OnSingle is a convenience function that returns the price for a single security and metric
func (req *Request) OnSingle(a time.Time) (float64, error) {
	metrics := req.metricsArray()
	if len(req.securities) > 1 || len(metrics) > 1 {
		return 0, ErrSingle
	}

	securityMetric := SecurityMetric{
		SecurityObject: *req.securities[0],
		MetricObject:   metrics[0],
	}

	price, err := req.On(a)
	if err != nil {
		return 0, err
	}

	priceVal, ok := price[securityMetric]
	if ok {
		return priceVal, err
	}

	return math.NaN(), err
}

// On returns the price for the requested date
func (req *Request) On(a time.Time) (map[SecurityMetric]float64, error) {
	manager := GetManagerInstance()

	start := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, common.GetTimezone())
	dfMap, err := manager.GetMetrics(req.securities, req.metricsArray(), start, start)
	if err != nil {
		return nil, err
	}

	if len(dfMap) == 0 {
		return nil, ErrSecurityNotFound
	}

	res := make(map[SecurityMetric]float64, len(dfMap))
	for _, df := range dfMap {
		for idx, colName := range df.ColNames {
			parts := strings.Split(colName, ":")
			security, err := SecurityFromFigi(parts[0])
			if err != nil {
				continue
			}
			securityMetric := SecurityMetric{
				SecurityObject: *security,
				MetricObject:   Metric(parts[1]),
			}

			if len(df.Vals) >= idx {
				if len(df.Vals[idx]) > 0 {
					res[securityMetric] = df.Vals[idx][0]
				} else {
					res[securityMetric] = math.NaN()
				}
			} else {
				res[securityMetric] = math.NaN()
			}
		}
	}

	return res, nil
}

func (req *Request) OnOrBefore(a time.Time) (float64, time.Time, error) {
	var requestedMetric Metric
	var metricCnt int
	for k, v := range req.metrics {
		if v == 1 {
			requestedMetric = k
			metricCnt++
		}
	}

	if len(req.securities) > 1 || metricCnt > 1 {
		log.Error().Msg("OnOrBefore called with multiple securities")
		return 0.0, time.Time{}, ErrMultipleNotSupported
	}

	manager := GetManagerInstance()
	val, eventDate, err := manager.GetMetricOnOrBefore(req.securities[0], requestedMetric, a)
	if err != nil {
		return 0.0, time.Time{}, err
	}

	return val, eventDate, nil
}

// private methods

func (req *Request) metricsArray() []Metric {
	metrics := make([]Metric, 0, len(req.metrics))
	for k := range req.metrics {
		metrics = append(metrics, k)
	}
	return metrics
}
