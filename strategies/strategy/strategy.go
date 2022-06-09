// Copyright 2021 JD Fergason
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

package strategy

import (
	"database/sql"
	"time"

	"github.com/penny-vault/pv-api/data"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/rocketlaunchr/dataframe-go"
)

// StrategyFactory factory method to create strategy
type StrategyFactory func(map[string]json.RawMessage) (Strategy, error)

// Argument an argument to a strategy
type Argument struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Typecode    string   `json:"typecode"`
	Default     string   `json:"default"`
	Advanced    bool     `json:"advanced"`
	Options     []string `json:"options"`
}

// StrategyMetrics collection of strategy metrics that should be regularly updated
type StrategyMetrics struct {
	ID                 uuid.UUID       `json:"id"`
	YTDReturn          sql.NullFloat64 `json:"ytdReturn"`
	CagrSinceInception sql.NullFloat64 `json:"cagrSinceInception"`
	CagrThreeYr        sql.NullFloat64 `json:"cagr3yr"`
	CagrFiveYr         sql.NullFloat64 `json:"cagr5yr"`
	CagrTenYr          sql.NullFloat64 `json:"cagr10yr"`
	StdDev             sql.NullFloat64 `json:"stdDev"`
	DownsideDeviation  sql.NullFloat64 `json:"downsideDeviation"`
	MaxDrawDown        sql.NullFloat64 `json:"maxDrawDown"`
	AvgDrawDown        sql.NullFloat64 `json:"avgDrawDown"`
	SharpeRatio        sql.NullFloat64 `json:"sharpeRatio"`
	SortinoRatio       sql.NullFloat64 `json:"sortinoRatio"`
	UlcerIndex         sql.NullFloat64 `json:"ulcerIndex"`
}

// StrategyInfo information about a strategy
type StrategyInfo struct {
	Name            string                       `json:"name"`
	Shortcode       string                       `json:"shortcode"`
	Description     string                       `json:"description"`
	LongDescription string                       `json:"longDescription"`
	Source          string                       `json:"source"`
	Version         string                       `json:"version"`
	Benchmark       string                       `json:"benchmark"`
	Arguments       map[string]Argument          `json:"arguments"`
	Suggested       map[string]map[string]string `json:"suggestedParams"`
	Schedule        string                       `json:"Schedule"`
	Metrics         StrategyMetrics              `json:"metrics"`
	Factory         StrategyFactory              `json:"-"`
}

type Prediction struct {
	TradeDate     time.Time
	Target        map[string]float64
	Justification map[string]float64
}

// Strategy an investing strategy
type Strategy interface {
	// Compute calculates the list of historical trades and returns a dataframe. Additionally, it
	// returns a dataframe that indicates what assets to hold at the next trading time.
	Compute(manager *data.Manager) (*dataframe.DataFrame, *Prediction, error)
}
