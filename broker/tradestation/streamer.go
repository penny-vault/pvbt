package tradestation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	maxReconnectAttempts = 3
	maxBackoff           = 30 * time.Second
	pruneThreshold       = 24 * time.Hour
)

type orderStreamer struct {
	client       *apiClient
	fills        chan broker.Fill
	baseURL      string
	accountID    string
	tokenFunc    func() string
	seenFills    map[string]time.Time
	mu           sync.Mutex
	done         chan struct{}
	wg           sync.WaitGroup
	cancelStream context.CancelFunc
	lastPruneDay time.Time
}

func newOrderStreamer(client *apiClient, fills chan broker.Fill, baseURL string, accountID string, tokenFunc func() string) *orderStreamer {
	return &orderStreamer{
		client:    client,
		fills:     fills,
		baseURL:   baseURL,
		accountID: accountID,
		tokenFunc: tokenFunc,
		seenFills: make(map[string]time.Time),
		done:      make(chan struct{}),
	}
}

func (streamer *orderStreamer) connect(ctx context.Context) error {
	streamCtx, cancel := context.WithCancel(ctx)
	streamer.cancelStream = cancel

	resp, connectErr := streamer.openStream(streamCtx)
	if connectErr != nil {
		cancel()
		return fmt.Errorf("order streamer connect: %w", connectErr)
	}

	streamer.wg.Add(1)

	go streamer.run(streamCtx, resp)

	return nil
}

func (streamer *orderStreamer) openStream(ctx context.Context) (*http.Response, error) {
	streamURL := fmt.Sprintf("%s/v3/brokerage/stream/accounts/%s/orders", streamer.baseURL, streamer.accountID)

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if reqErr != nil {
		return nil, fmt.Errorf("create stream request: %w", reqErr)
	}

	req.Header.Set("Authorization", "Bearer "+streamer.tokenFunc())
	req.Header.Set("Accept", "application/vnd.tradestation.streams.v3+json")

	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		return nil, fmt.Errorf("open stream: %w", doErr)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, broker.NewHTTPError(resp.StatusCode, "stream connection failed")
	}

	return resp, nil
}

func (streamer *orderStreamer) close() error {
	close(streamer.done)

	if streamer.cancelStream != nil {
		streamer.cancelStream()
	}

	streamer.wg.Wait()

	return nil
}

func (streamer *orderStreamer) run(ctx context.Context, resp *http.Response) {
	defer streamer.wg.Done()
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)

	for {
		select {
		case <-streamer.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		var event tsStreamOrderEvent
		if decodeErr := decoder.Decode(&event); decodeErr != nil {
			select {
			case <-streamer.done:
				return
			case <-ctx.Done():
				return
			default:
			}

			if reconnectErr := streamer.reconnect(ctx); reconnectErr != nil {
				return
			}

			// Re-open the stream and restart the decoder.
			newResp, openErr := streamer.openStream(ctx)
			if openErr != nil {
				return
			}

			resp.Body.Close()
			resp = newResp
			decoder = json.NewDecoder(resp.Body)

			continue
		}

		if event.GoAway {
			resp.Body.Close()

			newResp, openErr := streamer.reconnectStream(ctx)
			if openErr != nil {
				return
			}

			resp = newResp
			decoder = json.NewDecoder(resp.Body)

			continue
		}

		if event.Error != "" {
			log.Warn().Str("error", event.Error).Msg("tradestation: stream error")
			continue
		}

		if event.EndSnapshot || event.Heartbeat != 0 {
			continue
		}

		streamer.pruneSeenFills()
		streamer.handleEvent(event)
	}
}

func (streamer *orderStreamer) handleEvent(event tsStreamOrderEvent) {
	if event.Status != "FLL" && event.Status != "FLP" {
		return
	}

	for _, leg := range event.Legs {
		for _, fill := range leg.Fills {
			fillKey := fmt.Sprintf("%s-%s", event.OrderID, fill.ExecID)

			streamer.mu.Lock()

			_, alreadySeen := streamer.seenFills[fillKey]
			if !alreadySeen {
				streamer.seenFills[fillKey] = time.Now()
			}

			streamer.mu.Unlock()

			if alreadySeen {
				continue
			}

			filledAt, parseErr := time.Parse(time.RFC3339, fill.Timestamp)
			if parseErr != nil {
				log.Warn().Err(parseErr).Str("timestamp", fill.Timestamp).Msg("tradestation: could not parse fill timestamp, using current time")

				filledAt = time.Now()
			}

			brokerFill := broker.Fill{
				OrderID:  event.OrderID,
				Price:    parseFloat(fill.Price),
				Qty:      parseFloat(fill.Quantity),
				FilledAt: filledAt,
			}

			select {
			case streamer.fills <- brokerFill:
			case <-streamer.done:
				return
			}
		}
	}
}

func (streamer *orderStreamer) reconnect(ctx context.Context) error {
	backoff := 1 * time.Second

	for attempt := range maxReconnectAttempts {
		select {
		case <-streamer.done:
			return ErrStreamDisconnected
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-streamer.done:
				timer.Stop()
				return ErrStreamDisconnected
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	streamer.pollMissedFills(ctx)

	return nil
}

func (streamer *orderStreamer) reconnectStream(ctx context.Context) (*http.Response, error) {
	backoff := 1 * time.Second

	for attempt := range maxReconnectAttempts {
		select {
		case <-streamer.done:
			return nil, ErrStreamDisconnected
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-streamer.done:
				timer.Stop()
				return nil, ErrStreamDisconnected
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		resp, openErr := streamer.openStream(ctx)
		if openErr != nil {
			continue
		}

		streamer.pollMissedFills(ctx)

		return resp, nil
	}

	return nil, ErrStreamDisconnected
}

func (streamer *orderStreamer) pollMissedFills(ctx context.Context) {
	orders, fetchErr := streamer.client.getOrders(ctx)
	if fetchErr != nil {
		return
	}

	for _, order := range orders {
		if order.Status != "FLL" && order.Status != "FLP" {
			continue
		}

		for _, leg := range order.Legs {
			for _, fill := range leg.Fills {
				fillKey := fmt.Sprintf("%s-%s", order.OrderID, fill.ExecID)

				streamer.mu.Lock()

				_, alreadySeen := streamer.seenFills[fillKey]
				if !alreadySeen {
					streamer.seenFills[fillKey] = time.Now()
				}

				streamer.mu.Unlock()

				if alreadySeen {
					continue
				}

				filledAt, parseErr := time.Parse(time.RFC3339, fill.Timestamp)
				if parseErr != nil {
					filledAt = time.Now()
				}

				brokerFill := broker.Fill{
					OrderID:  order.OrderID,
					Price:    parseFloat(fill.Price),
					Qty:      parseFloat(fill.Quantity),
					FilledAt: filledAt,
				}

				select {
				case streamer.fills <- brokerFill:
				case <-streamer.done:
					return
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (streamer *orderStreamer) pruneSeenFills() {
	today := time.Now().Truncate(pruneThreshold)

	streamer.mu.Lock()
	defer streamer.mu.Unlock()

	if today.Equal(streamer.lastPruneDay) {
		return
	}

	streamer.lastPruneDay = today
	cutoff := time.Now().Add(-pruneThreshold)

	for fillKey, seenAt := range streamer.seenFills {
		if seenAt.Before(cutoff) {
			delete(streamer.seenFills, fillKey)
		}
	}
}
