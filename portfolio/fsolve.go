// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package portfolio

import (
	"errors"
	"math"
)

var (
	ErrDidNotConverge = errors.New("did not converge")
)

type objectiveFunc func(float64) float64

// fsolve uses a combination of bisection and false position to find a
// root of a function within a given interval. This is guaranteed to converge,
// and always keeps a bounding interval, unlike Newton's method. Inputs are:
//  f:  function to solve root for
//  f0: initial guess of root
// Returns:
//  root: root value
//  err: if an error occurred
func fsolve(f objectiveFunc, f0 float64) (float64, error) {
	// The false position steps are either unmodified, or modified with the
	// Anderson-Bjorck method as appropriate. Theoretically, this has a "speed of
	// convergence" of 1.7 (bisection is 1, Newton is 2).

	const (
		maxIterations = 500
		bisectIter    = 4
		bisectWidth   = 1.0
	)

	const (
		bisect = iota + 1
		falseP
	)

	var state uint8 = falseP
	gamma := 1.0
	tol := .0001
	x1 := 0.0
	x2 := 1.0

	f1 := f(x1)
	f2 := f(x2)

	w := math.Abs(x2 - x1)
	lastBisectWidth := w

	var nFalseP int
	var x3, f3, bestX float64
	for i := 0; i < maxIterations; i++ {
		switch state {
		case bisect:
			x3 = 0.5 * (x1 + x2)
			if x3 == x1 || x3 == x2 {
				// i.e., x1 and x2 are successive floating-point numbers
				bestX = x3
				return bestX, nil
			}

			f3 = f(x3)
			if f3 == 0 {
				return x3, nil
			}

			if f3*f2 < 0 {
				x1 = x2
				f1 = f2
			}
			x2 = x3
			f2 = f3
			w = math.Abs(x2 - x1)
			lastBisectWidth = w
			gamma = 1.0
			nFalseP = 0
			state = falseP
		case falseP:
			s12 := (f2 - gamma*f1) / (x2 - x1)
			x3 = x2 - f2/s12
			f3 = f(x3)
			if f3 == 0 {
				return x3, nil
			}

			nFalseP++
			if f3*f2 < 0 {
				gamma = 1.0
				x1 = x2
				f1 = f2
			} else {
				// Anderson-Bjorck method
				g := 1.0 - f3/f2
				if g <= 0 {
					g = 0.5
				}
				gamma *= g
			}
			x2 = x3
			f2 = f3
			w = math.Abs(x2 - x1)

			// Sanity check. For every 4 false position checks, see if we really are
			// decreasing the interval by comparing to what bisection would have
			// achieved (or, rather, a bit more lenient than that -- interval
			// decreased by 4 instead of by 16, as the fp could be decreasing gamma
			// for a bit). Note that this should guarantee convergence, as it makes
			// sure that we always end up decreasing the interval width with a
			// bisection.
			if nFalseP > bisectIter {
				if w*bisectWidth > lastBisectWidth {
					state = bisect
				}
				nFalseP = 0
				lastBisectWidth = w
			}
		}

		if w <= tol {
			if math.Abs(f1) < math.Abs(f2) {
				bestX = x1
			} else {
				bestX = x2
			}
			return bestX, nil
		}
	}

	return f0, ErrDidNotConverge
}
