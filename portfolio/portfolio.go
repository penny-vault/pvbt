package portfolio

import "time"

type Holding struct {
	Ticker     string
	SharesHeld float64
}

const (
	SellTransaction = "SELL"
	BuyTransaction  = "BUY"
)

type Transaction struct {
	Date          time.Time
	Ticker        string
	Kind          string
	PricePerShare float64
	Shares        float64
	TotalValue    float64
}

// Portfolio manage a portfolio
type Portfolio struct {
	Name         string
	StartTime    time.Time
	EndTime      time.Time
	Transactions []Transaction
}
