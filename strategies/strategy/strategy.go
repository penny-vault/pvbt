package strategy

import (
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
	DefaultVal  string   `json:"default"`
	Advanced    bool     `json:"advanced"`
	Options     []string `json:"options"`
}

// StrategyMetrics collection of strategy metrics that should be regularly updated
type StrategyMetrics struct {
	ID                 uuid.UUID `json:"id"`
	YTDReturn          float32   `json:"ytd_return"`
	CagrSinceInception float32   `json:"cagr_since_inception"`
	CagrThreeYr        float32   `json:"cagr_3yr"`
	CagrFiveYr         float32   `json:"cagr_5yr"`
	CagrTenYr          float32   `json:"cagr_10yr"`
	StdDev             float32   `json:"std_dev"`
	DownsideDeviation  float32   `json:"downside_deviation"`
	MaxDrawDown        float32   `json:"max_draw_down"`
	AvgDrawDown        float32   `json:"avg_draw_down"`
	SharpeRatio        float32   `json:"sharpe_ratio"`
	SortinoRatio       float32   `json:"sortino_ratio"`
	UlcerIndex         float32   `json:"ulcer_index"`
}

// StrategyInfo information about a strategy
type StrategyInfo struct {
	Name                string                       `json:"name"`
	Shortcode           string                       `json:"shortcode"`
	Description         string                       `json:"description"`
	Source              string                       `json:"source"`
	Version             string                       `json:"version"`
	Arguments           map[string]Argument          `json:"arguments"`
	SuggestedParameters map[string]map[string]string `json:"suggestedParams"`
	Metrics             StrategyMetrics              `json:"metrics"`
	Factory             StrategyFactory              `json:"-"`
}

// Strategy an investing strategy
type Strategy interface {
	Compute(manager *data.Manager) (*portfolio.Portfolio, error)
}
