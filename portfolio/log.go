// Copyright 2021-2025
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

import (
	"encoding/hex"

	"github.com/rs/zerolog"
)

func (o *TaxLot) MarshalZerologObject(e *zerolog.Event) {
	e.Time("Date", o.Date).Str("CompositeFIGI", o.CompositeFIGI).Str("Ticker", o.Ticker).Float64("Shares", o.Shares).Float64("PricePerShare", o.PricePerShare)
}

func (o *DrawDown) MarshalZerologObject(e *zerolog.Event) {
	e.Time("Begin", o.Begin).Time("End", o.End).Time("RecoveryDate", o.Recovery).Float64("LossPercent", o.LossPercent)
}

func (metrics *Metrics) MarshalZerologObject(e *zerolog.Event) {
	e.Float64("AlphaSinceInception", metrics.AlphaSinceInception)
	e.Float64("AvgDrawDown", metrics.AvgDrawDown)
	e.Float64("BetaSinceInception", metrics.BetaSinceInception)
	e.Float64("DownsideDeviationSinceInception", metrics.DownsideDeviationSinceInception)
	e.Float64("DynamicWithdrawalRateSinceInception", metrics.DynamicWithdrawalRateSinceInception)
	e.Float64("ExcessKurtosisSinceInception", metrics.ExcessKurtosisSinceInception)
	e.Float64("FinalBalance", metrics.FinalBalance)
	e.Float64("PerpetualWithdrawalRateSinceInception", metrics.PerpetualWithdrawalRateSinceInception)
	e.Float64("SafeWithdrawalRateSinceInception", metrics.SafeWithdrawalRateSinceInception)
	e.Float64("SharpeRatioSinceInception", metrics.SharpeRatioSinceInception)
	e.Float64("Skewness", metrics.Skewness)
	e.Float64("SortinoRatioSinceInception", metrics.SortinoRatioSinceInception)
	e.Float64("StdDevSinceInception", metrics.StdDevSinceInception)
	e.Float64("TotalDeposited", metrics.TotalDeposited)
	e.Float64("TotalWithdrawn", metrics.TotalWithdrawn)
	e.Float64("UlcerIndexAvg", metrics.UlcerIndexAvg)
	e.Float64("UlcerIndexP50", metrics.UlcerIndexP50)
	e.Float64("UlcerIndexP90", metrics.UlcerIndexP90)
	e.Float64("UlcerIndexP99", metrics.UlcerIndexP99)
	e.Float32("BestYear.Return", metrics.BestYear.Return)
	e.Uint16("BestYear.Year", metrics.BestYear.Year)
	e.Float32("WorstYear.Return", metrics.WorstYear.Return)
	e.Uint16("WorstYear.Year", metrics.WorstYear.Year)
}

func (o *Transaction) MarshalZerologObject(e *zerolog.Event) {
	related := make([]string, len(o.Related))
	for idx, relatedID := range o.Related {
		related[idx] = hex.EncodeToString(relatedID)
	}
	e.Str("TransactionID", hex.EncodeToString(o.ID)).
		Time("Date", o.Date).
		Str("Kind", o.Kind).
		Bool("Cleared", o.Cleared).
		Str("TaxDisposition", o.TaxDisposition).
		Str("CompositeFIGI", o.CompositeFIGI).
		Str("Ticker", o.Ticker).
		Float64("Shares", o.Shares).
		Float64("PricePerShare", o.PricePerShare).
		Float64("Commission", o.Commission).
		Float64("TotalValue", o.TotalValue).
		Float64("GainLoss", o.GainLoss).
		Str("Memo", o.Memo).
		Strs("Tags", o.Tags).
		Strs("Related", related)
}
