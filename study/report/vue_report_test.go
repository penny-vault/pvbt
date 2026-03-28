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
	"fmt"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/study/report"
)

// testReport is a minimal Report implementation for testing.
type testReport struct {
	name string
	data map[string]any
}

func (tr *testReport) Name() string { return tr.name }

func (tr *testReport) Data(w io.Writer) error {
	return json.NewEncoder(w).Encode(tr.data)
}

var _ report.Report = (*testReport)(nil)

// failingReport always returns an error from Data.
type failingReport struct {
	name string
}

func (fr *failingReport) Name() string { return fr.name }

func (fr *failingReport) Data(_ io.Writer) error {
	return fmt.Errorf("data generation failed")
}

var _ report.Report = (*failingReport)(nil)

var _ = Describe("Render", func() {
	It("produces a valid HTML page with embedded data", func() {
		rpt := &testReport{
			name: "TestComponent",
			data: map[string]any{
				"title":  "My Report",
				"values": []int{1, 2, 3},
			},
		}

		var buf bytes.Buffer
		err := report.Render(rpt, &buf)
		Expect(err).NotTo(HaveOccurred())

		output := buf.String()

		// Verify HTML structure.
		Expect(output).To(ContainSubstring("<!DOCTYPE html>"))
		Expect(output).To(ContainSubstring("<title>TestComponent</title>"))
		Expect(output).To(ContainSubstring("</html>"))

		// Verify data is embedded.
		Expect(output).To(ContainSubstring("__REPORT_DATA__"))
		Expect(output).To(ContainSubstring(`"My Report"`))
		Expect(output).To(ContainSubstring("[1,2,3]"))

		// Verify component name is embedded.
		Expect(output).To(ContainSubstring(`__REPORT_COMPONENT__`))
		Expect(output).To(ContainSubstring(`"TestComponent"`))

		// Verify CDN scripts are present.
		Expect(output).To(ContainSubstring("vue@3"))
		Expect(output).To(ContainSubstring("tailwindcss"))
		Expect(output).To(ContainSubstring("echarts"))

		// Verify the stub bundle is inlined.
		Expect(output).To(ContainSubstring("Vue.createApp"))
	})

	It("returns an error when Data fails", func() {
		rpt := &failingReport{name: "Broken"}

		var buf bytes.Buffer
		err := report.Render(rpt, &buf)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("generating report data"))
	})
})
