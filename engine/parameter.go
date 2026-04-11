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
	"fmt"
	"reflect"
	"strconv"
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

// ParameterName returns the parameter name used for a strategy struct field.
// It prefers an explicit `pvbt` struct tag and otherwise derives a slug from
// the Go field name by converting PascalCase or camelCase to kebab-case (for
// example `RiskOn` becomes `risk-on`). This function is the single source of
// truth for field-to-parameter-name derivation; both the engine and the CLI
// call it so the names used in `describe`, cobra flags, and `--preset`
// lookups always agree.
func ParameterName(field reflect.StructField) string {
	if tag := field.Tag.Get("pvbt"); tag != "" {
		return tag
	}

	return toKebabCase(field.Name)
}

// IsTestOnlyField reports whether the given strategy struct field is marked as
// test-only via a `testonly:"true"` struct tag. Test-only fields are hidden
// from every user-facing surface (CLI flags, describe output, TUI, presets,
// study sweeps) and cannot be set through ApplyParams. They remain exported
// so that test code can assign them directly.
//
// The tag value must parse as a Go boolean. An unparseable value is a
// programming error in the strategy source code, so this function panics
// rather than silently treating it as false.
func IsTestOnlyField(field reflect.StructField) bool {
	raw, ok := field.Tag.Lookup("testonly")
	if !ok {
		return false
	}

	val, err := strconv.ParseBool(raw)
	if err != nil {
		panic(fmt.Sprintf("strategy field %s: invalid testonly tag %q: %v", field.Name, raw, err))
	}

	return val
}

// toKebabCase converts a PascalCase or camelCase identifier to kebab-case.
// It inserts a hyphen before every upper-case rune that is not the first
// rune, then lower-cases the whole string.
func toKebabCase(s string) string {
	var result strings.Builder

	for idx, runeValue := range s {
		if idx > 0 && runeValue >= 'A' && runeValue <= 'Z' {
			result.WriteByte('-')
		}

		result.WriteRune(runeValue)
	}

	return strings.ToLower(result.String())
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

		name := ParameterName(field)

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
