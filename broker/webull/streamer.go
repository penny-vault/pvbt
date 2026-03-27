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

package webull

import (
	"context"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	maxReconnectBackoff = 30 * time.Second
	initialBackoff      = 1 * time.Second
)

type fillStreamer struct {
	fills       chan broker.Fill
	done        chan struct{}
	wg          sync.WaitGroup
	mu          sync.Mutex
	cumulFilled map[string]float64 // order ID -> cumulative filled qty

	// pollOrders retrieves current orders for reconciliation on reconnect.
	pollOrders func(ctx context.Context) ([]orderResponse, error)

	// gRPC connection fields
	grpcTarget string
	sign       signer
	accountID  string
}

// run starts the gRPC trade event stream with reconnect logic. The actual
// gRPC connection is not yet implemented; this method is a placeholder that
// will be wired up in Task 8. It references handleTradeEvent and
// pollMissedFills so the fill-processing logic is reachable from non-test code.
func (fs *fillStreamer) run(ctx context.Context) {
	defer fs.wg.Done()

	backoff := initialBackoff

	for {
		select {
		case <-fs.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		// On (re)connect, poll for any fills missed while disconnected.
		fs.pollMissedFills(ctx)

		// TODO(task-8): open gRPC stream and dispatch events via
		// fs.handleTradeEvent(orderID, status, filledQty, filledPrice)
		// On successful stream connection, reset backoff:
		//   backoff = initialBackoff

		log.Info().
			Str("target", fs.grpcTarget).
			Str("account_id", fs.accountID).
			Msg("webull: gRPC stream not yet connected, waiting before retry")

		select {
		case <-fs.done:
			return
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if backoff < maxReconnectBackoff {
			backoff *= 2
		}
	}
}

// handleTradeEvent processes a trade event. Sends a broker.Fill when the
// cumulative filled quantity increases.
func (fs *fillStreamer) handleTradeEvent(orderID, status string, filledQty, filledPrice float64) {
	if status != "FILLED" && status != "FINAL_FILLED" {
		switch status {
		case "PLACE_FAILED", "CANCEL_FAILED", "MODIFY_FAILED":
			log.Error().Str("order_id", orderID).Str("status", status).Msg("webull: trade event")
		case "CANCEL_SUCCESS", "MODIFY_SUCCESS":
			log.Info().Str("order_id", orderID).Str("status", status).Msg("webull: trade event")
		default:
			log.Info().Str("order_id", orderID).Str("status", status).Msg("webull: trade event")
		}

		return
	}

	fs.mu.Lock()
	prev := fs.cumulFilled[orderID]

	if filledQty <= prev {
		fs.mu.Unlock()
		return
	}

	delta := filledQty - prev
	fs.cumulFilled[orderID] = filledQty
	fs.mu.Unlock()

	fill := broker.Fill{
		OrderID:  orderID,
		Price:    filledPrice,
		Qty:      delta,
		FilledAt: time.Now(),
	}

	select {
	case fs.fills <- fill:
	case <-fs.done:
	}
}

// pollMissedFills queries orders and sends fills not yet delivered.
func (fs *fillStreamer) pollMissedFills(ctx context.Context) {
	if fs.pollOrders == nil {
		return
	}

	orders, pollErr := fs.pollOrders(ctx)
	if pollErr != nil {
		log.Error().Err(pollErr).Msg("webull: poll missed fills failed")
		return
	}

	for _, order := range orders {
		if order.Status != "FILLED" && order.Status != "PARTIALLY_FILLED" {
			continue
		}

		filledQty := parseFloat(order.FilledQty)
		filledPrice := parseFloat(order.FilledPrice)

		fs.handleTradeEvent(order.ID, "FILLED", filledQty, filledPrice)
	}
}

// close signals the background goroutine to exit and waits.
func (fs *fillStreamer) close() error {
	close(fs.done)
	fs.wg.Wait()

	return nil
}
