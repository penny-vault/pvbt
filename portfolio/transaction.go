// Copyright 2021-2023
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

import "github.com/penny-vault/pv-api/data"

// Security returns the security associated with the transaction
func (trx *Transaction) Security() *data.Security {
	return data.MustSecurityFromFigi(trx.CompositeFIGI)
}

// SplitFactor returns the split factor of the split transaction. If the transaction is not a Split transaction then returns 1.0
func (trx *Transaction) SplitFactor() float64 {
	if trx.Kind != SplitTransaction {
		return 1.0
	}
	for _, justification := range trx.Justification {
		if justification.Key == SplitFactor {
			return justification.Value
		}
	}
	return 1.0
}
