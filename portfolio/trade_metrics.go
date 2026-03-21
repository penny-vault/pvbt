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

package portfolio

// TradeMetrics contains trade analysis measurements for a portfolio,
// derived from the transaction log. These evaluate the quality and
// characteristics of individual trades.
type TradeMetrics struct {
	WinRate              float64 // percentage of trades that were profitable
	AverageWin           float64 // average profit on winning trades
	AverageLoss          float64 // average loss on losing trades
	ProfitFactor         float64 // gross profit divided by gross loss
	AverageHoldingPeriod float64 // average days a position is held
	Turnover             float64 // annual portfolio turnover rate
	NPositivePeriods     float64 // percentage of periods with positive returns
	GainLossRatio        float64 // average gain divided by average loss
	AverageMFE           float64 // mean MFE across all round trips
	AverageMAE           float64 // mean MAE across all round trips
	MedianMFE            float64 // median MFE across all round trips
	MedianMAE            float64 // median MAE across all round trips
}
