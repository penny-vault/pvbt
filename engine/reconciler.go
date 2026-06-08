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
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/portfolio"
)

// Reconciler is an optional interface a strategy can implement to react to how
// its batch resolved against the market. After every order in a batch has
// resolved -- filled or failed -- the engine calls Reconcile with the same
// batch, now carrying each order's outcome (read via batch.Outcomes or
// batch.FailedOrders). The strategy may amend the batch in place, for example
// resubmitting a failed order as a market order or simply logging the miss.
//
// Any orders the strategy appends are executed, and Reconcile is then called
// again with their outcomes. This repeats until a pass appends no new orders,
// so the strategy can drive a batch to completion. Strategies that do not
// implement Reconciler see failed orders silently not fill, as before.
type Reconciler interface {
	Reconcile(ctx context.Context, eng *Engine, port portfolio.Portfolio, batch *portfolio.Batch) error
}

// maxReconcilePasses bounds how many times the engine will call Reconcile for a
// single batch. A well-behaved strategy settles in one or two passes (e.g.
// resubmit a failed limit as market). The cap exists only to stop a strategy
// that appends orders unconditionally from looping forever; exceeding it is a
// strategy bug and surfaces as an error rather than silently truncating.
const maxReconcilePasses = 16

// reconcileBatch runs the reconcile loop for a strategy that implements
// Reconciler. It is a no-op for strategies that do not. Each pass calls
// Reconcile, then executes any orders the strategy appended; the loop ends when
// a pass appends nothing (the batch has settled).
func (e *Engine) reconcileBatch(
	ctx context.Context,
	strategy Strategy,
	acct portfolio.PortfolioManager,
	batch *portfolio.Batch,
) error {
	reconciler, ok := strategy.(Reconciler)
	if !ok {
		return nil
	}

	for range maxReconcilePasses {
		before := len(batch.Orders)

		if err := reconciler.Reconcile(ctx, e, acct, batch); err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}

		// No new orders means the strategy is satisfied: the batch is settled.
		if len(batch.Orders) == before {
			return nil
		}

		if err := e.prefetchBrokerPrices(ctx, batch.Orders[before:]); err != nil {
			return fmt.Errorf("reconcile prefetch broker prices: %w", err)
		}

		if err := acct.ExecuteBatch(ctx, batch); err != nil {
			return fmt.Errorf("reconcile execute batch: %w", err)
		}
	}

	return fmt.Errorf("reconcile: batch did not settle after %d passes", maxReconcilePasses)
}
