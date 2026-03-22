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

package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/report"
)

var _ = Describe("Section primitives", func() {
	Describe("Table", func() {
		var tbl *report.Table

		BeforeEach(func() {
			tbl = &report.Table{
				SectionName: "Returns",
				Columns: []report.Column{
					{Header: "Year", Format: "string", Align: "left"},
					{Header: "Return", Format: "percent", Align: "right"},
				},
				Rows: [][]any{
					{"2024", 0.12},
					{"2025", -0.03},
				},
			}
		})

		It("returns the correct type discriminator", func() {
			Expect(tbl.Type()).To(Equal("table"))
		})

		It("returns the section name", func() {
			Expect(tbl.Name()).To(Equal("Returns"))
		})

		It("implements the Section interface", func() {
			var section report.Section = tbl
			Expect(section.Type()).To(Equal("table"))
		})

		It("renders aligned text with headers and separator", func() {
			var buf bytes.Buffer
			Expect(tbl.Render(report.FormatText, &buf)).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("Year"))
			Expect(output).To(ContainSubstring("Return"))
			Expect(output).To(ContainSubstring("----"))
			Expect(output).To(ContainSubstring("2024"))
			Expect(output).To(ContainSubstring("2025"))
		})

		It("renders JSON with type discriminator", func() {
			var buf bytes.Buffer
			Expect(tbl.Render(report.FormatJSON, &buf)).To(Succeed())

			var parsed map[string]any
			Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("table"))
			Expect(parsed["name"]).To(Equal("Returns"))
			Expect(parsed).To(HaveKey("columns"))
			Expect(parsed).To(HaveKey("rows"))
		})

		It("returns an error for unsupported formats", func() {
			var buf bytes.Buffer
			err := tbl.Render(report.FormatHTML, &buf)
			Expect(err).To(MatchError(ContainSubstring("unsupported format")))
		})

		It("handles empty columns gracefully", func() {
			empty := &report.Table{SectionName: "Empty"}
			var buf bytes.Buffer
			Expect(empty.Render(report.FormatText, &buf)).To(Succeed())
			Expect(buf.String()).To(BeEmpty())
		})

		It("renders percent-format column values as percentages", func() {
			percentTable := &report.Table{
				SectionName: "Percent Test",
				Columns: []report.Column{
					{Header: "Label", Format: "string", Align: "left"},
					{Header: "Return", Format: "percent", Align: "right"},
				},
				Rows: [][]any{
					{"Strategy", 0.12},
					{"Benchmark", -0.03},
				},
			}
			var buf bytes.Buffer
			Expect(percentTable.Render(report.FormatText, &buf)).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("12.00%"))
			Expect(output).To(ContainSubstring("-3.00%"))
			// Raw decimal values must not appear in the output.
			Expect(output).NotTo(ContainSubstring("0.12"))
			Expect(output).NotTo(ContainSubstring("-0.03"))
		})
	})

	Describe("TimeSeries", func() {
		var ts *report.TimeSeries

		BeforeEach(func() {
			ts = &report.TimeSeries{
				SectionName: "Equity Curve",
				Series: []report.NamedSeries{
					{
						Name:   "Portfolio",
						Times:  []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
						Values: []float64{100.0, 112.0},
					},
					{
						Name:   "Benchmark",
						Times:  []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
						Values: []float64{100.0, 105.0},
					},
				},
			}
		})

		It("returns the correct type discriminator", func() {
			Expect(ts.Type()).To(Equal("time_series"))
		})

		It("returns the section name", func() {
			Expect(ts.Name()).To(Equal("Equity Curve"))
		})

		It("implements the Section interface", func() {
			var section report.Section = ts
			Expect(section.Type()).To(Equal("time_series"))
		})

		It("renders text summary with series names and point counts but no section header", func() {
			var buf bytes.Buffer
			Expect(ts.Render(report.FormatText, &buf)).To(Succeed())

			output := buf.String()
			Expect(output).NotTo(ContainSubstring("Equity Curve"))
			Expect(output).To(ContainSubstring("Portfolio: 2 points"))
			Expect(output).To(ContainSubstring("Benchmark: 2 points"))
		})

		It("renders JSON with type discriminator and series data", func() {
			var buf bytes.Buffer
			Expect(ts.Render(report.FormatJSON, &buf)).To(Succeed())

			var parsed map[string]any
			Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("time_series"))
			Expect(parsed["name"]).To(Equal("Equity Curve"))

			series, ok := parsed["series"].([]any)
			Expect(ok).To(BeTrue())
			Expect(series).To(HaveLen(2))
		})

		It("returns an error for unsupported formats", func() {
			var buf bytes.Buffer
			err := ts.Render(report.FormatHTML, &buf)
			Expect(err).To(MatchError(ContainSubstring("unsupported format")))
		})
	})

	Describe("MetricPairs", func() {
		It("returns the correct type discriminator", func() {
			mp := &report.MetricPairs{SectionName: "Risk"}
			Expect(mp.Type()).To(Equal("metric_pairs"))
		})

		It("returns the section name", func() {
			mp := &report.MetricPairs{SectionName: "Risk"}
			Expect(mp.Name()).To(Equal("Risk"))
		})

		It("implements the Section interface", func() {
			var section report.Section = &report.MetricPairs{SectionName: "Risk"}
			Expect(section.Type()).To(Equal("metric_pairs"))
		})

		It("renders metrics without comparison when Comparison is nil", func() {
			mp := &report.MetricPairs{
				SectionName: "Performance",
				Metrics: []report.MetricPair{
					{Label: "CAGR", Value: 0.085, Format: "percent"},
					{Label: "Max Drawdown Days", Value: 45, Format: "days"},
				},
			}

			var buf bytes.Buffer
			Expect(mp.Render(report.FormatText, &buf)).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("CAGR: 8.50%"))
			Expect(output).To(ContainSubstring("Max Drawdown Days: 45 days"))
			Expect(output).NotTo(ContainSubstring("vs"))
		})

		It("renders metrics with comparison when Comparison is set", func() {
			comp := 0.072
			mp := &report.MetricPairs{
				SectionName: "Performance",
				Metrics: []report.MetricPair{
					{Label: "CAGR", Value: 0.085, Comparison: &comp, Format: "percent"},
				},
			}

			var buf bytes.Buffer
			Expect(mp.Render(report.FormatText, &buf)).To(Succeed())

			output := buf.String()
			Expect(output).To(ContainSubstring("CAGR: 8.50% (vs 7.20%)"))
		})

		It("renders ratio format correctly", func() {
			mp := &report.MetricPairs{
				SectionName: "Ratios",
				Metrics: []report.MetricPair{
					{Label: "Sharpe", Value: 1.2345, Format: "ratio"},
				},
			}

			var buf bytes.Buffer
			Expect(mp.Render(report.FormatText, &buf)).To(Succeed())
			Expect(buf.String()).To(ContainSubstring("Sharpe: 1.2345"))
		})

		It("renders JSON with type discriminator", func() {
			comp := 0.05
			mp := &report.MetricPairs{
				SectionName: "Risk",
				Metrics: []report.MetricPair{
					{Label: "Volatility", Value: 0.15, Comparison: &comp, Format: "percent"},
				},
			}

			var buf bytes.Buffer
			Expect(mp.Render(report.FormatJSON, &buf)).To(Succeed())

			var parsed map[string]any
			Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("metric_pairs"))
			Expect(parsed["name"]).To(Equal("Risk"))
			Expect(parsed).To(HaveKey("metrics"))
		})

		It("returns an error for unsupported formats", func() {
			mp := &report.MetricPairs{SectionName: "Risk"}
			var buf bytes.Buffer
			err := mp.Render(report.FormatHTML, &buf)
			Expect(err).To(MatchError(ContainSubstring("unsupported format")))
		})
	})

	Describe("Text", func() {
		var txt *report.Text

		BeforeEach(func() {
			txt = &report.Text{
				SectionName: "Summary",
				Body:        "The strategy outperformed the benchmark by 3.5% annualized.",
			}
		})

		It("returns the correct type discriminator", func() {
			Expect(txt.Type()).To(Equal("text"))
		})

		It("returns the section name", func() {
			Expect(txt.Name()).To(Equal("Summary"))
		})

		It("implements the Section interface", func() {
			var section report.Section = txt
			Expect(section.Type()).To(Equal("text"))
		})

		It("renders the body as plain text", func() {
			var buf bytes.Buffer
			Expect(txt.Render(report.FormatText, &buf)).To(Succeed())
			Expect(buf.String()).To(Equal("The strategy outperformed the benchmark by 3.5% annualized."))
		})

		It("renders JSON with type discriminator and body", func() {
			var buf bytes.Buffer
			Expect(txt.Render(report.FormatJSON, &buf)).To(Succeed())

			var parsed map[string]any
			Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("text"))
			Expect(parsed["name"]).To(Equal("Summary"))
			Expect(parsed["body"]).To(Equal("The strategy outperformed the benchmark by 3.5% annualized."))
		})

		It("returns an error for unsupported formats", func() {
			var buf bytes.Buffer
			err := txt.Render(report.FormatHTML, &buf)
			Expect(err).To(MatchError(ContainSubstring("unsupported format")))
		})
	})
})

