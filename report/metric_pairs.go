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

package report

import (
	"encoding/json"
	"fmt"
	"io"
)

// MetricPair is a single labelled metric with an optional comparison value.
type MetricPair struct {
	Label      string   `json:"label"`
	Value      float64  `json:"value"`
	Comparison *float64 `json:"comparison,omitempty"` // nil when no comparison value
	Format     string   `json:"format"`               // "percent", "ratio", "days"
}

// MetricPairs is a Section that renders a list of label-value metric pairs.
type MetricPairs struct {
	SectionName string       `json:"name"`
	Metrics     []MetricPair `json:"metrics"`
}

// Type returns the discriminator "metric_pairs".
func (mp *MetricPairs) Type() string { return "metric_pairs" }

// Name returns the human-readable section heading.
func (mp *MetricPairs) Name() string { return mp.SectionName }

// Render writes the metric pairs in the requested format.
func (mp *MetricPairs) Render(format Format, writer io.Writer) error {
	switch format {
	case FormatText:
		return mp.renderText(writer)
	case FormatJSON:
		return mp.renderJSON(writer)
	default:
		return fmt.Errorf("unsupported format %q for metric_pairs section", format)
	}
}

func (mp *MetricPairs) renderText(writer io.Writer) error {
	for _, metric := range mp.Metrics {
		formatted := formatMetricValue(metric.Value, metric.Format)
		if metric.Comparison != nil {
			compFormatted := formatMetricValue(*metric.Comparison, metric.Format)
			if _, err := fmt.Fprintf(writer, "%s: %s (vs %s)\n", metric.Label, formatted, compFormatted); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(writer, "%s: %s\n", metric.Label, formatted); err != nil {
				return err
			}
		}
	}

	return nil
}

func formatMetricValue(value float64, metricFormat string) string {
	switch metricFormat {
	case "percent":
		return fmt.Sprintf("%.2f%%", value*100)
	case "days":
		return fmt.Sprintf("%.0f days", value)
	case "ratio":
		return fmt.Sprintf("%.4f", value)
	default:
		return fmt.Sprintf("%g", value)
	}
}

func (mp *MetricPairs) renderJSON(writer io.Writer) error {
	envelope := struct {
		Type    string       `json:"type"`
		Name    string       `json:"name"`
		Metrics []MetricPair `json:"metrics"`
	}{
		Type:    mp.Type(),
		Name:    mp.SectionName,
		Metrics: mp.Metrics,
	}

	encoder := json.NewEncoder(writer)

	return encoder.Encode(envelope)
}
