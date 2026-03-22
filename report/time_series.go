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
	"time"
)

// NamedSeries is a single labelled time series within a TimeSeries section.
type NamedSeries struct {
	Name   string      `json:"name"`
	Times  []time.Time `json:"times"`
	Values []float64   `json:"values"`
}

// TimeSeries is a Section that holds one or more named time series.
type TimeSeries struct {
	SectionName string        `json:"name"`
	Series      []NamedSeries `json:"series"`
}

// Type returns the discriminator "time_series".
func (ts *TimeSeries) Type() string { return "time_series" }

// Name returns the human-readable section heading.
func (ts *TimeSeries) Name() string { return ts.SectionName }

// Render writes the time series in the requested format.
func (ts *TimeSeries) Render(format Format, writer io.Writer) error {
	switch format {
	case FormatText:
		return ts.renderText(writer)
	case FormatJSON:
		return ts.renderJSON(writer)
	default:
		return fmt.Errorf("unsupported format %q for time_series section", format)
	}
}

func (ts *TimeSeries) renderText(writer io.Writer) error {
	if _, err := fmt.Fprintf(writer, "%s\n", ts.SectionName); err != nil {
		return err
	}

	for _, series := range ts.Series {
		if _, err := fmt.Fprintf(writer, "  %s: %d points\n", series.Name, len(series.Values)); err != nil {
			return err
		}
	}

	return nil
}

func (ts *TimeSeries) renderJSON(writer io.Writer) error {
	envelope := struct {
		Type   string        `json:"type"`
		Name   string        `json:"name"`
		Series []NamedSeries `json:"series"`
	}{
		Type:   ts.Type(),
		Name:   ts.SectionName,
		Series: ts.Series,
	}

	encoder := json.NewEncoder(writer)

	return encoder.Encode(envelope)
}