var _ = Describe("Report", func() {
	It("renders all sections in order for text format", func() {
		rpt := report.Report{
			Title: "Test Report",
			Sections: []report.Section{
				&report.Text{SectionName: "Intro", Body: "Hello"},
				&report.Table{
					SectionName: "Data",
					Columns:     []report.Column{{Header: "X", Format: "number", Align: "right"}},
					Rows:        [][]any{{1}},
				},
			},
		}

		var buf bytes.Buffer
		Expect(rpt.Render(report.FormatText, &buf)).To(Succeed())
		output := buf.String()
		introIdx := strings.Index(output, "Hello")
		// Table renders its column header "X", not the SectionName, in text output.
		colHeaderIdx := strings.Index(output, "X")
		Expect(introIdx).To(BeNumerically("<", colHeaderIdx))
	})

	It("renders JSON with title and sections array", func() {
		rpt := report.Report{
			Title: "JSON Test",
			Sections: []report.Section{
				&report.Text{SectionName: "Note", Body: "test"},
			},
		}

		var buf bytes.Buffer
		Expect(rpt.Render(report.FormatJSON, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring(`"title":"JSON Test"`))
		Expect(buf.String()).To(ContainSubstring(`"sections":[`))
	})

	It("handles empty sections slice", func() {
		rpt := report.Report{Title: "Empty"}
		var buf bytes.Buffer
		Expect(rpt.Render(report.FormatText, &buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("Empty"))
	})

	It("returns error for unsupported format", func() {
		rpt := report.Report{Title: "X"}
		var buf bytes.Buffer
		Expect(rpt.Render("invalid", &buf)).To(HaveOccurred())
	})
})
