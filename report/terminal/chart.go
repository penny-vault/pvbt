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

package terminal

import (
	"fmt"
	"math"
	"strings"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/report"
)

const (
	chartWidth  = 60
	chartHeight = 12
	leftMargin  = 10
)

// renderEquityCurve draws a line chart of the equity curve.
func renderEquityCurve(builder *strings.Builder, curve report.EquityCurve, hasBenchmark bool) {
	builder.WriteString(sectionTitleStyle.Render("Equity Curve"))
	builder.WriteString("\n")

	if len(curve.StrategyValues) == 0 {
		builder.WriteString(dimStyle.Render("  No equity data available."))
		builder.WriteString("\n")

		return
	}

	// Find global min/max across strategy and benchmark.
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	updateMinMax := func(values []float64) {
		for _, val := range values {
			if !math.IsNaN(val) {
				if val < minVal {
					minVal = val
				}

				if val > maxVal {
					maxVal = val
				}
			}
		}
	}

	updateMinMax(curve.StrategyValues)

	if hasBenchmark && len(curve.BenchmarkValues) > 0 {
		updateMinMax(curve.BenchmarkValues)
	}

	if math.IsInf(minVal, 1) || math.IsInf(maxVal, -1) {
		minVal = 0
		maxVal = 1
	}

	// Add 5% padding.
	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
	}

	padding := valRange * 0.05
	minVal -= padding
	maxVal += padding
	valRange = maxVal - minVal

	// Create canvas.
	chart := canvas.New(chartWidth, chartHeight)

	xAxisRow := chartHeight - 1

	// Resample and draw a series.
	drawSeries := func(values []float64, color lipgloss.TerminalColor) {
		numPoints := len(values)
		seqY := make([]int, chartWidth)
		hasData := make([]bool, chartWidth)

		for col := 0; col < chartWidth; col++ {
			idx := col * numPoints / chartWidth
			if idx >= numPoints {
				idx = numPoints - 1
			}

			val := values[idx]
			if math.IsNaN(val) {
				continue
			}

			cartY := int((val - minVal) / valRange * float64(chartHeight-1))
			if cartY < 0 {
				cartY = 0
			}

			if cartY >= chartHeight {
				cartY = chartHeight - 1
			}

			seqY[col] = canvas.CanvasYCoordinate(xAxisRow, cartY)
			hasData[col] = true
		}

		// Draw contiguous segments.
		style := lipgloss.NewStyle().Foreground(color)
		startCol := -1

		for col := 0; col <= chartWidth; col++ {
			if col < chartWidth && hasData[col] {
				if startCol < 0 {
					startCol = col
				}
			} else {
				if startCol >= 0 {
					seg := seqY[startCol:col]
					graph.DrawLineSequence(&chart, false, startCol, seg, runes.ArcLineStyle, style)
					startCol = -1
				}
			}
		}
	}

	// Draw benchmark first (behind strategy).
	if hasBenchmark && len(curve.BenchmarkValues) > 0 {
		drawSeries(curve.BenchmarkValues, ColorBenchmark)
	}

	drawSeries(curve.StrategyValues, ColorStrategy)

	// Get canvas view and add Y-axis labels.
	canvasView := chart.View()
	canvasLines := strings.Split(canvasView, "\n")

	gridStyle := lipgloss.NewStyle().Foreground(colorMuted)

	// Determine grid rows (top, bottom, and a few in between).
	gridInterval := chartHeight / 4
	if gridInterval < 2 {
		gridInterval = 2
	}

	isGridRow := func(row int) bool {
		return row == 0 || row == chartHeight-1 || row%gridInterval == 0
	}

	for row := 0; row < chartHeight && row < len(canvasLines); row++ {
		var line strings.Builder

		// Y-axis label.
		if isGridRow(row) {
			val := maxVal - float64(row)/float64(chartHeight-1)*valRange
			label := formatAxisLabel(val)
			line.WriteString(gridStyle.Render(fmt.Sprintf("%9s", label)))
			line.WriteString(gridStyle.Render("\u2524"))
		} else {
			line.WriteString(strings.Repeat(" ", leftMargin-1))
			line.WriteString(gridStyle.Render("\u2502"))
		}

		line.WriteString(canvasLines[row])
		builder.WriteString(line.String())
		builder.WriteString("\n")
	}

	// X-axis with start and end dates.
	startDate := curve.Times[0].Format("2006-01-02")
	endDate := curve.Times[len(curve.Times)-1].Format("2006-01-02")
	xGap := chartWidth - len(startDate) - len(endDate)

	if xGap < 1 {
		xGap = 1
	}

	xAxis := strings.Repeat(" ", leftMargin) + startDate + strings.Repeat(" ", xGap) + endDate
	builder.WriteString(dimStyle.Render(xAxis))
	builder.WriteString("\n")

	// Legend.
	stratLegend := lipgloss.NewStyle().Foreground(ColorStrategy).Render("--") + " Strategy"
	legend := "  " + stratLegend

	if hasBenchmark && len(curve.BenchmarkValues) > 0 {
		benchLegend := lipgloss.NewStyle().Foreground(ColorBenchmark).Render("--") + " Benchmark"
		legend += "    " + benchLegend
	}

	builder.WriteString(dimStyle.Render("  ") + legend)
	builder.WriteString("\n")
}

// formatAxisLabel creates a compact label for Y-axis values (e.g., "$140k", "$1.2M").
func formatAxisLabel(val float64) string {
	absVal := math.Abs(val)
	sign := ""

	if val < 0 {
		sign = "-"
	}

	switch {
	case absVal >= 1_000_000:
		return fmt.Sprintf("%s$%.1fM", sign, absVal/1_000_000)
	case absVal >= 1_000:
		return fmt.Sprintf("%s$%.0fk", sign, absVal/1_000)
	default:
		return fmt.Sprintf("%s$%.0f", sign, absVal)
	}
}
