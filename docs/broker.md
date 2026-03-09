# Broker

The broker package defines the interface between the portfolio and a brokerage. It is intentionally decoupled from the portfolio package -- the broker package imports only `asset`, and the portfolio package imports `broker`. This one-way dependency keeps the packages clean and allows broker implementations to be developed independently.

When a portfolio has an associated broker, order execution is delegated to the broker. When no broker is attached, the portfolio uses simulated execution for backtesting. Strategy code never interacts with the broker directly -- the portfolio handles translation between its modifier-based order API and the broker's concrete order types.

## Broker interface

```go
type Broker interface {
    Connect(ctx context.Context) error
    Close() error
    Submit(order Order) ([]Fill, error)
    Cancel(orderID string) error
    Replace(orderID string, order Order) ([]Fill, error)
    Orders() ([]Order, error)
    Positions() ([]Position, error)
    Balance() (Balance, error)
}
```

### Lifecycle

- **Connect** establishes a session with the brokerage, performing authentication and any setup required before trading. Credentials and token refresh are implementation details of each broker. For example, a tastytrade implementation would accept an API key in its constructor and use OAuth2 to obtain a session token in `Connect`.
- **Close** tears down the broker session and releases resources. If the portfolio outlives the broker, call `SetBroker(nil)` before closing the broker.

### Trading

- **Submit** sends an order to the brokerage and returns one or more fill reports. Large orders may be filled in multiple lots at different prices.
- **Cancel** requests cancellation of an open order by ID.
- **Replace** performs an atomic cancel-replace: cancels an existing order and submits a replacement in one operation. Returns one or more fills for the replacement order.

### Queries

- **Orders** returns all orders for the current trading day.
- **Positions** returns all current positions in the account.
- **Balance** returns the current account balance.

## Order

An `Order` describes what to trade, how to price it, and how long it should remain active:

```go
type Order struct {
    ID          string
    Asset       asset.Asset
    Side        Side
    Qty         float64
    OrderType   OrderType
    TimeInForce TimeInForce
    LimitPrice  float64
    StopPrice   float64
    GTDDate     time.Time
}
```

`Side` is defined in the broker package (not imported from portfolio) to keep the dependency direction clean.

```go
type Side int

const (
    Buy Side = iota
    Sell
)
```

### Order types

| Type | Behavior |
|------|----------|
| `Market` | Executes at the next available price |
| `Limit` | Maximum buy price or minimum sell price |
| `Stop` | Triggers a market order when the price reaches a threshold |
| `StopLimit` | Triggers a limit order when the price reaches a threshold |

### Time in force

| Value | Behavior |
|-------|----------|
| `Day` | Cancels at market close if not executed |
| `GTC` | Good til cancelled (up to 180 days at most brokers) |
| `GTD` | Good til a specified date (set via `GTDDate`) |
| `IOC` | Immediate or cancel -- fill what you can, cancel the rest |
| `FOK` | Fill or kill -- fill entirely or cancel immediately |
| `OnOpen` | Fill only at the opening price |
| `OnClose` | Fill only at the closing price |

## Fill

A `Fill` reports how an order was executed:

```go
type Fill struct {
    OrderID  string
    Price    float64
    Qty      float64
    FilledAt time.Time
}
```

## Position

A `Position` represents a holding in the account:

```go
type Position struct {
    Asset         asset.Asset
    Qty           float64
    AvgOpenPrice  float64
    MarkPrice     float64
    RealizedDayPL float64
}
```

## Balance

`Balance` represents the account's financial state:

```go
type Balance struct {
    CashBalance         float64
    NetLiquidatingValue float64
    EquityBuyingPower   float64
    MaintenanceReq      float64
}
```

## Connecting a broker to a portfolio

A broker is always required. For backtesting, the engine provides a simulated broker; for live trading, attach a real one. The portfolio delegates all order execution to the broker and never computes fill prices itself.

```go
// at construction
p := portfolio.New(portfolio.WithCash(10_000), portfolio.WithBroker(b))

// swap brokers at runtime
p.SetBroker(liveBroker)
```

The portfolio translates its modifier-based orders (`Limit(150.00)`, `GoodTilCancel`, etc.) into `broker.Order` values with concrete `OrderType` and `TimeInForce` fields before calling `Submit`. Strategy code is never aware of the broker.

## Implementations

Broker implementations live in sub-packages under `broker/`. The first planned implementations are a simulated broker for backtesting and a tastytrade broker for live trading. Additional brokers (e.g., Interactive Brokers, Schwab) can be added by implementing the `Broker` interface.
