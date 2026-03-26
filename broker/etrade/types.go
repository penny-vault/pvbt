package etrade

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

// --- E*TRADE API response types ---

type etradeAccountListResponse struct {
	AccountListResponse struct {
		Accounts struct {
			Account []etradeAccount `json:"Account"`
		} `json:"Accounts"`
	} `json:"AccountListResponse"`
}

type etradeAccount struct {
	AccountID     string `json:"accountId"`
	AccountIDKey  string `json:"accountIdKey"`
	AccountMode   string `json:"accountMode"`
	AccountType   string `json:"accountType"`
	AccountStatus string `json:"accountStatus"`
}

type etradeBalanceResponse struct {
	BalanceResponse struct {
		Cash        float64               `json:"totalAccountValue"`
		Computed    etradeComputedBalance `json:"Computed"`
		AccountType string                `json:"accountType"`
	} `json:"BalanceResponse"`
}

type etradeComputedBalance struct {
	CashAvailableForInvestment float64              `json:"cashAvailableForInvestment"`
	RealTimeValues             etradeRealTimeValues `json:"RealTimeValues"`
	CashBuyingPower            float64              `json:"cashBuyingPower"`
	MarginBuyingPower          float64              `json:"marginBuyingPower"`
	MaintenanceReq             float64              `json:"reqMaintenanceValue"`
}

type etradeRealTimeValues struct {
	TotalAccountValue float64 `json:"totalAccountValue"`
}

type etradePortfolioResponse struct {
	PortfolioResponse struct {
		AccountPortfolio []struct {
			Position []etradePosition `json:"Position"`
		} `json:"AccountPortfolio"`
	} `json:"PortfolioResponse"`
}

type etradePosition struct {
	PositionID   int64   `json:"positionId"`
	Symbol       string  `json:"-"`
	Quantity     float64 `json:"quantity"`
	PositionType string  `json:"positionType"`
	CostPerShare float64 `json:"costPerShare"`
	MarketValue  float64 `json:"marketValue"`
	TotalGain    float64 `json:"totalGain"`
	Product      struct {
		Symbol       string `json:"symbol"`
		SecurityType string `json:"securityType"`
	} `json:"Product"`
}

type etradeOrdersResponse struct {
	OrdersResponse struct {
		Order []etradeOrderDetail `json:"Order"`
	} `json:"OrdersResponse"`
}

type etradeOrderDetail struct {
	OrderID   int64            `json:"orderId"`
	OrderType string           `json:"orderType"`
	Status    string           `json:"orderStatus"`
	OrderList []etradeOrderLeg `json:"OrderDetail"`
}

type etradeOrderLeg struct {
	PriceType     string             `json:"priceType"`
	OrderTerm     string             `json:"orderTerm"`
	LimitPrice    float64            `json:"limitPrice"`
	StopPrice     float64            `json:"stopPrice"`
	MarketSession string             `json:"marketSession"`
	Instrument    []etradeInstrument `json:"Instrument"`
	ExecutedPrice float64            `json:"executedPrice"`
	FilledQty     float64            `json:"filledQuantity"`
	OrderedQty    float64            `json:"orderedQuantity"`
}

type etradeInstrument struct {
	Product struct {
		Symbol       string `json:"symbol"`
		SecurityType string `json:"securityType"`
	} `json:"Product"`
	OrderAction  string  `json:"orderAction"`
	Quantity     float64 `json:"orderedQuantity"`
	FilledQty    float64 `json:"filledQuantity"`
	AveragePrice float64 `json:"averageExecutionPrice"`
}

// Preview/Place order request types.

type etradePreviewRequest struct {
	OrderType     string           `json:"orderType"`
	ClientOrderID string           `json:"clientOrderId"`
	Order         []etradeOrderReq `json:"Order"`
}

type etradeOrderReq struct {
	PriceType     string           `json:"priceType"`
	OrderTerm     string           `json:"orderTerm"`
	MarketSession string           `json:"marketSession"`
	LimitPrice    float64          `json:"limitPrice,omitempty"`
	StopPrice     float64          `json:"stopPrice,omitempty"`
	AllOrNone     bool             `json:"allOrNone"`
	Instrument    []etradeInstrReq `json:"Instrument"`
}

type etradeInstrReq struct {
	Product struct {
		Symbol       string `json:"symbol"`
		SecurityType string `json:"securityType"`
	} `json:"Product"`
	OrderAction  string  `json:"orderAction"`
	QuantityType string  `json:"quantityType"`
	Quantity     float64 `json:"quantity"`
}

type etradePreviewResponse struct {
	PreviewOrderResponse struct {
		PreviewIDs []struct {
			PreviewID int64 `json:"previewId"`
		} `json:"PreviewIds"`
	} `json:"PreviewOrderResponse"`
}

type etradePlaceResponse struct {
	PlaceOrderResponse struct {
		OrderID int64 `json:"orderId"`
	} `json:"PlaceOrderResponse"`
}

