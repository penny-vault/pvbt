# Broker

The broker package defines the interface between the portfolio and a brokerage. It is intentionally decoupled from the portfolio package -- the broker package imports only `asset`, and the portfolio package imports `broker`. This one-way dependency keeps the packages clean and allows broker implementations to be developed independently.

When a portfolio has an associated broker, order execution is delegated to the broker. When no broker is attached, the portfolio uses simulated execution for backtesting. Strategy code never interacts with the broker directly -- the portfolio handles translation between its modifier-based order API and the broker's concrete order types.

## Broker interface

```go
type Broker interface {
    Connect(ctx context.Context) error
    Close() error
    Submit(ctx context.Context, order Order) error
    Fills() <-chan Fill
    Cancel(ctx context.Context, orderID string) error
    Replace(ctx context.Context, orderID string, order Order) error
    Orders(ctx context.Context) ([]Order, error)
    Positions(ctx context.Context) ([]Position, error)
    Balance(ctx context.Context) (Balance, error)
}
```

All trading and query methods now accept a `context.Context` for cancellation and deadline propagation.

### Lifecycle

- **Connect** establishes a session with the brokerage, performing authentication and any setup required before trading. Credentials and token refresh are implementation details of each broker. For example, a tastytrade implementation would accept an API key in its constructor and use OAuth2 to obtain a session token in `Connect`.
- **Close** tears down the broker session and releases resources. If the portfolio outlives the broker, call `SetBroker(nil)` before closing the broker.

### Trading

- **Submit** sends an order to the brokerage. It returns only an error -- fills are delivered asynchronously through the `Fills` channel.
- **Fills** returns a receive-only channel (`<-chan Fill`) on which fill reports arrive after each `Submit` call. The engine drains this channel at every step.
- **Cancel** requests cancellation of an open order by ID.
- **Replace** performs an atomic cancel-replace: cancels an existing order and submits a replacement in one operation.

### Queries

- **Orders** returns all orders for the current trading day.
- **Positions** returns all current positions in the account.
- **Balance** returns the current account balance.

## Order

An `Order` describes what to trade, how to price it, and how long it should remain active:

```go
type Order struct {
    ID            string
    Asset         asset.Asset
    Side          Side
    Status        OrderStatus
    Qty           float64
    Amount        float64
    OrderType     OrderType
    TimeInForce   TimeInForce
    LimitPrice    float64
    StopPrice     float64
    GTDDate       time.Time
    Justification string
    GroupID       string
    GroupRole     GroupRole
}
```

`Justification` is an optional human-readable explanation carried from the strategy through middleware to the fill. It is set by `WithJustification` on the portfolio order API and copied onto resulting transactions.

`Side` is defined in the broker package (not imported from portfolio) to keep the dependency direction clean.

```go
type Side int

const (
    Buy Side = iota
    Sell
)
```

### Dollar-amount orders

When `Qty` is zero and `Amount` is positive, the broker treats it as a dollar-amount order and computes the share quantity from the current market price. This is useful for allocating a fixed dollar amount rather than a specific number of shares.

### Order status

`OrderStatus` tracks the lifecycle state of an order:

```go
type OrderStatus int

const (
    OrderOpen OrderStatus = iota
    OrderSubmitted
    OrderFilled
    OrderPartiallyFilled
    OrderCancelled
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

## Order Groups

Orders can be linked into groups for coordinated execution.

| Type | Description |
|------|-------------|
| `GroupOCO` | Two orders where filling one cancels the other |
| `GroupBracket` | Entry order plus an OCO pair of exits |

`GroupRole` identifies an order's purpose within its group:

| Role | Description |
|------|-------------|
| `RoleEntry` | The entry order in a bracket group |
| `RoleStopLoss` | The stop-loss exit leg |
| `RoleTakeProfit` | The take-profit exit leg |

The `GroupSubmitter` interface allows brokers to submit linked orders atomically:

```go
type GroupSubmitter interface {
    SubmitGroup(ctx context.Context, orders []Order, groupType GroupType) error
}
```

When a broker implements `GroupSubmitter`, the account submits OCO pairs through it for atomic execution. When it does not, the account submits orders individually and manages cancellation on fill.

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

## Connecting a broker

A broker is always required. For backtesting, the engine provides a simulated broker; for live trading, pass a real broker to the engine via `engine.WithBroker(b)`. The portfolio delegates all order execution to the broker and never computes fill prices itself.

```go
// backtesting -- simulated broker is attached automatically
eng := engine.New(&MyStrategy{},
    engine.WithInitialDeposit(10_000),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)

