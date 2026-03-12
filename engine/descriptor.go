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

package engine

import (
	"github.com/penny-vault/pvbt/asset"
)

// Descriptor is an optional interface strategies can implement to provide
// metadata for serialization. Strategies that don't implement it get empty fields.
type Descriptor interface {
	Describe() StrategyDescription
}

// StrategyDescription holds optional metadata about a strategy.
type StrategyDescription struct {
	ShortCode   string `json:"shortcode,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	Version     string `json:"version,omitempty"`
}

// ParameterInfo is the JSON-serializable form of a Parameter.
type ParameterInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}

// StrategyInfo is the complete serializable description of a strategy.
type StrategyInfo struct {
	Name        string                       `json:"name"`
	ShortCode   string                       `json:"shortcode,omitempty"`
	Description string                       `json:"description,omitempty"`
	Source      string                       `json:"source,omitempty"`
	Version     string                       `json:"version,omitempty"`
	Schedule    string                       `json:"schedule,omitempty"`
	Benchmark   string                       `json:"benchmark,omitempty"`
	RiskFree    string                       `json:"riskFree,omitempty"`
	Parameters  []ParameterInfo              `json:"parameters"`
	Suggestions map[string]map[string]string `json:"suggestions,omitempty"`
}

// DescribeStrategy builds a StrategyInfo from the engine's state after Setup.
// Call this after Backtest or RunLive initialization has completed Setup.
func DescribeStrategy(e *Engine) StrategyInfo {
	info := StrategyInfo{
		Name: e.strategy.Name(),
	}

	// Pull from Descriptor if implemented.
	if d, ok := e.strategy.(Descriptor); ok {
		desc := d.Describe()
		info.ShortCode = desc.ShortCode
		info.Description = desc.Description
		info.Source = desc.Source
		info.Version = desc.Version
	}

	// Engine state from Setup.
	if e.schedule != nil {
		info.Schedule = e.schedule.ScheduleString
	}
	if e.benchmark != (asset.Asset{}) {
		info.Benchmark = e.benchmark.Ticker
	}
	if e.riskFree != (asset.Asset{}) {
		info.RiskFree = e.riskFree.Ticker
	}

	// Parameters and suggestions.
	params := StrategyParameters(e.strategy)
	info.Parameters = make([]ParameterInfo, len(params))
	suggestions := make(map[string]map[string]string)

	for i, p := range params {
		info.Parameters[i] = ParameterInfo{
			Name:        p.Name,
			Description: p.Description,
			Type:        p.GoType.String(),
			Default:     p.Default,
		}

		for sugName, sugVal := range p.Suggestions {
			if suggestions[sugName] == nil {
				suggestions[sugName] = make(map[string]string)
			}
			suggestions[sugName][p.Name] = sugVal
		}
	}

	if len(suggestions) > 0 {
		info.Suggestions = suggestions
	}

	return info
}
