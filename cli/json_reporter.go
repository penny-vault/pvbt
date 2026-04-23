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
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/penny-vault/pvbt/engine"
	"github.com/rs/zerolog/log"
)

type jsonReporter struct {
	w         io.Writer
	startTime time.Time
}

func newJSONReporter(w io.Writer) *jsonReporter {
	return &jsonReporter{w: w, startTime: time.Now()}
}

type statusStartedMsg struct {
	Type     string  `json:"type"`
	Event    string  `json:"event"`
	RunID    string  `json:"run_id"`
	Strategy string  `json:"strategy"`
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Cash     float64 `json:"cash"`
	Output   string  `json:"output"`
	Time     string  `json:"time"`
}

type statusCompletedMsg struct {
	Type      string `json:"type"`
	Event     string `json:"event"`
	RunID     string `json:"run_id"`
	Output    string `json:"output"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Time      string `json:"time"`
}

type statusErrorMsg struct {
	Type  string `json:"type"`
	Event string `json:"event"`
	RunID string `json:"run_id"`
	Error string `json:"error"`
	Time  string `json:"time"`
}

type progressLineMsg struct {
	Type         string  `json:"type"`
	Step         int     `json:"step"`
	TotalSteps   int     `json:"total_steps"`
	CurrentDate  string  `json:"current_date"`
	TargetDate   string  `json:"target_date"`
	Pct          float64 `json:"pct"`
	ElapsedMS    int64   `json:"elapsed_ms"`
	EtaMS        int64   `json:"eta_ms"`
	Measurements int     `json:"measurements"`
}

func (r *jsonReporter) Started(runID, strategy, startDate, endDate string, cash float64, output string) {
	r.writeJSON(statusStartedMsg{
		Type:     "status",
		Event:    "started",
		RunID:    runID,
		Strategy: strategy,
		Start:    startDate,
		End:      endDate,
		Cash:     cash,
		Output:   output,
		Time:     time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *jsonReporter) Completed(runID, output string) {
	r.writeJSON(statusCompletedMsg{
		Type:      "status",
		Event:     "completed",
		RunID:     runID,
		Output:    output,
		ElapsedMS: time.Since(r.startTime).Milliseconds(),
		Time:      time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *jsonReporter) Error(runID string, err error) {
	r.writeJSON(statusErrorMsg{
		Type:  "status",
		Event: "error",
		RunID: runID,
		Error: err.Error(),
		Time:  time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *jsonReporter) Progress(ev engine.ProgressEvent) {
	pct := progressFraction(ev)
	elapsed := time.Since(r.startTime)
	r.writeJSON(progressLineMsg{
		Type:         "progress",
		Step:         ev.Step,
		TotalSteps:   ev.TotalSteps,
		CurrentDate:  ev.Date.Format("2006-01-02"),
		TargetDate:   ev.End.Format("2006-01-02"),
		Pct:          math.Round(pct*10000) / 100,
		ElapsedMS:    elapsed.Milliseconds(),
		EtaMS:        computeEtaMS(elapsed, pct),
		Measurements: ev.MeasurementsEvaluated,
	})
}

func (r *jsonReporter) writeJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Msg("json reporter: marshal failed")
		return
	}

	if _, err := fmt.Fprintln(r.w, string(b)); err != nil {
		log.Error().Err(err).Msg("json reporter: write failed")
	}
}

// progressFraction computes completion as a fraction in [0,1] from the date span.
func progressFraction(ev engine.ProgressEvent) float64 {
	if ev.Date.IsZero() || !ev.End.After(ev.Start) {
		return 0
	}

	span := ev.End.Sub(ev.Start).Seconds()
	if span <= 0 {
		return 0
	}

	frac := ev.Date.Sub(ev.Start).Seconds() / span
	switch {
	case frac < 0:
		return 0
	case frac > 1:
		return 1
	default:
		return frac
	}
}

// computeEtaMS returns estimated milliseconds remaining, or 0 if pct is 0 or 1.
func computeEtaMS(elapsed time.Duration, pct float64) int64 {
	if pct <= 0 || pct >= 1 {
		return 0
	}

	return time.Duration(float64(elapsed) / pct * (1 - pct)).Milliseconds()
}