// live trading -- provide a real broker
eng := engine.New(&MyStrategy{},
    engine.WithInitialDeposit(10_000),
    engine.WithBroker(liveBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

The batch translates its modifier-based orders (`Limit(150.00)`, `GoodTilCancel`, etc.) into `broker.Order` values with concrete `OrderType` and `TimeInForce` fields. After middleware processing, the engine calls `Submit` for each order. Strategy code is never aware of the broker.

## PriceProvider

The `PriceProvider` interface supplies current market prices. The engine implements this interface; the simulated broker uses it to determine fill prices and convert dollar-amount orders to share quantities.

```go
type PriceProvider interface {
    Prices(ctx context.Context, assets ...asset.Asset) (*data.DataFrame, error)
}
```

## Implementations

### SimulatedBroker

The `SimulatedBroker` fills all orders at the close price for backtesting. The engine sets a `PriceProvider` and date on the simulated broker before each step. It supports dollar-amount orders by dividing the requested dollar amount by the current price (rounded down to whole shares). Fills are delivered through the `Fills()` channel, consistent with the async interface used by live brokers. The simulated broker does not support `Cancel` or `Replace` operations.

#### Short selling

The simulated broker supports short selling. All securities are assumed to be borrowable -- the broker does not model locate requirements or hard-to-borrow conditions.

**Margin enforcement.** When a short order is submitted, the broker checks that the account has sufficient cash to satisfy the initial margin requirement. If cash is insufficient, the order is rejected and the fill channel receives no fill for that order ID. Margin rates are configurable at construction:

```go
sim := broker.NewSimulatedBroker(priceProvider,
    broker.WithInitialMargin(0.50),      // 50% initial margin (default: Reg T, 50%)
    broker.WithMaintenanceMargin(0.30),  // 30% maintenance margin (default: 25%)
)
```

`WithInitialMargin` sets the fraction of the short sale proceeds that must be held as collateral at the time of the trade. `WithMaintenanceMargin` sets the fraction of the current market value of short positions that must be maintained as equity; the engine checks this every trading day (see [engine.md](engine.md)).

**Borrow fees.** The simulated broker charges a daily borrow fee on all open short positions. The fee is calculated from a configurable annualized rate and debited from cash each day during the engine's housekeeping step:

```go
sim := broker.NewSimulatedBroker(priceProvider,
    broker.WithBorrowRate(0.03), // 3% annualized borrow fee (default: 0.0%)
)
```

The daily fee for each short lot is `(annualized rate / 252) * current market value of the lot`. The same rate applies to all securities; per-symbol borrow rate modeling is not currently supported.

### tastytrade

The `broker/tastytrade` package implements `Broker` and `GroupSubmitter` for live trading with tastytrade. It supports equities, all four order types (Market, Limit, Stop, StopLimit), dollar-amount orders, and native OCO/bracket (OTOCO) order groups.

```go
import "github.com/penny-vault/pvbt/broker/tastytrade"

ttBroker := tastytrade.New()
// or for paper trading:
ttBroker := tastytrade.New(tastytrade.WithSandbox())

eng := engine.New(&MyStrategy{},
    engine.WithBroker(ttBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

Authentication uses environment variables:

| Variable | Description |
|----------|-------------|
| `TASTYTRADE_USERNAME` | tastytrade account username |
| `TASTYTRADE_PASSWORD` | tastytrade account password |

Authentication happens during `Connect()`. The session token is managed internally and refreshed automatically on 401 responses.

Fills are delivered via a WebSocket connection to tastytrade's account streamer. On disconnect, the broker reconnects with exponential backoff and polls for any fills missed during the outage. Duplicate fills are suppressed automatically.

### Alpaca

The `broker/alpaca` package implements `Broker` and `GroupSubmitter` for live and paper trading with Alpaca. It supports equities, all four order types (Market, Limit, Stop, StopLimit), dollar-amount orders, fractional shares, and native OCO/bracket orders.

```go
import "github.com/penny-vault/pvbt/broker/alpaca"

alphaBroker := alpaca.New()
// or for paper trading:
alphaBroker := alpaca.New(alpaca.WithPaper())
// or with fractional shares:
alphaBroker := alpaca.New(alpaca.WithFractionalShares())

eng := engine.New(&MyStrategy{},
    engine.WithBroker(alphaBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

Authentication uses environment variables:

| Variable | Description |
|----------|-------------|
| `ALPACA_API_KEY` | Alpaca API key ID |
| `ALPACA_API_SECRET` | Alpaca API secret key |

Authentication happens during `Connect()`. API keys are stateless -- no session token management is required.

Fills are delivered via a WebSocket connection to Alpaca's trade updates stream. On disconnect, the broker reconnects with exponential backoff and polls for any fills missed during the outage.

### Other brokers

Additional brokers can be added by implementing the `Broker` interface. Broker implementations live in sub-packages under `broker/`.
