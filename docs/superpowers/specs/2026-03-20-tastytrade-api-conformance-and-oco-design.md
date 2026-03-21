# tastytrade API Conformance and OCO Integration Design

## Overview

Update the `broker/tastytrade` package to conform to the tastytrade developer API and implement the `broker.GroupSubmitter` interface for native OCO/OTOCO order support.

The tastytrade broker was built against an assumed API shape. Comparison against the [tastytrade developer docs](https://developer.tastytrade.com/) revealed several mismatches in endpoints, request fields, and streaming message formats. This spec fixes all conformance issues and adds `GroupSubmitter` support in a single pass since the changes are tightly coupled within one package.

## API Reference

All endpoints are relative to the base URL:
- Production: `https://api.tastyworks.com`
- Sandbox: `https://api.cert.tastyworks.com`

WebSocket URLs:
- Production: `wss://streamer.tastyworks.com`
- Sandbox: `wss://streamer.cert.tastyworks.com`

Authoritative docs: https://developer.tastytrade.com/

## Changes

### 1. Fix Quote Endpoint

**Problem:** `client.getQuote()` hits `GET /market-data/{symbol}/quotes`. The actual API uses `GET /market-data/by-type?equity={symbol}`.

**Fix:** Change the endpoint to `/market-data/by-type` with query parameter `equity={symbol}`. The response structure uses a `MarketData` items array with a `last` field for last traded price. The existing `quoteResponse`/`quoteItem` types match structurally (we use the `last` field), but the endpoint path and query parameter encoding must change.

**Current:**
```go
endpoint := fmt.Sprintf("/market-data/%s/quotes", symbol)
```

**Target:**
```go
endpoint := fmt.Sprintf("/market-data/by-type?equity=%s", url.QueryEscape(symbol))
```

### 2. Add `price-effect` to Order Requests

**Problem:** The tastytrade API requires a `price-effect` field (`"Credit"` or `"Debit"`) on order submissions. Our `orderRequest` omits it.

**Fix:** Add `PriceEffect string` field to `orderRequest`. Derive it in `toTastytradeOrder()`:

| broker.Side | price-effect |
|-------------|-------------|
| Buy | `"Debit"` |
| Sell | `"Credit"` |

```go
type orderRequest struct {
    TimeInForce string     `json:"time-in-force"`
    OrderType   string     `json:"order-type"`
    Price       float64    `json:"price,omitempty"`
    PriceEffect string     `json:"price-effect,omitempty"`
    StopTrigger float64    `json:"stop-trigger,omitempty"`
    Legs        []orderLeg `json:"legs"`
}
```

The `price-effect` field is omitted for Market orders (where no price is set) since the API does not require it in that case.

### 3. Add `automated-source` to Order Requests

**Problem:** The API has an `automated-source` boolean field. Since pvbt is algorithmic trading software, this should be set to `true`.

**Fix:** Add `AutomatedSource bool` to `orderRequest`. Always set to `true` in `toTastytradeOrder()`.

```go
type orderRequest struct {
    // ... existing fields ...
    AutomatedSource bool `json:"automated-source"`
}
```

### 4. Fix WebSocket Streaming Format

**Problem:** The streamer's `handleMessage()` expects raw `fillEvent` JSON. The actual API sends an envelope:

```json
{
  "type": "Order",
  "data": {
    "id": "123",
    "status": "Filled",
    "legs": [{
      "symbol": "AAPL",
      "fills": [{
        "fill-id": "F1",
        "fill-price": 150.25,
        "quantity": "50",
        "filled-at": "2026-03-20T14:30:00Z"
      }]
    }]
  },
  "timestamp": 1688595114405
}
```

Fills are nested inside `legs[].fills[]`, not delivered as standalone events.

**Fix:** Replace the `fillEvent` type with proper envelope types:

```go
// streamerMessage is the top-level WebSocket envelope.
type streamerMessage struct {
    Type      string          `json:"type"`
    Data      json.RawMessage `json:"data"`
    Timestamp int64           `json:"timestamp"`
}
```

The streamer reuses the `orderResponse` and `orderLegResponse` types (updated with nested fills in Section 6) to parse the notification data. This avoids duplicate type definitions -- both the REST `getOrders` response and the WebSocket notification use the same order structure per the tastytrade docs ("Streamer messages always contain a full object representation").

A helper function `parseLegFillQuantity(s string) float64` converts the string quantity field to float64. The `filled-at` field is an ISO 8601 timestamp string parsed with `time.Parse(time.RFC3339, ...)`.

Update `handleMessage()`:
1. Unmarshal into `streamerMessage`.
2. If `Type != "Order"`, ignore.
3. Unmarshal `Data` into `orderResponse`.
4. Iterate `Legs[].Fills[]`, deduplicate by `fill-id`, emit a `broker.Fill` for each unseen fill with `OrderID` set to the order's `ID`.

### 5. Add WebSocket Connect and Heartbeat

**Problem:** The streamer dials the WebSocket but never sends the required `connect` action. The API requires an authenticated connect message before it will send notifications. Heartbeats must be sent at 2s-60s intervals to keep the connection alive.

**Fix:**

After dialing, send:
```json
{
  "action": "connect",
  "value": ["ACCT-001"],
  "auth-token": "session-token-here"
}
```

The streamer needs the session token and account ID from the `apiClient`. Add unexported accessor methods to `apiClient`:

```go
func (client *apiClient) sessionToken() string {
    return client.resty.Token  // resty stores the auth token
}

func (client *apiClient) account() string {
    return client.accountID
}
```

Add a heartbeat ticker in `run()` that sends `{"action": "heartbeat"}` every 30 seconds. The ticker is stopped on shutdown.

**Reconnection:** The `reconnect()` method must also send the connect message after re-dialing, before calling `pollMissedFills`. The heartbeat ticker in `run()` continues across reconnections since it runs in the main loop -- no restart is needed. If a heartbeat write fails (connection broken), it triggers the normal reconnection flow via the readPump error path.

### 6. Fix `pollMissedFills` to Extract Fills from Order Legs

**Problem:** `pollMissedFills` uses `order.Price` and `order.ID` for the fill. The actual API returns fills nested in `legs[].fills[]` with individual fill IDs, prices, quantities, and timestamps.

**Fix:** Update `orderResponse` and `orderLegResponse` in `types.go` to include the nested fills structure. These types are shared by both REST responses and WebSocket notifications (see Section 4).

```go
type orderResponse struct {
    ID             string             `json:"id"`
    Status         string             `json:"status"`
    OrderType      string             `json:"order-type"`
    TimeInForce    string             `json:"time-in-force"`
    Price          float64            `json:"price"`
    StopTrigger    float64            `json:"stop-trigger"`
    ComplexOrderID string             `json:"complex-order-id"`
    Legs           []orderLegResponse `json:"legs"`
}

type orderLegResponse struct {
    Symbol         string             `json:"symbol"`
    InstrumentType string             `json:"instrument-type"`
    Action         string             `json:"action"`
    Quantity       float64            `json:"quantity"`
    Fills          []legFillResponse  `json:"fills"`
}

type legFillResponse struct {
    FillID   string  `json:"fill-id"`
    Price    float64 `json:"fill-price"`
    Quantity string  `json:"quantity"`
    FilledAt string  `json:"filled-at"`
}
```

The `Quantity` field in `legFillResponse` is a string per the API. Add a helper `parseLegFillQuantity(s string) float64` that calls `strconv.ParseFloat` and returns 0 on error. The `FilledAt` field is parsed with `time.Parse(time.RFC3339, ...)`.

Update `pollMissedFills` to iterate over `legs[].fills[]` for each order, deduplicate by `fill-id`, and emit individual `broker.Fill` entries with proper price, quantity, and timestamp.

### 7. Add Pagination to `getOrders`

**Problem:** The API returns paginated results (default 10 per page). Our code makes a single request and may miss orders.

**Fix:** Add pagination to `getOrders()`. Use query parameters `page-offset` and `per-page`. Request pages in a loop, incrementing `page-offset`, until the returned items count is less than `per-page`. Use `per-page=200` (the API maximum) to minimize round trips.

```go
func (client *apiClient) getOrders(ctx context.Context) ([]orderResponse, error) {
    var allOrders []orderResponse
    perPage := 200
    offset := 0

    for {
        endpoint := fmt.Sprintf("/accounts/%s/orders?per-page=%d&page-offset=%d",
            client.accountID, perPage, offset)

        var result ordersListResponse
        resp, err := client.resty.R().
            SetContext(ctx).
            SetResult(&result).
            Get(endpoint)
        if err != nil {
            return nil, fmt.Errorf("get orders: %w", err)
        }
        if resp.IsError() {
            return nil, NewHTTPError(resp.StatusCode(), resp.String())
        }

        allOrders = append(allOrders, result.Data.Items...)

        if len(result.Data.Items) < perPage {
            break
        }
        offset += perPage
    }

    return allOrders, nil
}
```

### 8. Map `Contingent` Order Status

**Problem:** Complex order legs can have status `"Contingent"` (awaiting trigger). Our `mapTTStatus` doesn't handle it.

**Fix:** Add `"Contingent"` to the `mapTTStatus` switch, mapping to `broker.OrderSubmitted` since the order exists but is not yet active at the exchange.

```go
case "Received", "Routed", "In Flight", "Contingent":
    return broker.OrderSubmitted
```

### 9. Implement `GroupSubmitter`

Add `SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error` to `TastytradeBroker`.

#### Complex Order Request Types

```go
type complexOrderRequest struct {
    Type         string        `json:"type"`
    TriggerOrder *orderRequest `json:"trigger-order,omitempty"`
    Orders       []orderRequest `json:"orders"`
}
```

#### Client Method

```go
func (client *apiClient) submitComplexOrder(ctx context.Context, order complexOrderRequest) (string, error) {
    endpoint := fmt.Sprintf("/accounts/%s/complex-orders", client.accountID)

    var result complexOrderSubmitResponse
    resp, err := client.resty.R().
        SetContext(ctx).
        SetBody(order).
        SetResult(&result).
        Post(endpoint)
    if err != nil {
        return "", fmt.Errorf("submit complex order: %w", err)
    }
    if resp.IsError() {
        return "", NewHTTPError(resp.StatusCode(), resp.String())
    }

    return result.Data.ComplexOrder.ID, nil
}
```

The response type (complete definition -- also used in Section 10 for populating the `complexOrderIDs` map):

```go
type complexOrderSubmitResponse struct {
    Data struct {
        ComplexOrder struct {
            ID     string          `json:"id"`
            Orders []orderResponse `json:"orders"`
        } `json:"complex-order"`
    } `json:"data"`
}
```

#### SubmitGroup Method

```go
func (ttBroker *TastytradeBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
    if len(orders) == 0 {
        return fmt.Errorf("tastytrade: SubmitGroup called with no orders")
    }

    ttBroker.mu.Lock()
    defer ttBroker.mu.Unlock()

    switch groupType {
    case broker.GroupOCO:
        return ttBroker.submitOCO(ctx, orders)
    case broker.GroupBracket:
        return ttBroker.submitOTOCO(ctx, orders)
    default:
        return fmt.Errorf("tastytrade: unsupported group type %d", groupType)
    }
}
```

**OCO mapping:** All orders go into the `orders` array:

```go
func (ttBroker *TastytradeBroker) submitOCO(ctx context.Context, orders []broker.Order) error {
    ttOrders := make([]orderRequest, len(orders))
    for idx, order := range orders {
        ttOrders[idx] = toTastytradeOrder(order)
    }

    req := complexOrderRequest{
        Type:   "OCO",
        Orders: ttOrders,
    }

    _, err := ttBroker.client.submitComplexOrder(ctx, req)
    return err
}
```

**OTOCO mapping:** The entry order (`GroupRole == RoleEntry`) becomes the `trigger-order`. The remaining orders (stop-loss, take-profit) go into `orders`. Exactly one entry order must be present; zero or multiple is an error.

```go
func (ttBroker *TastytradeBroker) submitOTOCO(ctx context.Context, orders []broker.Order) error {
    var triggerOrder *orderRequest
    var contingentOrders []orderRequest

    for _, order := range orders {
        ttOrder := toTastytradeOrder(order)
        if order.GroupRole == broker.RoleEntry {
            if triggerOrder != nil {
                return fmt.Errorf("tastytrade: OTOCO group has multiple entry orders")
            }
            triggerOrder = &ttOrder
        } else {
            contingentOrders = append(contingentOrders, ttOrder)
        }
    }

    if triggerOrder == nil {
        return fmt.Errorf("tastytrade: OTOCO group has no entry order")
    }

    req := complexOrderRequest{
        Type:         "OTOCO",
        TriggerOrder: triggerOrder,
        Orders:       contingentOrders,
    }

    _, err := ttBroker.client.submitComplexOrder(ctx, req)
    return err
}
```

### 10. Complex Order Cancellation

**Problem:** Cancelling an order that belongs to a complex order group requires `DELETE /accounts/{id}/complex-orders/{complex-order-id}`, not the regular order cancel endpoint. The regular cancel endpoint will reject the request for orders that are part of a complex order.

**Fix:** Add `cancelComplexOrder` to the client:

```go
func (client *apiClient) cancelComplexOrder(ctx context.Context, complexOrderID string) error {
    endpoint := fmt.Sprintf("/accounts/%s/complex-orders/%s", client.accountID, complexOrderID)

    resp, err := client.resty.R().
        SetContext(ctx).
        Delete(endpoint)
    if err != nil {
        return fmt.Errorf("cancel complex order: %w", err)
    }
    if resp.IsError() {
        return NewHTTPError(resp.StatusCode(), resp.String())
    }

    return nil
}
```

The `Cancel` method on the broker needs to determine which endpoint to use. Since `broker.Cancel` only receives an `orderID` string, the broker must track which orders belong to complex order groups. Add an internal map:

```go
type TastytradeBroker struct {
    client          *apiClient
    streamer        *fillStreamer
    fills           chan broker.Fill
    sandbox         bool
    mu              sync.Mutex
    complexOrderIDs map[string]string // orderID -> complexOrderID
}
```

When `SubmitGroup` succeeds, store the mapping from each child order ID to the complex order ID. When `Cancel` is called, check the map first:

```go
func (ttBroker *TastytradeBroker) Cancel(ctx context.Context, orderID string) error {
    ttBroker.mu.Lock()
    complexID, isComplex := ttBroker.complexOrderIDs[orderID]
    ttBroker.mu.Unlock()

    if isComplex {
        return ttBroker.client.cancelComplexOrder(ctx, complexID)
    }

    return ttBroker.client.cancelOrder(ctx, orderID)
}
```

The complex order ID comes from the `submitComplexOrder` response (see `complexOrderSubmitResponse` in Section 9, which includes both the complex order ID and the child `Orders`). Update `submitOCO` and `submitOTOCO` to capture the returned complex order ID and child order IDs, then populate `complexOrderIDs` by mapping each child order ID to the complex order ID.

### 11. Update `orderResponse` with `complex-order-id`

The REST order response includes a `complex-order-id` field for orders that are part of a complex order group. Add this to `orderResponse` so that `Orders()` and `pollMissedFills` can properly identify complex-order membership. This also allows the `complexOrderIDs` map to be populated from REST queries, not just from `SubmitGroup` calls (important for recovery after restart).

Already shown in Section 6 above:

```go
type orderResponse struct {
    // ... existing fields ...
    ComplexOrderID string `json:"complex-order-id"`
}
```

When `Orders()` is called, populate the `complexOrderIDs` map from any order that has a non-empty `ComplexOrderID`.

## API Verification Checklist

After implementation, each endpoint and message format must be verified against the tastytrade developer documentation at https://developer.tastytrade.com/. The implementation plan must include a verification step that checks:

| Area | Endpoint / Format | Doc Reference |
|------|-------------------|--------------|
| Authentication | `POST /sessions` | [Sessions](https://developer.tastytrade.com/open-api-spec/sessions/) |
| Accounts | `GET /customers/me/accounts` | [Accounts](https://developer.tastytrade.com/open-api-spec/accounts/) |
| Submit order | `POST /accounts/{id}/orders` | [Order Management](https://developer.tastytrade.com/order-management/) |
| Cancel order | `DELETE /accounts/{id}/orders/{id}` | [Order Management](https://developer.tastytrade.com/order-management/) |
| Replace order | `PUT /accounts/{id}/orders/{id}` | [Order Management](https://developer.tastytrade.com/order-management/) |
| Get orders | `GET /accounts/{id}/orders` (paginated) | [Order Management](https://developer.tastytrade.com/order-management/) |
| Submit complex order | `POST /accounts/{id}/complex-orders` | [Order Management](https://developer.tastytrade.com/order-management/) |
| Cancel complex order | `DELETE /accounts/{id}/complex-orders/{id}` | [Order Management](https://developer.tastytrade.com/order-management/) |
| Positions | `GET /accounts/{id}/positions` | [Balances and Positions](https://developer.tastytrade.com/open-api-spec/balances-and-positions/) |
| Balances | `GET /accounts/{id}/balances` | [Balances and Positions](https://developer.tastytrade.com/open-api-spec/balances-and-positions/) |
| Quote | `GET /market-data/by-type?equity={symbol}` | [Market Data](https://developer.tastytrade.com/open-api-spec/market-data/) |
| WebSocket connect | `{"action":"connect","value":[account],"auth-token":token}` | [Streaming Account Data](https://developer.tastytrade.com/streaming-account-data/) |
| WebSocket heartbeat | `{"action":"heartbeat"}` | [Streaming Account Data](https://developer.tastytrade.com/streaming-account-data/) |
| Fill notification format | `{"type":"Order","data":{...legs[].fills[]},"timestamp":ms}` | [Streaming Account Data](https://developer.tastytrade.com/streaming-account-data/) |
| Order request fields | `price-effect`, `automated-source` | [Orders API](https://developer.tastytrade.com/open-api-spec/orders/) |
| Order response fields | `complex-order-id`, `legs[].fills[]` | [Orders API](https://developer.tastytrade.com/open-api-spec/orders/) |
| Order statuses | `Contingent` status mapping | [Order Flow](https://developer.tastytrade.com/order-flow/) |
| Complex order types | `OCO`, `OTOCO` request structure | [Order Submission](https://developer.tastytrade.com/order-submission/) |
| Action values | `Buy to Open`, `Sell to Close` (equity scope) | [Orders API](https://developer.tastytrade.com/open-api-spec/orders/) |
| Pagination | `page-offset`, `per-page` on order queries | [Orders API](https://developer.tastytrade.com/open-api-spec/orders/) |

The verification step must fetch each doc URL and confirm the implementation matches the documented endpoint path, HTTP method, request body fields, and response structure. Discrepancies must be flagged and resolved before the work is considered complete.

## Files Modified

| File | Change |
|------|--------|
| `broker/tastytrade/types.go` | Add `price-effect` and `automated-source` to `orderRequest`. Add `complexOrderRequest`, `complexOrderSubmitResponse`, `streamerMessage` types. Add `legFillResponse` type. Add `complex-order-id` and nested `Fills []legFillResponse` to `orderResponse`/`orderLegResponse`. Update `toTastytradeOrder()` to set `price-effect` and `automated-source`. Add `parseLegFillQuantity` helper. |
| `broker/tastytrade/client.go` | Fix `getQuote()` endpoint. Add pagination to `getOrders()`. Add `submitComplexOrder()` and `cancelComplexOrder()` methods. Add `sessionToken()` and `account()` accessors. |
| `broker/tastytrade/broker.go` | Add `complexOrderIDs` map to `TastytradeBroker`. Add `SubmitGroup()`, `submitOCO()`, `submitOTOCO()` methods. Update `Cancel()` to check complex order map. Update `Orders()` to populate complex order map from REST response. Initialize `complexOrderIDs` in `New()`. |
| `broker/tastytrade/streamer.go` | Update `handleMessage()` to parse streamer envelope and extract fills from `legs[].fills[]`. Add `connect` message after WebSocket dial. Add heartbeat ticker in `run()`. Update `pollMissedFills()` to extract fills from order leg structure. |
| `broker/tastytrade/errors.go` | Add sentinel errors for invalid group submissions (`ErrEmptyOrderGroup`, `ErrNoEntryOrder`, `ErrMultipleEntryOrders`). |
| `broker/tastytrade/exports_test.go` | Add test exports for `submitComplexOrder`, `cancelComplexOrder`, `sessionToken`, `account`. Add `ComplexOrderRequest` and related type aliases. |

## Testing Strategy

- Compile-time check: `var _ broker.GroupSubmitter = (*TastytradeBroker)(nil)`
- Unit tests for `SubmitGroup` with OCO and OTOCO using `httptest.Server` mock
- Unit tests for `SubmitGroup` validation: empty orders slice, OTOCO with no entry, OTOCO with multiple entries
- Unit tests for `Cancel` routing between simple and complex order endpoints
- Unit tests for updated `handleMessage` with tastytrade streamer envelope format
- Unit tests for WebSocket connect message sent on initial connect and on reconnect
- Unit tests for heartbeat messages being sent periodically
- Unit tests for `pollMissedFills` with nested `legs[].fills[]` structure
- Unit tests for `getOrders` pagination (multi-page response)
- Unit tests for `getQuote` with new endpoint format
- Unit tests for `price-effect` and `automated-source` in `toTastytradeOrder`
- Unit tests for `Contingent` status mapping
- Unit tests for `Orders()` populating `complexOrderIDs` from REST response
- Update existing tests to use correct API response formats where they differ
- API verification step in the implementation plan that fetches developer docs and confirms conformance
