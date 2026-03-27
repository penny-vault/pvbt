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

package summary

import (
	"math"
	"testing"
)

func TestAnnualizeTWRR(t *testing.T) {
	tests := []struct {
		name     string
		twrr     float64
		years    float64
		expected float64
	}{
		{
			name:     "10% over 2 years",
			twrr:     0.10,
			years:    2.0,
			expected: math.Pow(1.10, 1.0/2.0) - 1,
		},
		{
			name:     "100% over 5 years",
			twrr:     1.0,
			years:    5.0,
			expected: math.Pow(2.0, 1.0/5.0) - 1,
		},
		{
			name:     "exactly 1 year is identity",
			twrr:     0.25,
			years:    1.0,
			expected: 0.25,
		},
		{
			name:     "negative return",
			twrr:     -0.20,
			years:    3.0,
			expected: math.Pow(0.80, 1.0/3.0) - 1,
		},
		{
			name:     "NaN input produces NaN",
			twrr:     math.NaN(),
			years:    1.0,
			expected: math.NaN(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := annualizeTWRR(tc.twrr, tc.years)
			if math.IsNaN(tc.expected) {
				if !math.IsNaN(got) {
					t.Errorf("expected NaN, got %f", got)
				}

				return
			}

			if math.Abs(got-tc.expected) > 1e-10 {
				t.Errorf("annualizeTWRR(%f, %f) = %f, want %f", tc.twrr, tc.years, got, tc.expected)
			}
		})
	}
}