type etradeQuoteResponse struct {
	QuoteResponse struct {
		QuoteData []struct {
			All struct {
				LastTrade float64 `json:"lastTrade"`
			} `json:"All"`
		} `json:"QuoteData"`
	} `json:"QuoteResponse"`
}

type etradeTransactionsResponse struct {
	TransactionListResponse struct {
		Transaction []etradeTransaction `json:"Transaction"`
	} `json:"TransactionListResponse"`
}

type etradeTransaction struct {
	TransactionID   int64   `json:"transactionId"`
	TransactionDate string  `json:"transactionDate"`
	Amount          float64 `json:"amount"`
	Description     string  `json:"description"`
	Brokerage       struct {
		Product struct {
			Symbol string `json:"symbol"`
		} `json:"Product"`
		Quantity float64 `json:"quantity"`
		Price    float64 `json:"price"`
		Fee      float64 `json:"fee"`
	} `json:"Brokerage"`
}

// --- Translation functions ---

// toBrokerOrder converts an etradeOrderDetail to a broker.Order.
// It extracts the first leg's first instrument to determine the ticker, side,
// quantity, and prices.
func toBrokerOrder(detail etradeOrderDetail) broker.Order {
	ord := broker.Order{
		ID:     strconv.FormatInt(detail.OrderID, 10),
		Status: mapOrderStatus(detail.Status),
	}

	if len(detail.OrderList) == 0 {
		return ord
	}

	leg := detail.OrderList[0]
	ord.OrderType = unmapPriceType(leg.PriceType)
	ord.LimitPrice = leg.LimitPrice
	ord.StopPrice = leg.StopPrice

	if len(leg.Instrument) == 0 {
		return ord
	}

	instr := leg.Instrument[0]
	ord.Asset = asset.Asset{Ticker: instr.Product.Symbol}
	ord.Side = unmapOrderAction(instr.OrderAction)
	ord.Qty = instr.Quantity

	return ord
}

// toBrokerPosition converts an etradePosition to a broker.Position.
func toBrokerPosition(pos etradePosition) broker.Position {
	return broker.Position{
		Asset:         asset.Asset{Ticker: pos.Product.Symbol},
		Qty:           pos.Quantity,
		AvgOpenPrice:  pos.CostPerShare,
		MarkPrice:     pos.MarketValue / pos.Quantity,
		RealizedDayPL: 0,
	}
}

// toBrokerBalance converts an etradeBalanceResponse to a broker.Balance.
// For margin accounts, MarginBuyingPower is used for EquityBuyingPower.
// For cash accounts, CashBuyingPower is used.
func toBrokerBalance(resp etradeBalanceResponse) broker.Balance {
	computed := resp.BalanceResponse.Computed
	buyingPower := computed.MarginBuyingPower

	if buyingPower == 0 {
		buyingPower = computed.CashBuyingPower
	}

	return broker.Balance{
		CashBalance:         computed.CashAvailableForInvestment,
		NetLiquidatingValue: computed.RealTimeValues.TotalAccountValue,
		EquityBuyingPower:   buyingPower,
		MaintenanceReq:      computed.MaintenanceReq,
	}
}

// toBrokerTransaction converts an etradeTransaction to a broker.Transaction.
func toBrokerTransaction(txn etradeTransaction) broker.Transaction {
	parsedDate, dateErr := parseDate(txn.TransactionDate)
	if dateErr != nil {
		log.Warn().Err(dateErr).Str("transactionId", strconv.FormatInt(txn.TransactionID, 10)).Msg("etrade: could not parse transaction date")
	}

	txType := classifyTransaction(txn.Description)

	return broker.Transaction{
		ID:            strconv.FormatInt(txn.TransactionID, 10),
		Date:          parsedDate,
		Asset:         asset.Asset{Ticker: txn.Brokerage.Product.Symbol},
		Type:          txType,
		Qty:           txn.Brokerage.Quantity,
		Price:         txn.Brokerage.Price,
		Amount:        txn.Amount,
		Justification: txn.Description,
	}
}

// classifyTransaction maps an E*TRADE description string to an asset.TransactionType.
func classifyTransaction(description string) asset.TransactionType {
	upper := strings.ToUpper(description)
	switch {
	case strings.Contains(upper, "BOUGHT") || strings.Contains(upper, "BUY"):
		return asset.BuyTransaction
	case strings.Contains(upper, "SOLD") || strings.Contains(upper, "SELL"):
		return asset.SellTransaction
	case strings.Contains(upper, "DIVIDEND"):
		return asset.DividendTransaction
	case strings.Contains(upper, "INTEREST"):
		return asset.InterestTransaction
	case strings.Contains(upper, "FEE") || strings.Contains(upper, "COMMISSION"):
		return asset.FeeTransaction
	case strings.Contains(upper, "DEPOSIT") || strings.Contains(upper, "CONTRIBUTION"):
		return asset.DepositTransaction
	case strings.Contains(upper, "WITHDRAWAL") || strings.Contains(upper, "DISBURSEMENT"):
		return asset.WithdrawalTransaction
	case strings.Contains(upper, "SPLIT"):
		return asset.SplitTransaction
	default:
		return asset.JournalTransaction
	}
}

