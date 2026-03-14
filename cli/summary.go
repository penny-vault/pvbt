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

func printSummary(acct portfolio.Portfolio) {
	s, _ := acct.Summary()
	r, _ := acct.RiskMetrics()
	t, _ := acct.TradeMetrics()
	w, _ := acct.WithdrawalMetrics()

	var sb strings.Builder

	sb.WriteString(headerStyle.Render("Backtest Results"))
	sb.WriteString("\n\n")

	// Returns section
	sb.WriteString(sectionStyle.Render(renderSection("Returns", []row{
		{"TWRR", fmtPct(s.TWRR)},
		{"MWRR", fmtPct(s.MWRR)},
		{"Sharpe", fmtRatio(s.Sharpe)},
		{"Sortino", fmtRatio(s.Sortino)},
		{"Calmar", fmtRatio(s.Calmar)},
		{"Max Drawdown", fmtPct(s.MaxDrawdown)},
		{"Std Dev", fmtPct(s.StdDev)},
	})))

	// Risk section
	sb.WriteString(sectionStyle.Render(renderSection("Risk", []row{
		{"Beta", fmtRatio(r.Beta)},
		{"Alpha", fmtPct(r.Alpha)},
		{"Tracking Error", fmtPct(r.TrackingError)},
		{"Downside Deviation", fmtPct(r.DownsideDeviation)},
		{"Information Ratio", fmtRatio(r.InformationRatio)},
		{"Treynor", fmtRatio(r.Treynor)},
		{"Ulcer Index", fmtRatio(r.UlcerIndex)},
		{"Excess Kurtosis", fmtRatio(r.ExcessKurtosis)},
		{"Skewness", fmtRatio(r.Skewness)},
		{"R-Squared", fmtRatio(r.RSquared)},
		{"Value at Risk", fmtPct(r.ValueAtRisk)},
		{"Upside Capture", fmtPct(r.UpsideCaptureRatio)},
		{"Downside Capture", fmtPct(r.DownsideCaptureRatio)},
	})))

	// Trading section
	sb.WriteString(sectionStyle.Render(renderSection("Trading", []row{
		{"Win Rate", fmtPct(t.WinRate)},
		{"Average Win", fmtCurrency(t.AverageWin)},
		{"Average Loss", fmtCurrency(t.AverageLoss)},
		{"Profit Factor", fmtRatio(t.ProfitFactor)},
		{"Avg Holding Period", fmt.Sprintf("%.0f days", t.AverageHoldingPeriod)},
		{"Turnover", fmtPct(t.Turnover)},
		{"Positive Periods", fmtPct(t.NPositivePeriods)},
		{"Gain/Loss Ratio", fmtRatio(t.GainLossRatio)},
	})))

	// Withdrawals section
	sb.WriteString(sectionStyle.Render(renderSection("Withdrawals", []row{
		{"Safe Withdrawal Rate", fmtPct(w.SafeWithdrawalRate)},
		{"Perpetual Rate", fmtPct(w.PerpetualWithdrawalRate)},
		{"Dynamic Rate", fmtPct(w.DynamicWithdrawalRate)},
	})))

	fmt.Fprint(os.Stdout, sb.String())
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
