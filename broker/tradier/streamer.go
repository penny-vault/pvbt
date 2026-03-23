package tradier

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

// activityStreamer connects to Tradier's account events WebSocket and delivers
// fills to the TradierBroker. Full implementation is in progress.
type activityStreamer struct {
	client *apiClient
	fills  chan broker.Fill
	wsURL  string
}

func newActivityStreamer(client *apiClient, fills chan broker.Fill) *activityStreamer {
	return &activityStreamer{
		client: client,
		fills:  fills,
	}
}

func (streamer *activityStreamer) connect(ctx context.Context) error {
	sessionID, sessionErr := streamer.client.createStreamSession(ctx)
	if sessionErr != nil {
		return sessionErr
	}

	streamer.wsURL = sessionID

	return nil
}

func (streamer *activityStreamer) listen(ctx context.Context, messages <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-messages:
			if !ok {
				return
			}

			if eventErr := streamer.processEvent(ctx, data); eventErr != nil {
				log.Error().Err(eventErr).Msg("tradier: process account event")
			}
		}
	}
}

func (streamer *activityStreamer) processEvent(ctx context.Context, data []byte) error {
	var ev tradierAccountEvent
	if unmarshalErr := json.Unmarshal(data, &ev); unmarshalErr != nil {
		return unmarshalErr
	}

	if ev.Event != "order" {
		return nil
	}

	orderID := fmt.Sprintf("%d", ev.ID)

	order, orderErr := streamer.client.getOrder(ctx, orderID)
	if orderErr != nil {
		return orderErr
	}

	if order.Status == "filled" || order.Status == "partially_filled" {
		streamer.fills <- broker.Fill{
			OrderID: orderID,
			Price:   order.AvgFillPrice,
			Qty:     order.ExecQuantity,
		}
	}

	return nil
}