// toEtradeOrderRequest creates an etradePreviewRequest from a broker.Order.
func toEtradeOrderRequest(order broker.Order) (etradePreviewRequest, error) {
	priceType := mapPriceType(order.OrderType)

	orderTerm, err := mapOrderTerm(order.TimeInForce)
	if err != nil {
		return etradePreviewRequest{}, fmt.Errorf("etrade: build order request: %w", err)
	}

	clientID := strings.ReplaceAll(uuid.New().String(), "-", "")
	if len(clientID) > 20 {
		clientID = clientID[:20]
	}

	var instr etradeInstrReq

	instr.Product.Symbol = order.Asset.Ticker
	instr.Product.SecurityType = "EQ"
	instr.OrderAction = mapOrderAction(order.Side)
	instr.QuantityType = "QUANTITY"
	instr.Quantity = order.Qty

	req := etradePreviewRequest{
		OrderType:     "EQ",
		ClientOrderID: clientID,
		Order: []etradeOrderReq{
			{
				PriceType:     priceType,
				OrderTerm:     orderTerm,
				MarketSession: "REGULAR",
				LimitPrice:    order.LimitPrice,
				StopPrice:     order.StopPrice,
				AllOrNone:     false,
				Instrument:    []etradeInstrReq{instr},
			},
		},
	}

	return req, nil
}

// --- Mapping helpers ---

// mapPriceType converts a broker.OrderType to the E*TRADE priceType string.
func mapPriceType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "MARKET"
	case broker.Limit:
		return "LIMIT"
	case broker.Stop:
		return "STOP"
	case broker.StopLimit:
		return "STOP_LIMIT"
	default:
		return "MARKET"
	}
}

// mapOrderTerm converts a broker.TimeInForce to the E*TRADE orderTerm string.
// OnOpen and OnClose are not supported as order terms -- the caller must handle
// them via a priceType override (MARKET_ON_OPEN / MARKET_ON_CLOSE).
func mapOrderTerm(tif broker.TimeInForce) (string, error) {
	switch tif {
	case broker.Day:
		return "GOOD_FOR_DAY", nil
	case broker.GTC:
		return "GOOD_UNTIL_CANCEL", nil
	case broker.GTD:
		return "GOOD_TILL_DATE", nil
	case broker.IOC:
		return "IMMEDIATE_OR_CANCEL", nil
	case broker.FOK:
		return "FILL_OR_KILL", nil
	case broker.OnOpen:
		return "", fmt.Errorf("etrade: OnOpen time-in-force must be handled via priceType override")
	case broker.OnClose:
		return "", fmt.Errorf("etrade: OnClose time-in-force must be handled via priceType override")
	default:
		return "GOOD_FOR_DAY", nil
	}
}

// mapOrderAction converts a broker.Side to the E*TRADE orderAction string.
func mapOrderAction(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "BUY"
	case broker.Sell:
		return "SELL"
	default:
		return "BUY"
	}
}

// mapOrderStatus converts an E*TRADE order status string to a broker.OrderStatus.
func mapOrderStatus(status string) broker.OrderStatus {
	switch status {
	case "OPEN":
		return broker.OrderOpen
	case "EXECUTED":
		return broker.OrderFilled
	case "CANCELLED", "CANCEL_REQUESTED":
		return broker.OrderCancelled
	case "PARTIAL", "INDIVIDUAL_FILLS":
		return broker.OrderPartiallyFilled
	default:
		return broker.OrderSubmitted
	}
}

// unmapPriceType converts an E*TRADE priceType string to a broker.OrderType.
func unmapPriceType(priceType string) broker.OrderType {
	switch priceType {
	case "MARKET":
		return broker.Market
	case "LIMIT":
		return broker.Limit
	case "STOP":
		return broker.Stop
	case "STOP_LIMIT":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

// unmapOrderAction converts an E*TRADE orderAction string to a broker.Side.
func unmapOrderAction(action string) broker.Side {
	switch action {
	case "BUY", "BUY_TO_COVER":
		return broker.Buy
	case "SELL", "SELL_SHORT":
		return broker.Sell
	default:
		return broker.Buy
	}
}

// formatDate formats a time.Time as MMDDYYYY.
func formatDate(tt time.Time) string {
	return tt.Format("01022006")
}

// parseDate parses a date string in MMDDYYYY format.
func parseDate(ss string) (time.Time, error) {
	tt, err := time.Parse("01022006", ss)
	if err != nil {
		return time.Time{}, fmt.Errorf("etrade: parse date %q: %w", ss, err)
	}

	return tt, nil
}
