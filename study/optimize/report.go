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

package optimize

import (
	"encoding/json"
	"io"
	"math"
	"time"
)

// optimizerReport implements report.Report for the parameter optimizer.
type optimizerReport struct {
	ObjectiveName string              `json:"objectiveName"`
	Rankings      []rankingRow        `json:"rankings"`
	BestDetail    *bestComboDetail    `json:"bestDetail,omitempty"`
	Overfitting   []overfittingRow    `json:"overfitting"`
	EquityCurves  []equityCurveSeries `json:"equityCurves"`
}

type rankingRow struct {
	Rank       int     `json:"rank"`
	Parameters string  `json:"parameters"`
	MeanOOS    float64 `json:"meanOOS"`
	MeanIS     float64 `json:"meanIS"`
	OOSStdDev  float64 `json:"oosStdDev"`
}

type bestComboDetail struct {
	Parameters string       `json:"parameters"`
	Folds      []foldDetail `json:"folds"`
}

type foldDetail struct {
	FoldName string  `json:"foldName"`
	ISScore  float64 `json:"isScore"`
	OOSScore float64 `json:"oosScore"`
}

type overfittingRow struct {
	Parameters  string  `json:"parameters"`
	MeanIS      float64 `json:"meanIS"`
	MeanOOS     float64 `json:"meanOOS"`
	Degradation float64 `json:"degradation"`
}

type equityCurveSeries struct {
	Name   string      `json:"name"`
	Times  []time.Time `json:"times,omitempty"`
	Values []float64   `json:"values,omitempty"`
}

func (or *optimizerReport) Name() string { return "Optimize" }

func (or *optimizerReport) Data(writer io.Writer) error {
	// Create a sanitized copy that replaces NaN with null-friendly zero values
	// since JSON does not support NaN.
	sanitized := *or

	sanitized.Rankings = make([]rankingRow, len(or.Rankings))
	for idx, row := range or.Rankings {
		sanitized.Rankings[idx] = rankingRow{
			Rank:       row.Rank,
			Parameters: row.Parameters,
			MeanOOS:    sanitizeFloat(row.MeanOOS),
			MeanIS:     sanitizeFloat(row.MeanIS),
			OOSStdDev:  sanitizeFloat(row.OOSStdDev),
		}
	}

	if or.BestDetail != nil {
		detail := *or.BestDetail
		detail.Folds = make([]foldDetail, len(or.BestDetail.Folds))

		for idx, fold := range or.BestDetail.Folds {
			detail.Folds[idx] = foldDetail{
				FoldName: fold.FoldName,
				ISScore:  sanitizeFloat(fold.ISScore),
				OOSScore: sanitizeFloat(fold.OOSScore),
			}
		}

		sanitized.BestDetail = &detail
	}

	sanitized.Overfitting = make([]overfittingRow, len(or.Overfitting))
	for idx, row := range or.Overfitting {
		sanitized.Overfitting[idx] = overfittingRow{
			Parameters:  row.Parameters,
			MeanIS:      sanitizeFloat(row.MeanIS),
			MeanOOS:     sanitizeFloat(row.MeanOOS),
			Degradation: sanitizeFloat(row.Degradation),
		}
	}

	return json.NewEncoder(writer).Encode(&sanitized)
}

// sanitizeFloat replaces NaN and Inf with zero for JSON compatibility.
func sanitizeFloat(val float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}

	return val
}
