package strategies

import (
	"encoding/json"
	"main/data"
)

type StrategyFactory func(map[string]json.RawMessage) (Strategy, error)

type Argument struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Typecode    string   `json:"typecode"`
	DefaultVal  string   `json:"default"`
	Options     []string `json:"options"`
}

type StrategyInfo struct {
	Name        string              `json:"name"`
	Shortcode   string              `json:"shortcode"`
	Description string              `json:"description"`
	Source      string              `json:"source"`
	Version     string              `json:"version"`
	YTDGain     float64             `json:"ytd_gain"`
	Arguments   map[string]Argument `json:"arguments"`
	Factory     StrategyFactory     `json:"-"`
}

type PerformanceMeasurement struct {
	Time   int64   `json:"time"`
	Return float64 `json:"return"`
}

type StrategyPerformance struct {
	StrategyInformation StrategyInfo             `json:"strategy"`
	PeriodStart         int64                    `json:"period.start"`
	PeriodEnd           int64                    `json:"period.end"`
	Return              []PerformanceMeasurement `json:"return"`
}

type Strategy interface {
	GetInfo() StrategyInfo
	Compute(manager *data.Manager) (StrategyPerformance, error)
}
