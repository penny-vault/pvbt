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
	Name        string       // from pvbt tag, or field name lowercased
	FieldName   string       // Go struct field name
	Description string       // from desc tag
	GoType      reflect.Type // field's Go type
	Default     string       // from default tag
}

// StrategyParameters reflects over the strategy struct and returns metadata
// for each exported field. Used by the CLI to generate flags and by UIs to
// build configuration forms.
func StrategyParameters(s Strategy) []Parameter {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return nil
	}

	var params []Parameter
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = strings.ToLower(field.Name)
		}

		params = append(params, Parameter{
			Name:        name,
			FieldName:   field.Name,
			Description: field.Tag.Get("desc"),
			GoType:      field.Type,
			Default:     field.Tag.Get("default"),
		})
	}

	return params
}
