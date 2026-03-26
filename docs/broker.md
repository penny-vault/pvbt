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
    Transactions(ctx context.Context, since time.Time) ([]Transaction, error)
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

### Transaction sync

- **Transactions** returns account activity (dividends, splits, fees, delistings, etc.) since a given time. The engine calls this during housekeeping to sync broker-side events into the portfolio. Each transaction carries a stable ID so the portfolio can deduplicate across repeated calls.

For live brokers, this pulls the brokerage's actual transaction history. For the simulated broker, it synthesizes transactions from data provider metrics (dividends, split factors) and detects delistings when an asset's price data disappears.

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

## Transaction

A `Transaction` represents an account activity entry reported by the broker. The engine syncs these into the portfolio's transaction log via `Account.SyncTransactions`, which deduplicates by `ID`.

```go
type Transaction struct {
    ID            string
    Date          time.Time
    Asset         asset.Asset
    Type          asset.TransactionType
    Qty           float64
    Price         float64
    Amount        float64
    Justification string
}
```

`Type` uses `asset.TransactionType`, which is shared between the `broker` and `portfolio` packages: Buy, Sell, Dividend, Fee, Deposit, Withdrawal, Split, Interest, Journal.

Splits receive special handling during sync -- they adjust holdings and tax lots via `ApplySplit` rather than recording a simple cash event. All other types are recorded directly.

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

**Transaction synthesis.** The simulated broker's `Transactions` method synthesizes corporate action and fee transactions from data provider metrics. At each step it returns dividends (from `Dividend` metric), splits (from `SplitFactor` metric), and borrow fees (computed from the configured borrow rate). When a held asset's close price becomes NaN or zero, the broker treats it as a delisting and emits a sell transaction at the last known price to liquidate the position.

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

The `broker/tastytrade` package enables live trading through tastytrade. It supports equities with market, limit, stop, and stop-limit orders, dollar-amount orders, and OCO/bracket (OTOCO) order groups.

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

The `broker/alpaca` package enables live and paper trading through Alpaca. It supports equities with market, limit, stop, and stop-limit orders, dollar-amount orders, fractional shares, and OCO/bracket order groups.

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

### Schwab

The `broker/schwab` package enables live trading through Charles Schwab. It supports equities with market, limit, stop, and stop-limit orders, dollar-amount orders, tax lot selection, and OCO/bracket order groups.

```go
import "github.com/penny-vault/pvbt/broker/schwab"

schwabBroker := schwab.New()

eng := engine.New(&MyStrategy{},
    engine.WithBroker(schwabBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

Authentication uses environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `SCHWAB_CLIENT_ID` | yes | OAuth app client ID |
| `SCHWAB_CLIENT_SECRET` | yes | OAuth app client secret |
| `SCHWAB_CALLBACK_URL` | no | Registered OAuth callback URL (default: `https://127.0.0.1:5174`) |
| `SCHWAB_TOKEN_FILE` | no | Path to persist OAuth tokens (default: `~/.config/pvbt/schwab-tokens.json`) |
| `SCHWAB_ACCOUNT_NUMBER` | no | Plain-text account number; if unset, uses the first linked account |

Schwab uses OAuth 2.0 with the authorization_code grant type. On first run, `Connect()` prints an authorization URL to the console. Open it in a browser, log in, and authorize the app -- a local HTTPS callback server captures the tokens automatically. The access token refreshes automatically every 25 minutes. The refresh token is persisted to disk and is valid for 7 days; after expiry, the browser authorization flow must be repeated.

Fills are delivered via a WebSocket connection to Schwab's streaming API (`ACCT_ACTIVITY` service). On disconnect, the broker reconnects with exponential backoff and polls the REST orders endpoint for any fills missed during the outage. Duplicate fills are suppressed automatically.

### Tradier

The `broker/tradier` package enables live and paper trading through Tradier. It supports equities with market, limit, stop, and stop-limit orders, dollar-amount orders, and OCO/bracket (OTOCO) order groups.

```go
import "github.com/penny-vault/pvbt/broker/tradier"

tradierBroker := tradier.New()
// or for paper trading:
tradierBroker := tradier.New(tradier.WithSandbox())

eng := engine.New(&MyStrategy{},
    engine.WithBroker(tradierBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

Tradier supports two authentication modes. The broker auto-detects which to use based on environment variables.

**API access token** (individual developers): Set `TRADIER_ACCESS_TOKEN` with a long-lived token from the Tradier developer portal. Individual tokens never expire.

| Variable | Required | Description |
|----------|----------|-------------|
| `TRADIER_ACCESS_TOKEN` | yes | Long-lived API access token |
| `TRADIER_ACCOUNT_ID` | yes | Account number to trade |

**OAuth 2.0** (partner/multi-user apps): Set `TRADIER_CLIENT_ID` and `TRADIER_CLIENT_SECRET`. On first run, `Connect()` prints an authorization URL to the console. Open it in a browser, log in, and authorize the app -- a local HTTPS callback server captures the tokens automatically. Access tokens expire in 24 hours. Refresh tokens (available only to approved Tradier Partners) are used automatically when present; without one, the browser flow must be repeated after 24 hours.

| Variable | Required | Description |
|----------|----------|-------------|
| `TRADIER_CLIENT_ID` | yes | OAuth app client ID |
| `TRADIER_CLIENT_SECRET` | yes | OAuth app client secret |
| `TRADIER_ACCOUNT_ID` | yes | Account number to trade |
| `TRADIER_CALLBACK_URL` | no | OAuth callback URL (default: `https://127.0.0.1:5174`) |
| `TRADIER_TOKEN_FILE` | no | Path to persist OAuth tokens (default: `~/.config/pvbt/tradier-tokens.json`) |

