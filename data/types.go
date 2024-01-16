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

const (
	CashAsset = "$CASH"
)

type SecurityMap map[SecurityMetric]float64

// Security represents a tradeable asset
type Security struct {
	Ticker        string `json:"ticker"`
	CompositeFigi string `json:"compositeFigi"`
}

type SecurityMetric struct {
	SecurityObject Security
	MetricObject   Metric
}

var (
	CashSecurity = Security{
		Ticker:        CashAsset,
		CompositeFigi: CashAsset,
	}
	RiskFreeSecurity = Security{
		CompositeFigi: "PVGG06TNP6J8",
		Ticker:        "DGS3MO",
	}
)

type Metric string

const (
	MetricOpen          Metric = "Open"
	MetricLow           Metric = "Low"
	MetricHigh          Metric = "High"
	MetricClose         Metric = "Close"
	MetricVolume        Metric = "Volume"
	MetricAdjustedClose Metric = "AdjustedClose"
	MetricDividendCash  Metric = "DividendCash"
	MetricSplitFactor   Metric = "SplitFactor"
)

type FundamentalMetric string

const (
	FundamentalWorkingCapital FundamentalMetric = "working_capital"
	FundamentalMarketCap      FundamentalMetric = "market_cap"
)

type ReportingPeriod string

const (
	PeriodQ1     ReportingPeriod = "Q1"
	PeriodQ2     ReportingPeriod = "Q2"
	PeriodQ3     ReportingPeriod = "Q3"
	PeriodQ4     ReportingPeriod = "Q4"
	PeriodAnnual ReportingPeriod = "Annual"
)
