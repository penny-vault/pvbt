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

package portfolio

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// mapToLogical converts a real asset to its logical original if an active
// substitution exists. If no substitution is active, returns the asset unchanged.
func mapToLogical(realAsset asset.Asset, subs map[asset.Asset]Substitution, asOf time.Time) asset.Asset {
	for _, sub := range subs {
		if sub.Substitute == realAsset && asOf.Before(sub.Until) {
			return sub.Original
		}
	}

	return realAsset
}
