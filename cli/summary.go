package cli

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/portfolio"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			MarginBottom(1)

	labelStyle = lipgloss.NewStyle().
			Width(26).
			Foreground(lipgloss.Color("7"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true)

	sectionStyle = lipgloss.NewStyle().
			MarginBottom(1)
)

func printSummary(acct portfolio.Portfolio) error {
	summary, err := acct.Summary()
	if err != nil {
		return fmt.Errorf("computing summary metrics: %w", err)
	}

	risk, err := acct.RiskMetrics()
	if err != nil {
		return fmt.Errorf("computing risk metrics: %w", err)
	}

	trade, err := acct.TradeMetrics()
	if err != nil {
		return fmt.Errorf("computing trade metrics: %w", err)
	}

	withdrawal, err := acct.WithdrawalMetrics()
	if err != nil {
		return fmt.Errorf("computing withdrawal metrics: %w", err)
	}

	var sb strings.Builder

	sb.WriteString(headerStyle.Render("Backtest Results"))
	sb.WriteString("\n\n")

	// Returns section
	sb.WriteString(sectionStyle.Render(renderSection("Returns", []row{
		{"TWRR", fmtPct(summary.TWRR)},
		{"MWRR", fmtPct(summary.MWRR)},
		{"Sharpe", fmtRatio(summary.Sharpe)},
		{"Sortino", fmtRatio(summary.Sortino)},
		{"Calmar", fmtRatio(summary.Calmar)},
		{"Max Drawdown", fmtPct(summary.MaxDrawdown)},
		{"Std Dev", fmtPct(summary.StdDev)},
	})))

	// Risk section
	sb.WriteString(sectionStyle.Render(renderSection("Risk", []row{
		{"Beta", fmtRatio(risk.Beta)},
		{"Alpha", fmtPct(risk.Alpha)},
		{"Tracking Error", fmtPct(risk.TrackingError)},
		{"Downside Deviation", fmtPct(risk.DownsideDeviation)},
		{"Information Ratio", fmtRatio(risk.InformationRatio)},
		{"Treynor", fmtRatio(risk.Treynor)},
		{"Ulcer Index", fmtRatio(risk.UlcerIndex)},
		{"Excess Kurtosis", fmtRatio(risk.ExcessKurtosis)},
		{"Skewness", fmtRatio(risk.Skewness)},
		{"R-Squared", fmtRatio(risk.RSquared)},
		{"Value at Risk", fmtPct(risk.ValueAtRisk)},
		{"Upside Capture", fmtPct(risk.UpsideCaptureRatio)},
		{"Downside Capture", fmtPct(risk.DownsideCaptureRatio)},
	})))

	// Trading section
	sb.WriteString(sectionStyle.Render(renderSection("Trading", []row{
		{"Win Rate", fmtPct(trade.WinRate)},
		{"Average Win", fmtCurrency(trade.AverageWin)},
		{"Average Loss", fmtCurrency(trade.AverageLoss)},
		{"Profit Factor", fmtRatio(trade.ProfitFactor)},
		{"Avg Holding Period", fmt.Sprintf("%.0f days", trade.AverageHoldingPeriod)},
		{"Turnover", fmtPct(trade.Turnover)},
		{"Positive Periods", fmtPct(trade.NPositivePeriods)},
		{"Gain/Loss Ratio", fmtRatio(trade.GainLossRatio)},
	})))

	// Withdrawals section
	sb.WriteString(sectionStyle.Render(renderSection("Withdrawals", []row{
		{"Safe Withdrawal Rate", fmtPct(withdrawal.SafeWithdrawalRate)},
		{"Perpetual Rate", fmtPct(withdrawal.PerpetualWithdrawalRate)},
		{"Dynamic Rate", fmtPct(withdrawal.DynamicWithdrawalRate)},
	})))

	fmt.Fprint(os.Stdout, sb.String())

	return nil
}

type row struct {
	label string
	value string
}

func renderSection(title string, rows []row) string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(title))
	sb.WriteString("\n")

	for _, r := range rows {
		sb.WriteString(labelStyle.Render(r.label))
		sb.WriteString(valueStyle.Render(r.value))
		sb.WriteString("\n")
	}

	return sb.String()
}

func fmtPct(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}

	return fmt.Sprintf("%.2f%%", v*100)
}

func fmtCurrency(v float64) string {
	return fmt.Sprintf("$%.2f", v)
}

func fmtRatio(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}

	return fmt.Sprintf("%.3f", v)
}
