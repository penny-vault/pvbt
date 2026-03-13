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

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/universe"
)

var (
	assetType    = reflect.TypeOf(asset.Asset{})
	universeType = reflect.TypeOf((*universe.Universe)(nil)).Elem()
	durationType = reflect.TypeOf(time.Duration(0))
)

// hydrateFields reflects over the target struct and populates exported fields
// from their `default` tags. Fields with non-zero values are not overwritten.
// asset.Asset fields are resolved via the engine's asset registry.
// universe.Universe fields are built from comma-separated tickers via e.Universe.
func hydrateFields(e *Engine, target interface{}) error {
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return nil
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		defaultVal := field.Tag.Get("default")
		if defaultVal == "" {
			continue
		}

		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}

		// Skip non-zero fields (caller may have pre-set them).
		if !fv.IsZero() {
			continue
		}

		switch {
		case field.Type == assetType:
			a := e.Asset(defaultVal)
			fv.Set(reflect.ValueOf(a))

		case field.Type.Implements(universeType):
			tickers := strings.Split(defaultVal, ",")
			assets := make([]asset.Asset, len(tickers))
			for j, ticker := range tickers {
				assets[j] = e.Asset(strings.TrimSpace(ticker))
			}
			u := e.Universe(assets...)
			fv.Set(reflect.ValueOf(u))

		case field.Type == durationType:
			d, err := time.ParseDuration(defaultVal)
			if err != nil {
				return fmt.Errorf("hydrate %s.%s: parsing duration %q: %w", t.Name(), field.Name, defaultVal, err)
			}
			fv.Set(reflect.ValueOf(d))

		default:
			switch field.Type.Kind() {
			case reflect.Float64:
				f, err := strconv.ParseFloat(defaultVal, 64)
				if err != nil {
					return fmt.Errorf("hydrate %s.%s: parsing float64 %q: %w", t.Name(), field.Name, defaultVal, err)
				}
				fv.SetFloat(f)
			case reflect.Int:
				n, err := strconv.Atoi(defaultVal)
				if err != nil {
					return fmt.Errorf("hydrate %s.%s: parsing int %q: %w", t.Name(), field.Name, defaultVal, err)
				}
				fv.SetInt(int64(n))
			case reflect.String:
				fv.SetString(defaultVal)
			case reflect.Bool:
				b, err := strconv.ParseBool(defaultVal)
				if err != nil {
					return fmt.Errorf("hydrate %s.%s: parsing bool %q: %w", t.Name(), field.Name, defaultVal, err)
				}
				fv.SetBool(b)
			}
		}
	}
	return nil
}
