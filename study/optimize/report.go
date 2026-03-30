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
	"io"
	"time"

	"github.com/bytedance/sonic"
)

var nanSafeAPI = sonic.Config{
	EscapeHTML:            true,
	SortMapKeys:           true,
	CompactMarshaler:      true,
	CopyString:            true,
	ValidateString:        true,
	EncodeNullForInfOrNan: true,
}.Froze()

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
	return nanSafeAPI.NewEncoder(writer).Encode(or)
}
