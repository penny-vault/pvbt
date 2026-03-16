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

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Monokai-inspired adaptive color palette
// ---------------------------------------------------------------------------

var (
	// Foreground / emphasis -- Monokai warm white on dark, near-black on light.
	colorFg = lipgloss.AdaptiveColor{Dark: "#F8F8F2", Light: "#272822"}

	// Muted text -- Monokai comment gray on dark, medium gray on light.
	colorMuted = lipgloss.AdaptiveColor{Dark: "#75715E", Light: "#6E7066"}

	// Accent / headings -- Monokai cyan on dark, deeper teal on light.
	colorAccent = lipgloss.AdaptiveColor{Dark: "#66D9EF", Light: "#0087AF"}

	// Positive values -- Monokai green on dark, darker green on light.
	colorPositive = lipgloss.AdaptiveColor{Dark: "#A6E22E", Light: "#4E9A06"}

	// Negative values -- Monokai pink on dark, deeper red on light.
	colorNegative = lipgloss.AdaptiveColor{Dark: "#F92672", Light: "#CC0000"}

	// Dim / disabled -- darker gray on dark, lighter gray on light.
	colorDim = lipgloss.AdaptiveColor{Dark: "#49483E", Light: "#B0B0B0"}

	// Chart: strategy line -- Monokai purple on dark, deeper purple on light.
	ColorStrategy = lipgloss.AdaptiveColor{Dark: "#AE81FF", Light: "#6C3DAF"}

	// Chart: benchmark line -- Monokai orange on dark, deeper orange on light.
	ColorBenchmark = lipgloss.AdaptiveColor{Dark: "#FD971F", Light: "#C4700A"}
)

// ---------------------------------------------------------------------------
// Shared lipgloss styles
// ---------------------------------------------------------------------------

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	subHeaderStyle = lipgloss.NewStyle().Foreground(colorMuted)

	sectionTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).MarginTop(1)

	labelStyle = lipgloss.NewStyle().Foreground(colorMuted)
	valueStyle = lipgloss.NewStyle().Foreground(colorFg).Bold(true)

	positiveStyle = lipgloss.NewStyle().Foreground(colorPositive)
	negativeStyle = lipgloss.NewStyle().Foreground(colorNegative)

	dimStyle = lipgloss.NewStyle().Foreground(colorDim)

	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
)

// ---------------------------------------------------------------------------
// Format helpers
// ---------------------------------------------------------------------------

// fmtPct formats a float64 as a percentage string with green/red coloring.
// Returns "N/A" for NaN values.
func fmtPct(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}

	text := fmt.Sprintf("%.2f%%", val*100)

	if val >= 0 {
		return positiveStyle.Render(text)
	}

	return negativeStyle.Render(text)
}

// fmtPctDiff formats a float64 as a signed percentage string.
func fmtPctDiff(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}

	text := fmt.Sprintf("%+.2f%%", val*100)

	if val >= 0 {
		return positiveStyle.Render(text)
	}

	return negativeStyle.Render(text)
}

// fmtRatio formats a float64 as a ratio with 3 decimal places.
// Returns "N/A" for NaN values.
func fmtRatio(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}

	text := fmt.Sprintf("%.3f", val)

	if val >= 0 {
		return positiveStyle.Render(text)
	}

	return negativeStyle.Render(text)
}

// fmtCurrency formats a float64 as a dollar amount with commas.
func fmtCurrency(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}

	text := formatDollar(val)

	if val < 0 {
		return negativeStyle.Render(text)
	}

	return valueStyle.Render(text)
}

// fmtCurrencyLarge formats a float64 as a dollar amount with commas for large values.
func fmtCurrencyLarge(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}

	text := formatDollar(val)

	if val < 0 {
		return negativeStyle.Render(text)
	}

	return valueStyle.Render(text)
}

// fmtDays formats a float64 as "N days".
func fmtDays(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}

	return valueStyle.Render(fmt.Sprintf("%.0f days", val))
}

// padRight right-pads a string to the given width, accounting for ANSI escape
// sequences via lipgloss.Width.
func padRight(text string, width int) string {
	visible := lipgloss.Width(text)
	if visible >= width {
		return text
	}

	return text + strings.Repeat(" ", width-visible)
}

// padLeft left-pads a string to the given width, accounting for ANSI escape
// sequences via lipgloss.Width.
func padLeft(text string, width int) string {
	visible := lipgloss.Width(text)
	if visible >= width {
		return text
	}

	return strings.Repeat(" ", width-visible) + text
}

// formatDollar formats a float64 as "$1,234.56" with commas.
func formatDollar(val float64) string {
	negative := val < 0
	if negative {
		val = -val
	}

	whole := int64(val)
	cents := int64((val - float64(whole) + 0.005) * 100)

	if cents >= 100 {
		whole++
		cents = 0
	}

	// Format with commas.
	wholeStr := formatWithCommas(whole)

	if negative {
		return fmt.Sprintf("$-%s.%02d", wholeStr, cents)
	}

	return fmt.Sprintf("$%s.%02d", wholeStr, cents)
}

// formatWithCommas inserts commas into an integer string.
func formatWithCommas(val int64) string {
	str := fmt.Sprintf("%d", val)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder

	remainder := len(str) % 3
	if remainder > 0 {
		result.WriteString(str[:remainder])
	}

	for idx := remainder; idx < len(str); idx += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}

		result.WriteString(str[idx : idx+3])
	}

	return result.String()
}