Day and GTC durations are supported. IOC, FOK, GTD, OnOpen, and OnClose are not supported by Tradier and return an error.

Tradier's sandbox environment provides paper trading with simulated fills. Account event streaming is not available in sandbox mode; the broker falls back to polling the orders endpoint every 2 seconds.

Fills are delivered via a WebSocket connection to Tradier's account events streamer. On disconnect, the broker reconnects with exponential backoff and polls the REST orders endpoint for any fills missed during the outage. Duplicate fills are suppressed automatically.

### Interactive Brokers

The `broker/ibkr` package enables live trading through Interactive Brokers. It supports equities with market, limit, stop, and stop-limit orders, dollar-amount orders, and bracket and OCA order groups. Two ways to authenticate are available: OAuth for users with registered consumer keys, and the Client Portal Gateway for everyone else.

```go
import "github.com/penny-vault/pvbt/broker/ibkr"

// Using the Client Portal Gateway (requires gateway process running):
ibBroker := ibkr.New(ibkr.WithGateway("https://localhost:5000"))

// Using OAuth:
ibBroker := ibkr.New(ibkr.WithOAuth(ibkr.OAuthConfig{
    ConsumerKey: "your-consumer-key",
    KeyFile:     "/path/to/signing-key.pem",
}))

eng := engine.New(&MyStrategy{},
    engine.WithBroker(ibBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

**Gateway authentication** requires a running IB Client Portal Gateway process. The user must log in through the gateway's web UI before starting the strategy. The broker verifies the session on `Connect()` and keeps it alive with periodic tickle requests.

| Variable | Required | Description |
|----------|----------|-------------|
| `IBKR_GATEWAY_URL` | no | Gateway URL (default: `https://localhost:5000`) |

**OAuth authentication** uses IB's OAuth 1.0a-style signing flow with RSA-SHA256 and a Diffie-Hellman live session token exchange. Consumer keys are managed through IB's Self Service Portal.

| Variable | Required | Description |
|----------|----------|-------------|
| `IBKR_CONSUMER_KEY` | yes | OAuth consumer key |
| `IBKR_SIGNING_KEY_FILE` | yes | Path to RSA signing key (PEM, PKCS#8) |
| `IBKR_ACCESS_TOKEN` | no | Pre-existing access token (skip token exchange) |
| `IBKR_ACCESS_TOKEN_SECRET` | no | Pre-existing access token secret |

IB uses contract IDs (`conid`) rather than ticker symbols for order operations. The broker resolves tickers to conids automatically via `/iserver/secdef/search` and caches results for the session. GTD and FOK time-in-force values are not supported by IB's Web API and return an error.

Fills are delivered via a WebSocket connection. On disconnect, the broker reconnects with exponential backoff and polls the trades endpoint for any fills missed during the outage. Duplicate fills are suppressed automatically.

IB enforces a global rate limit of 10 requests per second; the client stays under this with a built-in rate limiter.

### E\*TRADE

The `broker/etrade` package enables live and paper trading through E\*TRADE (Morgan Stanley). It supports equities with market, limit, stop, and stop-limit orders and dollar-amount orders. E\*TRADE's API does not support contingent orders (OCO, bracket); the account layer manages group cancellation for this broker.

```go
import "github.com/penny-vault/pvbt/broker/etrade"

etradeBroker := etrade.New()
// or for paper trading:
etradeBroker := etrade.New(etrade.WithSandbox())

eng := engine.New(&MyStrategy{},
    engine.WithBroker(etradeBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

Authentication uses OAuth 1.0a with HMAC-SHA1 signed requests:

| Variable | Required | Description |
|----------|----------|-------------|
| `ETRADE_CONSUMER_KEY` | yes | OAuth 1.0a consumer key from the E\*TRADE developer portal |
| `ETRADE_CONSUMER_SECRET` | yes | OAuth 1.0a consumer secret |
| `ETRADE_ACCOUNT_ID_KEY` | yes | Account ID key (from the List Accounts API, not the display account number) |
| `ETRADE_CALLBACK_URL` | no | OAuth callback URL (default: out-of-band) |
| `ETRADE_TOKEN_FILE` | no | Path to persist OAuth tokens (default: `~/.config/pvbt/etrade-tokens.json`) |

On first run, `Connect()` prints an authorization URL to the console. Open it in a browser, log in, and authorize the app -- a local HTTPS callback server captures the verifier automatically. E\*TRADE access tokens expire at midnight US Eastern time every day and go inactive after 2 hours without API activity. The broker renews the token every 90 minutes to prevent inactivity timeout; after midnight expiry, the browser authorization flow must be repeated.

E\*TRADE requires a preview step before placing any order. The broker handles this transparently -- `Submit` calls preview, extracts the preview ID, then immediately places the order.

Day, GTC, GTD, IOC, and FOK durations are supported. OnOpen and OnClose are not supported and return an error.

Fills are detected by polling the orders endpoint every 2 seconds. Duplicate fills are suppressed automatically.

### Other brokers

Additional brokers can be added by implementing the `Broker` interface. Broker implementations live in sub-packages under `broker/`.
