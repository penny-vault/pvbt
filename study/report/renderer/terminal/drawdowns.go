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
	"strings"
	"time"

	"github.com/penny-vault/pvbt/study/report"
)

// renderDrawdowns writes the top drawdown episodes table.
func renderDrawdowns(builder *strings.Builder, drawdowns report.Drawdowns) {
	if len(drawdowns.Entries) == 0 {
		return
	}

	builder.WriteString(sectionTitleStyle.Render("Top Drawdowns"))
	builder.WriteString("\n")

	// Column widths.
	const (
		numCol   = 4
		dateCol  = 12
		depthCol = 10
		durCol   = 12
	)

	// Header.
	header := padRight(tableHeaderStyle.Render("#"), numCol) +
		padRight(tableHeaderStyle.Render("Start"), dateCol) +
		padRight(tableHeaderStyle.Render("End"), dateCol) +
		padRight(tableHeaderStyle.Render("Recovery"), dateCol) +
		padLeft(tableHeaderStyle.Render("Depth"), depthCol) +
		padLeft(tableHeaderStyle.Render("Duration"), durCol)

	builder.WriteString("  " + header + "\n")

	for idx, entry := range drawdowns.Entries {
		recoveryStr := "ongoing"
		if !entry.Recovery.IsZero() {
			recoveryStr = entry.Recovery.Format("2006-01-02")
		}

		endStr := entry.End.Format("2006-01-02")
		if entry.End.Equal(time.Time{}) {
			endStr = "ongoing"
		}

		line := padRight(dimStyle.Render(fmt.Sprintf("%d", idx+1)), numCol) +
			padRight(valueStyle.Render(entry.Start.Format("2006-01-02")), dateCol) +
			padRight(valueStyle.Render(endStr), dateCol) +
			padRight(valueStyle.Render(recoveryStr), dateCol) +
			padLeft(fmtPct(entry.Depth), depthCol) +
			padLeft(valueStyle.Render(fmt.Sprintf("%d days", entry.Days)), durCol)

		builder.WriteString("  " + line + "\n")
	}
}
