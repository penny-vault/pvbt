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
	"time"

	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
)

// childEntry holds all bookkeeping for a single child strategy discovered
// on a parent strategy struct via reflection.
type childEntry struct {
	strategy Strategy
	name     string  // from pvbt tag or lowercased field name
	weight   float64 // from weight tag
	schedule *tradecron.TradeCron
	account  portfolio.PortfolioManager
	broker   *SimulatedBroker
}

// discoverChildren reflects over parentStrategy's exported fields and
// populates e.children / e.childrenByName for every field that implements
// Strategy and carries a `weight` struct tag. Presets and parameter
// overrides are applied immediately; hydration, Setup, and portfolio
// creation are deferred to the Backtest caller.
func (eng *Engine) discoverChildren(parentStrategy Strategy, visited map[uintptr]bool) error {
	val := reflect.ValueOf(parentStrategy)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	parentType := val.Type()
	if parentType.Kind() != reflect.Struct {
		return nil
	}

	if eng.childrenByName == nil {
		eng.childrenByName = make(map[string]*childEntry)
	}

	for fieldIdx := 0; fieldIdx < parentType.NumField(); fieldIdx++ {
		field := parentType.Field(fieldIdx)
		if !field.IsExported() {
			continue
		}

		// Check if the field implements Strategy (directly or via pointer).
		implementsStrategy := field.Type.Implements(strategyType)

		pointerImplementsStrategy := field.Type.Kind() == reflect.Pointer &&
			field.Type.Elem().Implements(strategyType)
		if !implementsStrategy && !pointerImplementsStrategy {
			continue
		}

		// Must have a weight tag to be treated as a child.
		weightTag := field.Tag.Get("weight")
		if weightTag == "" {
			continue
		}

		parsedWeight, err := strconv.ParseFloat(weightTag, 64)
		if err != nil {
			return fmt.Errorf("discoverChildren: field %s: parsing weight %q: %w",
				field.Name, weightTag, err)
		}

		// Determine name from pvbt tag or lowercased field name.
		childName := field.Tag.Get("pvbt")
		if childName == "" {
			childName = strings.ToLower(field.Name)
		}

		// Get the field value; allocate nil pointers.
		fieldValue := val.Field(fieldIdx)

		// Interface-typed fields (e.g. `Strategy`) hold the concrete value
		// directly. Skip nil interfaces since we cannot allocate them.
		if field.Type.Kind() == reflect.Interface {
			if fieldValue.IsNil() {
				continue
			}
		} else if field.Type.Kind() == reflect.Pointer && fieldValue.IsNil() {
			allocated := reflect.New(field.Type.Elem())
			fieldValue.Set(allocated)
		}

		var childStrategy Strategy
		if field.Type.Kind() == reflect.Interface {
			// The field already holds a concrete Strategy value.
			childStrategy = fieldValue.Interface().(Strategy)
		} else if field.Type.Kind() == reflect.Pointer {
			childStrategy = fieldValue.Interface().(Strategy)
		} else {
			// Value type -- take its address so we get a *T.
			if fieldValue.CanAddr() {
				childStrategy = fieldValue.Addr().Interface().(Strategy)
			} else {
				childStrategy = fieldValue.Interface().(Strategy)
			}
		}

		// Cycle detection.
		childPtr := reflect.ValueOf(childStrategy).Pointer()
		if visited[childPtr] {
			return fmt.Errorf("discoverChildren: cycle detected on field %s", field.Name)
		}

		visited[childPtr] = true

		// Apply preset if tag is present.
		if presetTag := field.Tag.Get("preset"); presetTag != "" {
			info := DescribeStrategy(childStrategy)

			presetValues, found := info.Suggestions[presetTag]
			if !found {
				return fmt.Errorf("discoverChildren: field %s: preset %q not found on strategy %s",
					field.Name, presetTag, childStrategy.Name())
			}

			for paramName, paramValue := range presetValues {
				if err := applyParamValue(childStrategy, paramName, paramValue); err != nil {
					return fmt.Errorf("discoverChildren: field %s: applying preset %q param %q: %w",
						field.Name, presetTag, paramName, err)
				}
			}
		}

		// Apply params tag overrides (space-separated key=value pairs).
		if paramsTag := field.Tag.Get("params"); paramsTag != "" {
			pairs := strings.Fields(paramsTag)
			for _, pair := range pairs {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("discoverChildren: field %s: malformed param %q (expected key=value)",
						field.Name, pair)
				}

				if err := applyParamValue(childStrategy, parts[0], parts[1]); err != nil {
					return fmt.Errorf("discoverChildren: field %s: applying param %q: %w",
						field.Name, pair, err)
				}
			}
		}

		entry := &childEntry{
			strategy: childStrategy,
			name:     childName,
			weight:   parsedWeight,
		}

		eng.children = append(eng.children, entry)
		eng.childrenByName[childName] = entry

		// Recurse for meta-strategies that have their own weighted children.
		if err := eng.discoverChildren(childStrategy, visited); err != nil {
			return err
		}
	}

	// After processing all fields at this level, validate weight sum
	// (only at the top-level call -- when parentStrategy is the engine's root strategy).
	if reflect.ValueOf(parentStrategy).Pointer() == reflect.ValueOf(eng.strategy).Pointer() {
		var totalWeight float64
		for _, child := range eng.children {
			totalWeight += child.weight
		}

		if totalWeight > 1.0 {
			return fmt.Errorf("discoverChildren: total child weight %.4f exceeds 1.0", totalWeight)
		}
	}

	return nil
}

