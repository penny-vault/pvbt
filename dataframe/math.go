// Copyright 2021-2022
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

package dataframe

import (
	"math"

	"github.com/rs/zerolog/log"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat"
)

// AddScalar adds the scalar value to all columns in dataframe df and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) AddScalar(scalar float64) *DataFrame {
	df = df.Copy()

	for colIdx := range df.ColNames {
		for rowIdx := range df.Vals[colIdx] {
			df.Vals[colIdx][rowIdx] += scalar
		}
	}
	return df
}

// AddVec adds the vector to all columns in dataframe and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) AddVec(vec []float64) *DataFrame {
	df = df.Copy()
	for idx := range df.ColNames {
		floats.Add(df.Vals[idx], vec)
	}
	return df
}

// Count creates a new dataframe with the number of columns where the expression lambda func(float64) bool evaluates to true is placed
// in the `count` column
func (df *DataFrame) Count(lambda func(x float64) bool) *DataFrame {
	res := &DataFrame{
		Dates:    df.Dates,
		Vals:     [][]float64{make([]float64, df.Len())},
		ColNames: []string{"count"},
	}

	for rowIdx := range df.Dates {
		cnt := 0
		for _, col := range df.Vals {
			if lambda(col[rowIdx]) {
				cnt++
			}
		}
		res.Vals[0][rowIdx] = float64(cnt)
	}

	return res
}

// Div divides all columns in `df` by the corresponding column in `other` and returns a new dataframe.
// Panics if rows are not equal.
func (df *DataFrame) Div(other *DataFrame) *DataFrame {
	df = df.Copy()

	otherMap := make(map[string]int, len(other.ColNames))
	for idx, val := range other.ColNames {
		otherMap[val] = idx
	}

	for idx, colName := range df.ColNames {
		if otherIdx, ok := otherMap[colName]; ok {
			floats.Div(df.Vals[idx], other.Vals[otherIdx])
		}
	}
	return df
}

// Mean calculates the mean of all like columns in the dataframes and returns a new dataframe
// panics if rows are not equal.
func Mean(dfs ...*DataFrame) *DataFrame {
	resDf := dfs[0].Copy()

	otherMaps := make([]map[string]int, len(dfs))
	for dfIdx, resDf := range dfs {
		otherMaps[dfIdx] = make(map[string]int, len(resDf.ColNames))
		for idx, val := range resDf.ColNames {
			otherMaps[dfIdx][val] = idx
		}
	}

	for resColIdx, colName := range resDf.ColNames {
		for rowIdx := range resDf.Vals[0] {
			row := 0.0
			cnt := 0.0
			for dfIdx := range dfs {
				df := dfs[dfIdx]
				colIdx := otherMaps[dfIdx][colName]
				row += df.Vals[colIdx][rowIdx]
				cnt++
			}
			resDf.Vals[resColIdx][rowIdx] = row / cnt
		}
	}

	return resDf
}

// Mul multiplies all columns in dataframe df by the corresponding column in dataframe other and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) Mul(other *DataFrame) *DataFrame {
	df = df.Copy()

	otherMap := make(map[string]int, len(other.ColNames))
	for idx, val := range other.ColNames {
		otherMap[val] = idx
	}

	for idx, colName := range df.ColNames {
		if otherIdx, ok := otherMap[colName]; ok {
			floats.Mul(df.Vals[idx], other.Vals[otherIdx])
		}
	}
	return df
}

// MulScalar multiplies all columns in dataframe df by the scalar and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) MulScalar(scalar float64) *DataFrame {
	df = df.Copy()

	for colIdx := range df.ColNames {
		for rowIdx := range df.Vals[colIdx] {
			df.Vals[colIdx][rowIdx] *= scalar
		}
	}
	return df
}

// RollingSumScaled computes ∑ df[ii] * scalar and returns a new dataframe
// panics if rows are not equal.
func (df *DataFrame) RollingSumScaled(ii int, scalar float64) *DataFrame {
	df2 := df.Copy()
	for colIdx := range df.ColNames {
		roll := 0.0
		dropIdx := 0
		for rowIdx := range df.Vals[colIdx] {
			switch {
			case rowIdx >= ii:
				roll += df.Vals[colIdx][rowIdx]
				roll -= df.Vals[colIdx][dropIdx]
				df2.Vals[colIdx][rowIdx] = roll * scalar
				dropIdx++
			case rowIdx == (ii - 1):
				roll += df.Vals[colIdx][rowIdx]
				df2.Vals[colIdx][rowIdx] = roll * scalar
			default:
				df2.Vals[colIdx][rowIdx] = math.NaN()
				roll += df.Vals[colIdx][rowIdx]
			}
		}
	}
	return df2
}

// SMA computes the simple moving average of all the columns in df for the specified
// lookback period. The length of the resulting dataframe equals that of the input with NaNs during the warm-up period.
// Invalid lookback periods result in a dataframe of all NaN.
// NOTE: lookback is in terms of date periods. if the dataframe is sampled monthly then SMA is monthly,
func (df *DataFrame) SMA(lookback int) *DataFrame {
	// check that lookback is a valid period
	if (lookback > df.Len()) || (lookback <= 0) {
		log.Error().Stack().Int("Lookback", lookback).Int("NRows", df.Len()).Msg("lookback must be: 0 < lookback <= NRows")
		nullDf := &DataFrame{
			Dates:    df.Dates,
			Vals:     make([][]float64, df.ColCount()),
			ColNames: df.ColNames,
		}
		for colIdx := range nullDf.Vals {
			nullDf.Vals[colIdx] = make([]float64, df.Len())
			for rowIdx := range nullDf.Vals[colIdx] {
				nullDf.Vals[colIdx][rowIdx] = math.NaN()
			}
		}
		return nullDf
	}

	filterBank := make([][]float64, df.ColCount())
	for idx := range filterBank {
		filterBank[idx] = make([]float64, lookback)
	}

	smaVals := make([][]float64, df.ColCount())
	for idx := range smaVals {
		smaVals[idx] = make([]float64, df.Len())
	}

	warmup := true

	for rowIdx := range df.Dates {
		// if we have seen at least lookback rows then we are out of the warmup period
		// NOTE: row is 0 based, lookback is 1 based; hence the test applied below
		if rowIdx == (lookback - 1) {
			warmup = false
		}

		filterBankIdx := rowIdx % lookback

		for colIdx := range df.Vals {
			filterBank[colIdx][filterBankIdx] = df.Vals[colIdx][rowIdx]
			if warmup {
				smaVals[colIdx][rowIdx] = math.NaN()
			} else {
				smaVals[colIdx][rowIdx] = stat.Mean(filterBank[colIdx], nil)
			}
		}
	}

	smaDf := &DataFrame{
		Dates:    df.Dates,
		Vals:     smaVals,
		ColNames: df.ColNames,
	}

	return smaDf
}
