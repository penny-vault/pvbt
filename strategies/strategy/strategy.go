package strategy

import (
	"database/sql"
	"main/data"
	"main/portfolio"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
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
	Metrics         StrategyMetrics              `json:"metrics"`
	Factory         StrategyFactory              `json:"-"`
}

// Strategy an investing strategy
type Strategy interface {
	Compute(manager *data.Manager) (*portfolio.Portfolio, error)
}