// applyParamValue sets a field on the target strategy struct to the given
// string value. Field matching is by pvbt tag name or lowercased field name.
// Supported types: string, int, float64, bool, time.Duration.
func applyParamValue(target Strategy, paramName string, rawValue string) error {
	val := reflect.ValueOf(target)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	targetType := val.Type()
	if targetType.Kind() != reflect.Struct {
		return fmt.Errorf("target is not a struct")
	}

	for fieldIdx := 0; fieldIdx < targetType.NumField(); fieldIdx++ {
		field := targetType.Field(fieldIdx)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = strings.ToLower(field.Name)
		}

		if name != paramName {
			continue
		}

		fieldValue := val.Field(fieldIdx)
		if !fieldValue.CanSet() {
			return fmt.Errorf("field %s is not settable", field.Name)
		}

		switch field.Type {
		case durationType:
			parsed, err := time.ParseDuration(rawValue)
			if err != nil {
				return fmt.Errorf("parsing duration %q for field %s: %w", rawValue, field.Name, err)
			}

			fieldValue.Set(reflect.ValueOf(parsed))

		default:
			switch field.Type.Kind() {
			case reflect.String:
				fieldValue.SetString(rawValue)
			case reflect.Int:
				parsed, err := strconv.Atoi(rawValue)
				if err != nil {
					return fmt.Errorf("parsing int %q for field %s: %w", rawValue, field.Name, err)
				}

				fieldValue.SetInt(int64(parsed))
			case reflect.Float64:
				parsed, err := strconv.ParseFloat(rawValue, 64)
				if err != nil {
					return fmt.Errorf("parsing float64 %q for field %s: %w", rawValue, field.Name, err)
				}

				fieldValue.SetFloat(parsed)
			case reflect.Bool:
				parsed, err := strconv.ParseBool(rawValue)
				if err != nil {
					return fmt.Errorf("parsing bool %q for field %s: %w", rawValue, field.Name, err)
				}

				fieldValue.SetBool(parsed)
			default:
				// Skip types handled by hydrateFields later (asset.Asset, universe.Universe).
				return nil
			}
		}

		return nil
	}

	return fmt.Errorf("no field matching param %q on %s", paramName, targetType.Name())
}
