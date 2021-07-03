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
	YTDReturn          sql.NullFloat64 `json:"ytd_return"`
	CagrSinceInception sql.NullFloat64 `json:"cagr_since_inception"`
	CagrThreeYr        sql.NullFloat64 `json:"cagr_3yr"`
	CagrFiveYr         sql.NullFloat64 `json:"cagr_5yr"`
	CagrTenYr          sql.NullFloat64 `json:"cagr_10yr"`
	StdDev             sql.NullFloat64 `json:"std_dev"`
	DownsideDeviation  sql.NullFloat64 `json:"downside_deviation"`
	MaxDrawDown        sql.NullFloat64 `json:"max_draw_down"`
	AvgDrawDown        sql.NullFloat64 `json:"avg_draw_down"`
	SharpeRatio        sql.NullFloat64 `json:"sharpe_ratio"`
	SortinoRatio       sql.NullFloat64 `json:"sortino_ratio"`
	UlcerIndex         sql.NullFloat64 `json:"ulcer_index"`
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
