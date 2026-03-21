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
	"reflect"
	"strings"
)

// Parameter describes a single configurable field on a strategy struct.
type Parameter struct {
	Name        string            `json:"name"`
	FieldName   string            `json:"fieldName"`
	Description string            `json:"description,omitempty"`
	GoType      reflect.Type      `json:"-"`
	Default     string            `json:"default,omitempty"`
	Suggestions map[string]string `json:"suggestions,omitempty"` // preset name -> value
}

// StrategyParameters reflects over the strategy struct and returns metadata
// for each exported field. Used by the CLI to generate flags and by UIs to
// build configuration forms.
func StrategyParameters(s Strategy) []Parameter {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	paramType := v.Type()
	if paramType.Kind() != reflect.Struct {
		return nil
	}

	var params []Parameter

	for ii := 0; ii < paramType.NumField(); ii++ {
		field := paramType.Field(ii)
		if !field.IsExported() {
			continue
		}

		// Skip Strategy-typed fields -- these are children, not parameters.
		if field.Type.Implements(strategyType) ||
			(field.Type.Kind() == reflect.Pointer && field.Type.Elem().Implements(strategyType)) {
			continue
		}

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = strings.ToLower(field.Name)
		}

		var suggestions map[string]string
		if suggestTag := field.Tag.Get("suggest"); suggestTag != "" {
			suggestions = parseSuggestions(suggestTag)
		}

		params = append(params, Parameter{
			Name:        name,
			FieldName:   field.Name,
			Description: field.Tag.Get("desc"),
			GoType:      field.Type,
			Default:     field.Tag.Get("default"),
			Suggestions: suggestions,
		})
	}

	return params
}

// parseSuggestions parses a pipe-delimited suggest tag value into a map.
// Format: "Name1=value1|Name2=value2"
func parseSuggestions(tag string) map[string]string {
	paramMap := make(map[string]string)

	for _, entry := range strings.Split(tag, "|") {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			paramMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	if len(paramMap) == 0 {
		return nil
	}

	return paramMap
}
