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

	"github.com/penny-vault/pvbt/data"
)

// Option configures the engine.
type Option func(*Engine)

// WithDataProvider registers one or more data providers with the engine.
func WithDataProvider(providers ...data.DataProvider) Option {
	return func(e *Engine) {
		e.providers = append(e.providers, providers...)
	}
}

// WithAssetProvider sets the asset provider for ticker resolution.
func WithAssetProvider(p data.AssetProvider) Option {
	return func(e *Engine) {
		e.assetProvider = p
	}
}

// WithCacheMaxBytes sets the maximum memory for the data cache.
// Default is 512MB.
func WithCacheMaxBytes(n int64) Option {
	return func(e *Engine) {
		e.cacheMaxBytes = n
	}
}

// WithChunkSize sets the time duration of each data cache chunk.
// Default is 1 year.
func WithChunkSize(d time.Duration) Option {
	return func(e *Engine) {
		e.cacheChunkSize = d
	}
}
