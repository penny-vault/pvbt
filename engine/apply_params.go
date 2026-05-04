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
	"sort"
	"strings"
)

// ApplyParams resolves an optional preset and merges explicit parameter
// overrides onto the engine's strategy. When preset is non-empty, the
// strategy must implement Descriptor so that DescribeStrategy can produce the
// aggregated Suggestions map. Explicit params always take precedence over
// preset values. After all simple-type fields are set via applyParamValue,
// hydrateFields is called to resolve asset.Asset and universe.Universe fields.
func ApplyParams(eng *Engine, preset string, params map[string]string) error {
	merged := make(map[string]string, len(params))

	if preset != "" {
		// The strategy must implement Descriptor for presets to be available.
		if _, ok := eng.strategy.(Descriptor); !ok {
			return fmt.Errorf("ApplyParams: strategy %q does not implement Descriptor; cannot resolve preset %q",
				eng.strategy.Name(), preset)
		}

		info := DescribeStrategy(eng.strategy)

		presetValues, found := info.Suggestions[preset]
		if !found {
			available := make([]string, 0, len(info.Suggestions))
			for name := range info.Suggestions {
				available = append(available, name)
			}

			sort.Strings(available)

			return fmt.Errorf("ApplyParams: preset %q not found on strategy %q; available presets: %s",
				preset, eng.strategy.Name(), strings.Join(available, ", "))
		}

		for paramName, paramValue := range presetValues {
			merged[paramName] = paramValue
		}
	}

	// Explicit params override preset values.
	for paramName, paramValue := range params {
		merged[paramName] = paramValue
	}

	// Reject any merged param that targets a test-only field. Test-only
	// parameters are deliberately invisible to user surfaces and must not
	// be settable via ApplyParams; tests should construct strategies with
	// direct field assignment instead.
	testOnly := testOnlyParamNames(eng.strategy)
	for paramName := range merged {
		if _, isTestOnly := testOnly[paramName]; isTestOnly {
			return fmt.Errorf("ApplyParams: parameter %q is test-only and cannot be set via ApplyParams; assign the field directly on the strategy struct", paramName)
		}
	}

	// Apply all merged parameters via applyParamValue, and mark them on
	// the engine so hydrateFields will not re-default them. This is what
	// preserves an explicit zero (e.g. preset value "0" against a non-zero
	// struct-tag default) instead of having the default silently win.
	for paramName, paramValue := range merged {
		if err := applyParamValue(eng.strategy, paramName, paramValue); err != nil {
			return fmt.Errorf("ApplyParams: applying param %q=%q: %w", paramName, paramValue, err)
		}

		eng.MarkUserParams(paramName)
	}

	// Resolve asset.Asset and universe.Universe fields, and apply struct-tag
	// defaults for any field the caller did not explicitly set.
	if err := hydrateFields(eng, eng.strategy); err != nil {
		return fmt.Errorf("ApplyParams: hydrating fields: %w", err)
	}

	return nil
}

// testOnlyParamNames returns the set of parameter names (as kebab-case)
// belonging to fields marked testonly:"true" on the given strategy.
func testOnlyParamNames(strategy Strategy) map[string]struct{} {
	names := make(map[string]struct{})

	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return names
	}

	targetType := val.Type()
	for ii := 0; ii < targetType.NumField(); ii++ {
		field := targetType.Field(ii)
		if !field.IsExported() {
			continue
		}

		if !IsTestOnlyField(field) {
			continue
		}

		names[ParameterName(field)] = struct{}{}
	}

	return names
}
