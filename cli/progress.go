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

package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/portfolio"
	"golang.org/x/term"
)

// progressUpdateMsg carries a progress event from the engine into the
// bubble tea program. The model translates these into UI updates.
type progressUpdateMsg struct {
	step         int
	totalSteps   int
	date         time.Time
	measurements int
}

// progressDoneMsg signals that the engine has finished its run. The result
// and any error are stashed on the model so the caller can read them after
// the program exits.
type progressDoneMsg struct {
	result portfolio.Portfolio
	err    error
}

// progressModel is a minimal bubble tea model that renders a progress bar
// for a backtest run. Completion fraction is derived from the simulation
// date span; ETA is derived from elapsed wall time and that fraction.
type progressModel struct {
	bar progress.Model

	title string
	start time.Time
	end   time.Time

	current      time.Time
	step         int
	totalSteps   int
	measurements int

	runStart time.Time

	done   bool
	result portfolio.Portfolio
	err    error

	width int
}

func newProgressModel(title string, start, end time.Time) progressModel {
	bar := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)

	return progressModel{
		bar:      bar,
		title:    title,
		start:    start,
		end:      end,
		runStart: time.Now(),
		width:    80,
	}
}

func (m progressModel) Init() tea.Cmd { return nil }

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width

		barWidth := msg.Width - 4
		if barWidth < 10 {
			barWidth = 10
		}

		m.bar.Width = barWidth
	case progressUpdateMsg:
		m.step = msg.step
		m.totalSteps = msg.totalSteps
		m.current = msg.date
		m.measurements = msg.measurements
	case progressDoneMsg:
		m.done = true
		m.result = msg.result
		m.err = msg.err

		return m, tea.Quit
	}

	return m, nil
}

func (m progressModel) View() string {
	pct := m.fraction()
	bar := m.bar.ViewAs(pct)

	currentDate := "        "
	if !m.current.IsZero() {
		currentDate = m.current.Format("2006-01-02")
	}

	info := fmt.Sprintf(" %s -> %s   %5.1f%%   step %d/%d   measurements %s   eta %s   elapsed %s",
		currentDate,
		m.end.Format("2006-01-02"),
		pct*100,
		m.step,
		m.totalSteps,
		formatCount(m.measurements),
		m.formatETA(pct),
		time.Since(m.runStart).Truncate(time.Second),
	)

	titleStyle := lipgloss.NewStyle().Bold(true)

	return strings.Join([]string{titleStyle.Render(m.title), bar, info, ""}, "\n")
}

// fraction returns the completion fraction in [0,1] derived from the date
// span. Date-based progress matches what users see in their CLI flags
// (--start/--end) and is monotonic regardless of trading-day density.
func (m progressModel) fraction() float64 {
	if m.current.IsZero() || !m.end.After(m.start) {
		return 0
	}

	elapsed := m.current.Sub(m.start).Seconds()
	span := m.end.Sub(m.start).Seconds()

	if span <= 0 {
		return 0
	}

	frac := elapsed / span
	if frac < 0 {
		return 0
	}

	if frac > 1 {
		return 1
	}

	return frac
}

func (m progressModel) formatETA(pct float64) string {
	if pct <= 0 {
		return "--"
	}

	if pct >= 1 {
		return "0s"
	}

	elapsed := time.Since(m.runStart)
	remaining := time.Duration(float64(elapsed) / pct * (1 - pct))

	return remaining.Truncate(time.Second).String()
}

// formatCount renders an integer counter with k/M suffixes for readability.
func formatCount(num int) string {
	switch {
	case num < 1000:
		return fmt.Sprintf("%d", num)
	case num < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(num)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(num)/1_000_000)
	}
}

// stderrIsTerminal reports whether stderr is a terminal capable of rendering
// the bubble tea progress UI. When stderr is a pipe or file (CI logs, output
// redirection), the caller should fall back to plain logging.
func stderrIsTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
