package schwab

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bytedance/sonic"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	maxReconnectAttempts = 3
	pruneThreshold       = 24 * time.Hour
	stalenessTimeout     = 60 * time.Second
)

type wsMessage struct {
	data []byte
	err  error
}

type activityStreamer struct {
	client       *apiClient
	fills        chan broker.Fill
	wsURL        string
	wsConn       *websocket.Conn
	streamerInfo schwabStreamerInfo
	accountHash  string
	tokenFunc    func() string
	seenFills    map[string]time.Time
	mu           sync.Mutex
	done         chan struct{}
	wg           sync.WaitGroup
	ctx          context.Context
	lastPruneDay time.Time
	requestID    int
}

// streamerLoginResponse represents the LOGIN response from the streamer.
type streamerLoginResponse struct {
	Response []struct {
		Service string `json:"service"`
		Command string `json:"command"`
		Content struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"content"`
	} `json:"response"`
}

// streamerDataMessage represents an incoming data message from the streamer.
type streamerDataMessage struct {
	Data []struct {
		Service   string `json:"service"`
		Timestamp int64  `json:"timestamp"`
		Content   []struct {
			MessageType string          `json:"1"`
			AccountNum  string          `json:"2"`
			MessageData json.RawMessage `json:"3"`
		} `json:"content"`
	} `json:"data"`
	Notify []struct {
		Heartbeat string `json:"heartbeat"`
	} `json:"notify"`
}

func newActivityStreamer(client *apiClient, fills chan broker.Fill, wsURL string, info schwabStreamerInfo, accountHash string, tokenFunc func() string) *activityStreamer {
	return &activityStreamer{
		client:       client,
		fills:        fills,
		wsURL:        wsURL,
		streamerInfo: info,
		accountHash:  accountHash,
		tokenFunc:    tokenFunc,
		seenFills:    make(map[string]time.Time),
		done:         make(chan struct{}),
	}
}

func (streamer *activityStreamer) connect(ctx context.Context) error {
	streamer.ctx = ctx
	streamer.requestID = 0

	conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, streamer.wsURL, nil)
	if dialErr != nil {
		return fmt.Errorf("activity streamer connect: %w", dialErr)
	}

	streamer.mu.Lock()
	streamer.wsConn = conn
	streamer.mu.Unlock()

	if loginErr := streamer.sendLogin(conn); loginErr != nil {
		conn.Close()
		return loginErr
	}

	if subsErr := streamer.sendSubs(conn); subsErr != nil {
		conn.Close()
		return subsErr
	}

	streamer.wg.Add(1)

	go streamer.run()

	return nil
}

func (streamer *activityStreamer) sendLogin(conn *websocket.Conn) error {
	streamer.requestID++

	loginMsg := map[string]any{
		"requests": []map[string]any{
			{
				"requestid":              fmt.Sprintf("%d", streamer.requestID),
				"service":                "ADMIN",
				"command":                "LOGIN",
				"SchwabClientCustomerId": streamer.streamerInfo.SchwabClientCustomerID,
				"SchwabClientCorrelId":   streamer.streamerInfo.SchwabClientCorrelID,
				"parameters": map[string]any{
					"Authorization":          streamer.tokenFunc(),
					"SchwabClientChannel":    streamer.streamerInfo.SchwabClientChannel,
					"SchwabClientFunctionId": streamer.streamerInfo.SchwabClientFunctionID,
				},
			},
		},
	}

	if writeErr := conn.WriteJSON(loginMsg); writeErr != nil {
		return fmt.Errorf("send LOGIN: %w", writeErr)
	}

	// Read LOGIN response.
	_, respData, readErr := conn.ReadMessage()
	if readErr != nil {
		return fmt.Errorf("read LOGIN response: %w", readErr)
	}

	var loginResp streamerLoginResponse
	if unmarshalErr := sonic.Unmarshal(respData, &loginResp); unmarshalErr != nil {
		return fmt.Errorf("parse LOGIN response: %w", unmarshalErr)
	}

	if len(loginResp.Response) > 0 && loginResp.Response[0].Content.Code != 0 {
		return ErrLoginDenied
	}

	return nil
}

func (streamer *activityStreamer) sendSubs(conn *websocket.Conn) error {
	streamer.requestID++

	subsMsg := map[string]any{
		"requests": []map[string]any{
			{
				"requestid":              fmt.Sprintf("%d", streamer.requestID),
				"service":                "ACCT_ACTIVITY",
				"command":                "SUBS",
				"SchwabClientCustomerId": streamer.streamerInfo.SchwabClientCustomerID,
				"SchwabClientCorrelId":   streamer.streamerInfo.SchwabClientCorrelID,
				"parameters": map[string]any{
					"keys":   streamer.accountHash,
					"fields": "0,1,2,3",
				},
			},
		},
	}

	return conn.WriteJSON(subsMsg)
}

func (streamer *activityStreamer) close() error {
	close(streamer.done)

	streamer.mu.Lock()
	conn := streamer.wsConn
	streamer.mu.Unlock()

	if conn != nil {
		conn.Close()
	}

	streamer.wg.Wait()

	return nil
}

func (streamer *activityStreamer) readPump(conn *websocket.Conn, messages chan<- wsMessage) {
	for {
		_, data, readErr := conn.ReadMessage()
		if readErr != nil {
			messages <- wsMessage{err: readErr}
			return
		}

		messages <- wsMessage{data: data}
	}
}

func (streamer *activityStreamer) run() {
	defer streamer.wg.Done()

	messages := make(chan wsMessage, 16)

	stalenessTimer := time.NewTimer(stalenessTimeout)
	defer stalenessTimer.Stop()

	streamer.mu.Lock()
	conn := streamer.wsConn
	streamer.mu.Unlock()

	go streamer.readPump(conn, messages)

	for {
		select {
		case <-streamer.done:
			return
		case <-streamer.ctx.Done():
			return
		case <-stalenessTimer.C:
			// No message received within timeout, attempt reconnection.
			select {
			case <-streamer.done:
				return
			case <-streamer.ctx.Done():
				return
			default:
			}

			if reconnectErr := streamer.reconnect(streamer.ctx); reconnectErr != nil {
				return
			}

			streamer.mu.Lock()
			conn = streamer.wsConn
			streamer.mu.Unlock()

			go streamer.readPump(conn, messages)

			stalenessTimer.Reset(stalenessTimeout)
		case msg := <-messages:
			stalenessTimer.Reset(stalenessTimeout)

			if msg.err != nil {
				select {
				case <-streamer.done:
					return
				case <-streamer.ctx.Done():
					return
				default:
				}

				if reconnectErr := streamer.reconnect(streamer.ctx); reconnectErr != nil {
					return
				}

				streamer.mu.Lock()
				conn = streamer.wsConn
				streamer.mu.Unlock()

				go streamer.readPump(conn, messages)

				continue
			}

			streamer.pruneSeenFills()
			streamer.handleMessage(msg.data)
		}
	}
}

func (streamer *activityStreamer) handleMessage(data []byte) {
	var msg streamerDataMessage
	if unmarshalErr := sonic.Unmarshal(data, &msg); unmarshalErr != nil {
		return
	}

	for _, dataEntry := range msg.Data {
		if dataEntry.Service != "ACCT_ACTIVITY" {
			continue
		}

		for _, content := range dataEntry.Content {
			if content.MessageType != "OrderFill" {
				continue
			}

			var orderData schwabOrderResponse
			if unmarshalErr := sonic.Unmarshal(content.MessageData, &orderData); unmarshalErr != nil {
				continue
			}

			orderID := fmt.Sprintf("%d", orderData.OrderID)

			for _, activity := range orderData.OrderActivityCollection {
				if activity.ActivityType != "EXECUTION" {
					continue
				}

				for _, execLeg := range activity.ExecutionLegs {
					fillKey := fmt.Sprintf("%s-%s-%.2f", orderID, execLeg.Time, execLeg.Price)

					streamer.mu.Lock()

					_, alreadySeen := streamer.seenFills[fillKey]
					if !alreadySeen {
						streamer.seenFills[fillKey] = time.Now()
					}

					streamer.mu.Unlock()

					if alreadySeen {
						continue
					}

					filledAt, parseErr := time.Parse("2006-01-02T15:04:05+0000", execLeg.Time)
					if parseErr != nil {
						log.Warn().Err(parseErr).Str("timestamp", execLeg.Time).Msg("schwab: could not parse fill timestamp, using current time")

						filledAt = time.Now()
					}

					fill := broker.Fill{
						OrderID:  orderID,
						Price:    execLeg.Price,
						Qty:      execLeg.Quantity,
						FilledAt: filledAt,
					}

					select {
					case streamer.fills <- fill:
					case <-streamer.done:
						return
					case <-streamer.ctx.Done():
						return
					}
				}
			}
		}
	}
}

func (streamer *activityStreamer) reconnect(ctx context.Context) error {
	streamer.mu.Lock()
	if streamer.wsConn != nil {
		streamer.wsConn.Close()
		streamer.wsConn = nil
	}
	streamer.mu.Unlock()

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
		}

		conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, streamer.wsURL, nil)
		if dialErr != nil {
			continue
		}

		if loginErr := streamer.sendLogin(conn); loginErr != nil {
			conn.Close()
			continue
		}

		if subsErr := streamer.sendSubs(conn); subsErr != nil {
			conn.Close()
			continue
		}

		streamer.mu.Lock()
		streamer.wsConn = conn
		streamer.mu.Unlock()

		streamer.pollMissedFills(ctx)

		return nil
	}

	return ErrStreamDisconnected
}

func (streamer *activityStreamer) pollMissedFills(ctx context.Context) {
	orders, fetchErr := streamer.client.getOrders(ctx)
	if fetchErr != nil {
		return
	}

	for _, order := range orders {
		if order.Status != "FILLED" {
			continue
		}

		orderID := fmt.Sprintf("%d", order.OrderID)

		for _, activity := range order.OrderActivityCollection {
			if activity.ActivityType != "EXECUTION" {
				continue
			}

			for _, execLeg := range activity.ExecutionLegs {
				fillKey := fmt.Sprintf("%s-%s-%.2f", orderID, execLeg.Time, execLeg.Price)

				streamer.mu.Lock()

				_, alreadySeen := streamer.seenFills[fillKey]
				if !alreadySeen {
					streamer.seenFills[fillKey] = time.Now()
				}

				streamer.mu.Unlock()

				if alreadySeen {
					continue
				}

				filledAt, parseErr := time.Parse("2006-01-02T15:04:05+0000", execLeg.Time)
				if parseErr != nil {
					log.Warn().Err(parseErr).Str("timestamp", execLeg.Time).Msg("schwab: could not parse fill timestamp, using current time")

					filledAt = time.Now()
				}

				fill := broker.Fill{
					OrderID:  orderID,
					Price:    execLeg.Price,
					Qty:      execLeg.Quantity,
					FilledAt: filledAt,
				}

				select {
				case streamer.fills <- fill:
				case <-streamer.done:
					return
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (streamer *activityStreamer) pruneSeenFills() {
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
