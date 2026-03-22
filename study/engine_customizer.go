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

package study

import "github.com/penny-vault/pvbt/engine"

// EngineCustomizer is an optional interface that a Study can implement to
// customize per-run engine construction. When the runner detects that a study
// implements this interface, it calls EngineOptions for each run and appends
// the returned options to the base options before constructing the engine.
type EngineCustomizer interface {
	EngineOptions(cfg RunConfig) []engine.Option
}
