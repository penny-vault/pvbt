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
	"time"
)

// Descriptor is an optional interface strategies can implement to provide
// metadata for serialization. Strategies that don't implement it get empty fields.
type Descriptor interface {
	Describe() StrategyDescription
}

// StrategyDescription holds optional metadata about a strategy.
type StrategyDescription struct {
	ShortCode   string    `json:"shortcode,omitempty"`
	Description string    `json:"description,omitempty"`
	Source      string    `json:"source,omitempty"`
	Version     string    `json:"version,omitempty"`
	VersionDate time.Time `json:"versionDate,omitzero"`
	Schedule    string    `json:"schedule,omitempty"`
	Benchmark   string    `json:"benchmark,omitempty"`
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
	VersionDate time.Time                    `json:"versionDate,omitzero"`
	Schedule    string                       `json:"schedule,omitempty"`
	Benchmark   string                       `json:"benchmark,omitempty"`
	RiskFree    string                       `json:"riskFree,omitempty"`
	Parameters  []ParameterInfo              `json:"parameters"`
	Suggestions map[string]map[string]string `json:"suggestions,omitempty"`
}

// DescribeStrategy builds a StrategyInfo from the strategy's metadata.
func DescribeStrategy(strategy Strategy) StrategyInfo {
	info := StrategyInfo{
		Name: strategy.Name(),
	}

	if desc, ok := strategy.(Descriptor); ok {
		description := desc.Describe()
		info.ShortCode = description.ShortCode
		info.Description = description.Description
		info.Source = description.Source
		info.Version = description.Version
		info.VersionDate = description.VersionDate
		info.Schedule = description.Schedule
		info.Benchmark = description.Benchmark
	}

	info.RiskFree = "DGS3MO"

	params := StrategyParameters(strategy)
	info.Parameters = make([]ParameterInfo, len(params))
	suggestions := make(map[string]map[string]string)

	for idx, param := range params {
		info.Parameters[idx] = ParameterInfo{
			Name:        param.Name,
			Description: param.Description,
			Type:        param.GoType.String(),
			Default:     param.Default,
		}

		for sugName, sugVal := range param.Suggestions {
			if suggestions[sugName] == nil {
				suggestions[sugName] = make(map[string]string)
			}

			suggestions[sugName][param.Name] = sugVal
		}
	}

	if len(suggestions) > 0 {
		info.Suggestions = suggestions
	}

	return info
}
