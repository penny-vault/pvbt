package strategies

import (
	"encoding/json"
	"main/data"
	"main/portfolio"
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

type Strategy interface {
	GetInfo() StrategyInfo
	Compute(manager *data.Manager) (portfolio.Performance, error)
}
