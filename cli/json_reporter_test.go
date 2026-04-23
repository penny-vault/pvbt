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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/engine"
)

// parseLines parses all newline-delimited JSON objects written to buf.
func parseLines(buf *bytes.Buffer) []map[string]any {
	GinkgoHelper()
	var results []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		Expect(json.Unmarshal([]byte(line), &m)).To(Succeed())
		results = append(results, m)
	}
	return results
}

var _ = Describe("progressFraction", func() {
	It("returns 0 when Date is zero", func() {
		ev := engine.ProgressEvent{
			Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		Expect(progressFraction(ev)).To(Equal(0.0))
	})

	It("returns 0 when End is not after Start", func() {
		t := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		ev := engine.ProgressEvent{Start: t, End: t, Date: t}
		Expect(progressFraction(ev)).To(Equal(0.0))
	})

	It("returns ~0.5 for the midpoint date", func() {
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
		mid := start.Add(end.Sub(start) / 2)
		ev := engine.ProgressEvent{Start: start, End: end, Date: mid}
		Expect(progressFraction(ev)).To(BeNumerically("~", 0.5, 0.01))
	})

	It("clamps to 1 when Date exceeds End", func() {
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
		ev := engine.ProgressEvent{Start: start, End: end, Date: end.AddDate(1, 0, 0)}
		Expect(progressFraction(ev)).To(Equal(1.0))
	})
})

var _ = Describe("computeEtaMS", func() {
	It("returns 0 when pct is 0", func() {
		Expect(computeEtaMS(10*time.Second, 0)).To(Equal(int64(0)))
	})

	It("returns 0 when pct is 1", func() {
		Expect(computeEtaMS(10*time.Second, 1.0)).To(Equal(int64(0)))
	})

	It("returns approximate remaining ms at 25% completion", func() {
		// 25% done in 10s → 30s remaining
		eta := computeEtaMS(10*time.Second, 0.25)
		Expect(eta).To(BeNumerically("~", int64(30000), int64(100)))
	})
})

var _ = Describe("jsonReporter", func() {
	var (
		buf      *bytes.Buffer
		reporter *jsonReporter
	)

	BeforeEach(func() {
		buf = &bytes.Buffer{}
		reporter = newJSONReporter(buf)
	})

	Describe("Started", func() {
		It("emits a started status line with all fields", func() {
			reporter.Started("run123", "my-strategy", "2020-01-01", "2025-01-01", 100000, "/tmp/out.db")
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("status"))
			Expect(m["event"]).To(Equal("started"))
			Expect(m["run_id"]).To(Equal("run123"))
			Expect(m["strategy"]).To(Equal("my-strategy"))
			Expect(m["start"]).To(Equal("2020-01-01"))
			Expect(m["end"]).To(Equal("2025-01-01"))
			Expect(m["cash"]).To(BeNumerically("~", 100000.0, 0.01))
			Expect(m["output"]).To(Equal("/tmp/out.db"))
			Expect(m["time"]).NotTo(BeEmpty())
		})
	})

	Describe("Completed", func() {
		It("emits a completed status line with elapsed_ms", func() {
			reporter.Completed("run123", "/tmp/out.db")
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("status"))
			Expect(m["event"]).To(Equal("completed"))
			Expect(m["run_id"]).To(Equal("run123"))
			Expect(m["output"]).To(Equal("/tmp/out.db"))
			Expect(m["elapsed_ms"]).To(BeNumerically(">=", float64(0)))
			Expect(m["time"]).NotTo(BeEmpty())
		})
	})

	Describe("Error", func() {
		It("emits an error status line with the error message", func() {
			reporter.Error("run123", fmt.Errorf("data provider unavailable"))
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("status"))
			Expect(m["event"]).To(Equal("error"))
			Expect(m["run_id"]).To(Equal("run123"))
			Expect(m["error"]).To(Equal("data provider unavailable"))
			Expect(m["time"]).NotTo(BeEmpty())
		})
	})

	Describe("Progress", func() {
		It("emits a progress line with all fields", func() {
			start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			mid := start.Add(end.Sub(start) / 2)
			ev := engine.ProgressEvent{
				Step:                  500,
				TotalSteps:            1000,
				Date:                  mid,
				Start:                 start,
				End:                   end,
				MeasurementsEvaluated: 12345,
			}
			reporter.Progress(ev)
			lines := parseLines(buf)
			Expect(lines).To(HaveLen(1))
			m := lines[0]
			Expect(m["type"]).To(Equal("progress"))
			Expect(m["step"]).To(BeNumerically("~", float64(500), 1))
			Expect(m["total_steps"]).To(BeNumerically("~", float64(1000), 1))
			Expect(m["current_date"]).To(Equal(mid.Format("2006-01-02")))
			Expect(m["target_date"]).To(Equal("2025-01-01"))
			Expect(m["pct"]).To(BeNumerically("~", float64(50), 1))
			Expect(m["elapsed_ms"]).To(BeNumerically(">=", float64(0)))
			Expect(m["measurements"]).To(BeNumerically("~", float64(12345), 1))
		})
	})
})

var _ = Describe("backtest --json flag", func() {
	It("is registered on the backtest command", func() {
		strategy := &testStrategy{}
		cmd := newBacktestCmd(strategy)
		f := cmd.Flags().Lookup("json")
		Expect(f).NotTo(BeNil())
		Expect(f.Value.Type()).To(Equal("bool"))
	})
})
