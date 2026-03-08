package cli

import (
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/portfolio"
)

// Parquet record types

type portfolioRecord struct {
	Date             string  `parquet:"date"`
	Value            float64 `parquet:"value"`
	Cash             float64 `parquet:"cash"`
	Invested         float64 `parquet:"invested"`
	DailyReturn      float64 `parquet:"daily_return"`
	CumulativeReturn float64 `parquet:"cumulative_return"`
}

type transactionRecord struct {
	Date     string  `parquet:"date"`
	Action   string  `parquet:"action"`
	Ticker   string  `parquet:"ticker"`
	Figi     string  `parquet:"figi"`
	Quantity float64 `parquet:"quantity"`
	Price    float64 `parquet:"price"`
	Total    float64 `parquet:"total"`
}

type holdingEntry struct {
	Ticker   string  `parquet:"ticker"`
	Figi     string  `parquet:"figi"`
	Quantity float64 `parquet:"quantity"`
}

type metricRecord struct {
	Date               string  `parquet:"date"`
	TWRR               float64 `parquet:"twrr"`
	MWRR               float64 `parquet:"mwrr"`
	Sharpe             float64 `parquet:"sharpe"`
	Sortino            float64 `parquet:"sortino"`
	Calmar             float64 `parquet:"calmar"`
	MaxDrawdown        float64 `parquet:"max_drawdown"`
	StdDev             float64 `parquet:"std_dev"`
	Beta               float64 `parquet:"beta"`
	Alpha              float64 `parquet:"alpha"`
	WinRate            float64 `parquet:"win_rate"`
	ProfitFactor       float64 `parquet:"profit_factor"`
	SafeWithdrawalRate float64 `parquet:"safe_withdrawal_rate"`
}

func writePortfolioParquet(path, runID, strategy string, start, end time.Time, cash float64, params map[string]any, acct *portfolio.Account) error {
	// TODO: implement Parquet portfolio writer using parquet-go
	return fmt.Errorf("parquet output not yet implemented")
}

func writeTransactionsParquet(path string, acct *portfolio.Account) error {
	// TODO: implement Parquet transaction writer using parquet-go
	return fmt.Errorf("parquet output not yet implemented")
}

func writeHoldingsParquet(path string, acct *portfolio.Account) error {
	// TODO: implement Parquet holdings writer using parquet-go
	return fmt.Errorf("parquet output not yet implemented")
}

func writeMetricsParquet(path string, acct *portfolio.Account) error {
	// TODO: implement Parquet metrics writer using parquet-go
	return fmt.Errorf("parquet output not yet implemented")
}
