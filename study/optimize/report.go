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
	"fmt"
	"io"
	"math"
	"text/tabwriter"
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
	Warning       string              `json:"warning,omitempty"`
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

// Text writes a human-readable plain-text rendering of the report to
// writer. The output is intended for terminal use: header, ranking
// table, best combo, and any warning. Equity curves and the redundant
// overfitting table are omitted.
//
// Numeric columns are right-aligned by formatting the values into
// fixed-width strings before handing them to tabwriter, so the table
// stays readable without per-column alignment configuration.
func (or *optimizerReport) Text(writer io.Writer) error {
	if _, err := fmt.Fprintf(writer, "Optimization: %s (%d combos)\n", or.ObjectiveName, len(or.Rankings)); err != nil {
		return err
	}

	if or.Warning != "" {
		if _, err := fmt.Fprintf(writer, "\nWarning: %s\n", or.Warning); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "Rank\tParameters\t   OOS\t    IS\tStdDev"); err != nil {
		return err
	}

	for _, row := range or.Rankings {
		if _, err := fmt.Fprintf(tw, "%4d\t%s\t%s\t%s\t%s\n",
			row.Rank,
			row.Parameters,
			formatScore(row.MeanOOS),
			formatScore(row.MeanIS),
			formatScore(row.OOSStdDev),
		); err != nil {
			return err
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	if or.BestDetail != nil {
		if _, err := fmt.Fprintf(writer, "\nBest: %s\n", or.BestDetail.Parameters); err != nil {
			return err
		}

		if len(or.BestDetail.Folds) > 1 {
			foldTw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

			if _, err := fmt.Fprintln(foldTw, "Fold\t    IS\t   OOS"); err != nil {
				return err
			}

			for _, fold := range or.BestDetail.Folds {
				if _, err := fmt.Fprintf(foldTw, "%s\t%s\t%s\n",
					fold.FoldName,
					formatScore(fold.ISScore),
					formatScore(fold.OOSScore),
				); err != nil {
					return err
				}
			}

			if err := foldTw.Flush(); err != nil {
				return err
			}
		}
	}

	return nil
}

// formatScore renders a metric value into a fixed-width, right-aligned
// six-character string. NaN and Inf become "   n/a"; everything else
// is formatted to three decimal places (e.g. "  0.688", " -1.234").
func formatScore(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "   n/a"
	}

	return fmt.Sprintf("%6.3f", value)
}
