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

package strategy

import (
	"context"
	"time"

	"github.com/jackc/pgtype"
	"github.com/penny-vault/pv-api/data"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
)

// Factory method implementing the Factory pattern to create a new Strategy object
type Factory func(map[string]json.RawMessage) (Strategy, error)

// Argument an argument to a strategy
type Argument struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Typecode    string   `json:"typecode"`
	Default     string   `json:"default"`
	Advanced    bool     `json:"advanced"`
	Options     []string `json:"options"`
}

// Metrics collection of strategy metrics that should be regularly updated
type Metrics struct {
	ID                 uuid.UUID     `json:"id"`
	YTDReturn          pgtype.Float8 `json:"ytdReturn"`
	CagrSinceInception pgtype.Float8 `json:"cagrSinceInception"`
	CagrThreeYr        pgtype.Float4 `json:"cagr3yr"`
	CagrFiveYr         pgtype.Float4 `json:"cagr5yr"`
	CagrTenYr          pgtype.Float4 `json:"cagr10yr"`
	StdDev             pgtype.Float4 `json:"stdDev"`
	DownsideDeviation  pgtype.Float4 `json:"downsideDeviation"`
	MaxDrawDown        pgtype.Float4 `json:"maxDrawDown"`
	AvgDrawDown        pgtype.Float4 `json:"avgDrawDown"`
	SharpeRatio        pgtype.Float4 `json:"sharpeRatio"`
	SortinoRatio       pgtype.Float4 `json:"sortinoRatio"`
	UlcerIndex         pgtype.Float4 `json:"ulcerIndex"`
}

// Info information about a strategy
type Info struct {
	Name            string                       `json:"name"`
	Shortcode       string                       `json:"shortcode"`
	Description     string                       `json:"description"`
	LongDescription string                       `json:"longDescription"`
	Source          string                       `json:"source"`
	Version         string                       `json:"version"`
	Benchmark       data.Security                `json:"benchmark"`
	Arguments       map[string]Argument          `json:"arguments"`
	Suggested       map[string]map[string]string `json:"suggestedParams"`
	Schedule        string                       `json:"Schedule"`
	Metrics         Metrics                      `json:"metrics"`
	Factory         Factory                      `json:"-"`
}

// Strategy an investing strategy
type Strategy interface {
	// Compute calculates the list of historical trades and returns a dataframe. Additionally, it
	// returns a dataframe that indicates what assets to hold at the next trading time.
	Compute(ctx context.Context, begin, end time.Time) (PieList, *Pie, error)
}
