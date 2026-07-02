package etrade

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const defaultPollInterval = 2 * time.Second

// orderPoller periodically polls the E*TRADE orders endpoint and emits fills
// onto the fills channel when an order transitions to EXECUTED or
// INDIVIDUAL_FILLS status. E*TRADE reports the cumulative filled quantity per
// order, so the poller tracks a running total per order and emits only the
// delta on each change; this also suppresses duplicates across polls.
type orderPoller struct {
	client       *apiClient
	fills        chan broker.Fill
	cumFilled    map[string]float64 // key: orderID, value: cumulative filled qty
	mu           sync.Mutex
	cancel       context.CancelFunc
	pollInterval time.Duration
}

// newOrderPoller creates an orderPoller with the default poll interval.
func newOrderPoller(client *apiClient, fills chan broker.Fill) *orderPoller {
	return &orderPoller{
		client:       client,
		fills:        fills,
		cumFilled:    make(map[string]float64),
		pollInterval: defaultPollInterval,
	}
}

// start launches a background goroutine that calls poll on every tick until
// the supplied context is cancelled or stop is called.
func (op *orderPoller) start(ctx context.Context) {
	child, cancel := context.WithCancel(ctx)
	op.cancel = cancel

	go func() {
		ticker := time.NewTicker(op.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-child.Done():
				return
			case <-ticker.C:
				if pollErr := op.poll(child); pollErr != nil {
					log.Warn().Err(pollErr).Msg("etrade: order poller: poll failed")
				}
			}
		}
	}()
}

// stop cancels the background goroutine started by start.
func (op *orderPoller) stop() {
	if op.cancel != nil {
		op.cancel()
	}
}

// poll fetches current orders once and emits any new fills found.
func (op *orderPoller) poll(ctx context.Context) error {
	orders, fetchErr := op.client.getOrders(ctx)
	if fetchErr != nil {
		return fmt.Errorf("etrade: order poller: poll: %w", fetchErr)
	}

	for _, order := range orders {
		if order.Status != "EXECUTED" && order.Status != "INDIVIDUAL_FILLS" {
			continue
		}

		if len(order.OrderList) == 0 {
			continue
		}

		leg := order.OrderList[0]

		if len(leg.Instrument) == 0 {
			continue
		}

		instr := leg.Instrument[0]
		orderID := fmt.Sprintf("%d", order.OrderID)

		op.mu.Lock()

		delta := instr.FilledQty - op.cumFilled[orderID]
		if delta > 0 {
			op.cumFilled[orderID] = instr.FilledQty
		}

		op.mu.Unlock()

		if delta <= 0 {
			continue
		}

		fill := broker.Fill{
			OrderID:  orderID,
			Price:    instr.AveragePrice,
			Qty:      delta,
			FilledAt: time.Now(),
		}

		select {
		case op.fills <- fill:
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}
