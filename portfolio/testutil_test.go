package portfolio_test

import (
	"time"

	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// Compile-time interface checks.
var _ portfolio.Selector = portfolio.MaxAboveZero(data.MetricClose, nil)
var _ portfolio.Selector = portfolio.TopN(1, data.MetricClose)
var _ portfolio.Selector = portfolio.BottomN(1, data.MetricClose)

// buildDF builds a single-timestamp DataFrame with MetricClose and AdjClose
// for the given assets. closes and adjCloses must have the same length as assets.
func buildDF(t time.Time, assets []asset.Asset, closes, adjCloses []float64) *data.DataFrame {
	cols := make([][]float64, 0, len(assets)*2)
	for i := range assets {
		cols = append(cols, []float64{closes[i]})
		cols = append(cols, []float64{adjCloses[i]})
	}
	df, err := data.NewDataFrame(
		[]time.Time{t},
		assets,
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		cols,
	)
	Expect(err).NotTo(HaveOccurred())
	return df
}

// daySeq returns n weekday timestamps starting from start.
// Weekends are skipped.
func daySeq(start time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	d := start
	for len(out) < n {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			out = append(out, d)
		}
		d = d.AddDate(0, 0, 1)
	}
	return out
}

// buildAccountFromEquity creates an Account whose equity curve matches the
// given values exactly using dividend/fee transactions (invisible to TWRR's
// cash flow filter, which only considers deposits/withdrawals).
func buildAccountFromEquity(equityValues []float64) *portfolio.Account {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	aa := portfolio.New(portfolio.WithCash(equityValues[0], time.Time{}))
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	dates := daySeq(start, len(equityValues))

	for ii, vv := range equityValues {
		if ii > 0 {
			diff := vv - equityValues[ii-1]
			if diff > 0 {
				aa.Record(portfolio.Transaction{
					Date:   dates[ii],
					Type:   asset.DividendTransaction,
					Amount: diff,
				})
			} else if diff < 0 {
				aa.Record(portfolio.Transaction{
					Date:   dates[ii],
					Type:   asset.FeeTransaction,
					Amount: diff,
				})
			}
		}
		df := buildDF(dates[ii], []asset.Asset{spy}, []float64{450}, []float64{448})
		aa.UpdatePrices(df)
	}

	return aa
}

// buildAccountWithRF creates an Account with both equity curve and
// risk-free (BIL) prices, needed for metrics that use excess returns.
func buildAccountWithRF(spyPrices, bilPrices []float64) *portfolio.Account {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	n := len(spyPrices)
	times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

	acct := portfolio.New(
		portfolio.WithCash(5*spyPrices[0], time.Time{}),
		portfolio.WithBenchmark(spy),
	)

	acct.Record(portfolio.Transaction{
		Date:   times[0],
		Asset:  spy,
		Type:   asset.BuyTransaction,
		Qty:    5,
		Price:  spyPrices[0],
		Amount: -5 * spyPrices[0],
	})

	for i := range n {
		acct.SetRiskFreeValue(bilPrices[i])
		df := buildDF(times[i],
			[]asset.Asset{spy},
			[]float64{spyPrices[i]},
			[]float64{spyPrices[i]},
		)
		acct.UpdatePrices(df)
	}

	return acct
}
