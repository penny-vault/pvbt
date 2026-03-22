package schwab

import (
	"fmt"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Schwab API request/response types ---

type schwabOrderRequest struct {
	OrderType            string                `json:"orderType"`
	Session              string                `json:"session"`
	Duration             string                `json:"duration"`
	OrderStrategyType    string                `json:"orderStrategyType"`
	Price                float64               `json:"price,omitempty"`
	StopPrice            float64               `json:"stopPrice,omitempty"`
	TaxLotMethod         string                `json:"taxLotMethod,omitempty"`
	OrderLegCollection   []schwabOrderLegEntry `json:"orderLegCollection"`
	ChildOrderStrategies []schwabOrderRequest  `json:"childOrderStrategies,omitempty"`
}

type schwabOrderLegEntry struct {
	Instruction string           `json:"instruction"`
	Quantity    float64          `json:"quantity"`
	Instrument  schwabInstrument `json:"instrument"`
}

type schwabInstrument struct {
	Symbol    string `json:"symbol"`
	AssetType string `json:"assetType"`
}

type schwabOrderResponse struct {
	OrderID                 int64                 `json:"orderId"`
	Status                  string                `json:"status"`
	OrderType               string                `json:"orderType"`
	Price                   float64               `json:"price"`
	StopPrice               float64               `json:"stopPrice"`
	Duration                string                `json:"duration"`
	OrderStrategyType       string                `json:"orderStrategyType"`
	OrderLegCollection      []schwabOrderLeg      `json:"orderLegCollection"`
	OrderActivityCollection []schwabOrderActivity `json:"orderActivityCollection"`
}

type schwabOrderLeg struct {
	Instruction string           `json:"instruction"`
	Quantity    float64          `json:"quantity"`
	Instrument  schwabInstrument `json:"instrument"`
}

type schwabOrderActivity struct {
	ActivityType  string               `json:"activityType"`
	ExecutionType string               `json:"executionType"`
	Quantity      float64              `json:"quantity"`
	ExecutionLegs []schwabExecutionLeg `json:"executionLegs"`
}

type schwabExecutionLeg struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Time     string  `json:"time"`
}

type schwabAccountNumberEntry struct {
	AccountNumber string `json:"accountNumber"`
	HashValue     string `json:"hashValue"`
}

type schwabAccountResponse struct {
	SecuritiesAccount schwabSecuritiesAccount `json:"securitiesAccount"`
}

type schwabSecuritiesAccount struct {
	AccountNumber   string                `json:"accountNumber"`
	Type            string                `json:"type"`
	Positions       []schwabPositionEntry `json:"positions"`
	CurrentBalances schwabBalances        `json:"currentBalances"`
}

type schwabPositionEntry struct {
	Instrument           schwabInstrument `json:"instrument"`
	LongQuantity         float64          `json:"longQuantity"`
	ShortQuantity        float64          `json:"shortQuantity"`
	AveragePrice         float64          `json:"averagePrice"`
	MarketValue          float64          `json:"marketValue"`
	CurrentDayProfitLoss float64          `json:"currentDayProfitLoss"`
}

type schwabBalances struct {
	CashBalance            float64 `json:"cashBalance"`
	Equity                 float64 `json:"equity"`
	BuyingPower            float64 `json:"buyingPower"`
	MaintenanceRequirement float64 `json:"maintenanceRequirement"`
}

type schwabQuoteResponse struct {
	Quote schwabQuote `json:"quote"`
}

type schwabQuote struct {
	LastPrice float64 `json:"lastPrice"`
}

type schwabUserPreference struct {
	StreamerInfo []schwabStreamerInfo `json:"streamerInfo"`
}

type schwabStreamerInfo struct {
	StreamerSocketURL      string `json:"streamerSocketUrl"`
	SchwabClientCustomerID string `json:"schwabClientCustomerId"`
	SchwabClientCorrelID   string `json:"schwabClientCorrelId"`
	SchwabClientChannel    string `json:"schwabClientChannel"`
	SchwabClientFunctionID string `json:"schwabClientFunctionId"`
}

type schwabTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// --- Translation functions ---

func toSchwabOrder(order broker.Order) (schwabOrderRequest, error) {
	duration, tifErr := mapTimeInForce(order.TimeInForce)
	if tifErr != nil {
		return schwabOrderRequest{}, tifErr
	}

	return schwabOrderRequest{
		OrderType:         mapOrderType(order.OrderType),
		Session:           "NORMAL",
		Duration:          duration,
		OrderStrategyType: "SINGLE",
		Price:             order.LimitPrice,
		StopPrice:         order.StopPrice,
		TaxLotMethod:      mapLotSelection(order.LotSelection),
		OrderLegCollection: []schwabOrderLegEntry{
			{
				Instruction: mapSide(order.Side),
				Quantity:    order.Qty,
				Instrument: schwabInstrument{
					Symbol:    order.Asset.Ticker,
					AssetType: "EQUITY",
				},
			},
		},
	}, nil
}

