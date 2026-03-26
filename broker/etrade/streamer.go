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
// INDIVIDUAL_FILLS status. Duplicate fills are suppressed via seenFills.
type orderPoller struct {
	client       *apiClient
	fills        chan broker.Fill
	seenFills    map[string]bool // key: "orderID-qty-price"
	mu           sync.Mutex
	cancel       context.CancelFunc
	pollInterval time.Duration
}

// newOrderPoller creates an orderPoller with the default poll interval.
func newOrderPoller(client *apiClient, fills chan broker.Fill) *orderPoller {
	return &orderPoller{
		client:       client,
		fills:        fills,
		seenFills:    make(map[string]bool),
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
		fillKey := fmt.Sprintf("%d-%.4f-%.4f", order.OrderID, instr.FilledQty, instr.AveragePrice)

		op.mu.Lock()
		alreadySeen := op.seenFills[fillKey]

		if !alreadySeen {
			op.seenFills[fillKey] = true
		}

		op.mu.Unlock()

		if alreadySeen {
			continue
		}

		fill := broker.Fill{
			OrderID:  fmt.Sprintf("%d", order.OrderID),
			Price:    instr.AveragePrice,
			Qty:      instr.FilledQty,
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
