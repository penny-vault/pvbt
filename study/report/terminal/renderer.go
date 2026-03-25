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
	"io"

	"github.com/penny-vault/pvbt/study/report"
)

// Render writes a plain-text backtest report to the given writer.
// It delegates to the composable report's own Render method.
func Render(rpt report.Report, writer io.Writer) error {
	return rpt.Render(report.FormatText, writer)
}