func toBrokerOrder(resp schwabOrderResponse) broker.Order {
	order := broker.Order{
		ID:         fmt.Sprintf("%d", resp.OrderID),
		Status:     mapSchwabStatus(resp.Status),
		OrderType:  mapSchwabOrderType(resp.OrderType),
		LimitPrice: resp.Price,
		StopPrice:  resp.StopPrice,
	}

	if len(resp.OrderLegCollection) > 0 {
		leg := resp.OrderLegCollection[0]
		order.Asset = asset.Asset{Ticker: leg.Instrument.Symbol}
		order.Qty = leg.Quantity
		order.Side = mapSchwabSide(leg.Instruction)
	}

	return order
}

func toBrokerPosition(resp schwabPositionEntry) broker.Position {
	qty := resp.LongQuantity
	if qty == 0 {
		qty = -resp.ShortQuantity
	}

	markPrice := 0.0
	if qty != 0 {
		markPrice = resp.MarketValue / qty
	}

	return broker.Position{
		Asset:         asset.Asset{Ticker: resp.Instrument.Symbol},
		Qty:           qty,
		AvgOpenPrice:  resp.AveragePrice,
		MarkPrice:     markPrice,
		RealizedDayPL: resp.CurrentDayProfitLoss,
	}
}

func toBrokerBalance(resp schwabAccountResponse) broker.Balance {
	balances := resp.SecuritiesAccount.CurrentBalances

	return broker.Balance{
		CashBalance:         balances.CashBalance,
		NetLiquidatingValue: balances.Equity,
		EquityBuyingPower:   balances.BuyingPower,
		MaintenanceReq:      balances.MaintenanceRequirement,
	}
}

// --- Mapping helpers ---

func mapOrderType(orderType broker.OrderType) string {
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

func mapSchwabOrderType(orderType string) broker.OrderType {
	switch orderType {
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

func mapSide(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "BUY"
	case broker.Sell:
		return "SELL"
	default:
		return "BUY"
	}
}

func mapSchwabSide(instruction string) broker.Side {
	switch instruction {
	case "BUY", "BUY_TO_COVER":
		return broker.Buy
	case "SELL", "SELL_SHORT":
		return broker.Sell
	default:
		return broker.Buy
	}
}

func mapTimeInForce(tif broker.TimeInForce) (string, error) {
	switch tif {
	case broker.Day:
		return "DAY", nil
	case broker.GTC:
		return "GOOD_TILL_CANCEL", nil
	case broker.IOC:
		return "IMMEDIATE_OR_CANCEL", nil
	case broker.FOK:
		return "FILL_OR_KILL", nil
	case broker.GTD:
		return "", fmt.Errorf("schwab: GTD time-in-force is not supported for equities")
	case broker.OnOpen:
		return "", fmt.Errorf("schwab: OnOpen time-in-force is not supported")
	case broker.OnClose:
		return "", fmt.Errorf("schwab: OnClose time-in-force is not supported")
	default:
		return "DAY", nil
	}
}

func mapSchwabStatus(status string) broker.OrderStatus {
	switch status {
	case "NEW", "AWAITING_PARENT_ORDER", "AWAITING_CONDITION", "AWAITING_STOP_CONDITION",
		"AWAITING_MANUAL_REVIEW", "ACCEPTED", "PENDING_ACTIVATION", "QUEUED",
		"PENDING_ACKNOWLEDGEMENT":
		return broker.OrderSubmitted
	case "WORKING":
		return broker.OrderOpen
	case "FILLED":
		return broker.OrderFilled
	case "PENDING_CANCEL", "CANCELED", "REJECTED", "EXPIRED", "REPLACED",
		"PENDING_REPLACE", "PENDING_RECALL":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

func mapLotSelection(lotSelection int) string {
	switch lotSelection {
	case 0:
		return "FIFO"
	case 1:
		return "LIFO"
	case 2:
		return "HIGH_COST"
	case 3:
		return "SPECIFIC_LOT"
	default:
		return "FIFO"
	}
}

// --- Bracket/OCO order builders ---

func buildBracketOrder(orders []broker.Order) (schwabOrderRequest, error) {
	var entryOrder *schwabOrderRequest

	var contingentOrders []schwabOrderRequest

	for _, order := range orders {
		schwabOrd, tifErr := toSchwabOrder(order)
		if tifErr != nil {
			return schwabOrderRequest{}, tifErr
		}

		if order.GroupRole == broker.RoleEntry {
			if entryOrder != nil {
				return schwabOrderRequest{}, broker.ErrMultipleEntryOrders
			}

			schwabOrd.OrderStrategyType = "TRIGGER"
			entryOrder = &schwabOrd
		} else {
			contingentOrders = append(contingentOrders, schwabOrd)
		}
	}

	if entryOrder == nil {
		return schwabOrderRequest{}, broker.ErrNoEntryOrder
	}

	ocoChild := schwabOrderRequest{
		OrderStrategyType:    "OCO",
		ChildOrderStrategies: contingentOrders,
	}

	entryOrder.ChildOrderStrategies = []schwabOrderRequest{ocoChild}

	return *entryOrder, nil
}

func buildOCOOrder(orders []broker.Order) (schwabOrderRequest, error) {
	children := make([]schwabOrderRequest, len(orders))

	for idx, order := range orders {
		schwabOrd, tifErr := toSchwabOrder(order)
		if tifErr != nil {
			return schwabOrderRequest{}, tifErr
		}

		children[idx] = schwabOrd
	}

	return schwabOrderRequest{
		OrderStrategyType:    "OCO",
		ChildOrderStrategies: children,
	}, nil
}
