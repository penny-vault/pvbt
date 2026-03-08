package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

func writePortfolioJSONL(path, runID, strategy string, start, end time.Time, cash float64, acct *portfolio.Account) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create portfolio file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)

	// metadata line
	meta := map[string]any{
		"type":     "metadata",
		"run_id":   runID,
		"strategy": strategy,
		"start":    start.Format("2006-01-02"),
		"end":      end.Format("2006-01-02"),
		"cash":     cash,
	}
	if err := enc.Encode(meta); err != nil {
		return fmt.Errorf("write portfolio metadata: %w", err)
	}

	// TODO: iterate per-step portfolio snapshots when Account supports time-series history
	// For now, write a single summary line with current values
	snapshot := map[string]any{
		"date":  end.Format("2006-01-02"),
		"value": acct.Value(),
		"cash":  acct.Cash(),
	}
	if err := enc.Encode(snapshot); err != nil {
		return fmt.Errorf("write portfolio snapshot: %w", err)
	}

	return nil
}

func writeTransactionsJSONL(path string, acct *portfolio.Account) error {
	txns := acct.Transactions()
	if len(txns) == 0 {
		return nil
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create transactions file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, tx := range txns {
		action := "unknown"
		switch tx.Type {
		case portfolio.BuyTransaction:
			action = "buy"
		case portfolio.SellTransaction:
			action = "sell"
		case portfolio.DividendTransaction:
			action = "dividend"
		case portfolio.FeeTransaction:
			action = "fee"
		case portfolio.DepositTransaction:
			action = "deposit"
		case portfolio.WithdrawalTransaction:
			action = "withdrawal"
		}

		rec := map[string]any{
			"date":     tx.Date.Format("2006-01-02"),
			"action":   action,
			"ticker":   tx.Asset.Ticker,
			"figi":     tx.Asset.CompositeFigi,
			"quantity": tx.Qty,
			"price":    tx.Price,
			"total":    tx.Amount,
		}
		if err := enc.Encode(rec); err != nil {
			return fmt.Errorf("write transaction: %w", err)
		}
	}

	return nil
}

func writeHoldingsJSONL(path string, acct *portfolio.Account) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create holdings file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)

	type holding struct {
		Ticker string  `json:"ticker"`
		Figi   string  `json:"figi"`
		Qty    float64 `json:"quantity"`
	}

	var holdings []holding
	acct.Holdings(func(a asset.Asset, qty float64) {
		holdings = append(holdings, holding{
			Ticker: a.Ticker,
			Figi:   a.CompositeFigi,
			Qty:    qty,
		})
	})

	rec := map[string]any{
		"date":     time.Now().Format("2006-01-02"),
		"holdings": holdings,
	}
	if err := enc.Encode(rec); err != nil {
		return fmt.Errorf("write holdings: %w", err)
	}

	return nil
}

func writeMetricsJSONL(path string, acct *portfolio.Account) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create metrics file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)

	// metadata line
	meta := map[string]any{
		"type":   "metadata",
		"groups": []string{"summary", "risk", "trade", "withdrawal"},
	}
	if err := enc.Encode(meta); err != nil {
		return fmt.Errorf("write metrics metadata: %w", err)
	}

	s := acct.Summary()
	r := acct.RiskMetrics()
	t := acct.TradeMetrics()
	w := acct.WithdrawalMetrics()

	rec := map[string]any{
		"date": time.Now().Format("2006-01-02"),
		"summary": map[string]float64{
			"twrr":         s.TWRR,
			"mwrr":         s.MWRR,
			"sharpe":       s.Sharpe,
			"sortino":      s.Sortino,
			"calmar":       s.Calmar,
			"max_drawdown": s.MaxDrawdown,
			"std_dev":      s.StdDev,
		},
		"risk": map[string]float64{
			"beta":               r.Beta,
			"alpha":              r.Alpha,
			"tracking_error":     r.TrackingError,
			"downside_deviation": r.DownsideDeviation,
			"information_ratio":  r.InformationRatio,
			"treynor":            r.Treynor,
			"ulcer_index":        r.UlcerIndex,
			"excess_kurtosis":    r.ExcessKurtosis,
			"skewness":           r.Skewness,
			"r_squared":          r.RSquared,
			"value_at_risk":      r.ValueAtRisk,
			"upside_capture":     r.UpsideCaptureRatio,
			"downside_capture":   r.DownsideCaptureRatio,
		},
		"trade": map[string]float64{
			"win_rate":               t.WinRate,
			"average_win":            t.AverageWin,
			"average_loss":           t.AverageLoss,
			"profit_factor":          t.ProfitFactor,
			"average_holding_period": t.AverageHoldingPeriod,
			"turnover":               t.Turnover,
			"n_positive_periods":     t.NPositivePeriods,
			"gain_loss_ratio":        t.GainLossRatio,
		},
		"withdrawal": map[string]float64{
			"safe_withdrawal_rate":      w.SafeWithdrawalRate,
			"perpetual_withdrawal_rate": w.PerpetualWithdrawalRate,
			"dynamic_withdrawal_rate":   w.DynamicWithdrawalRate,
		},
	}
	if err := enc.Encode(rec); err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}

	return nil
}
