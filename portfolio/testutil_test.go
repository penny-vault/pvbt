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
	vals := make([]float64, 0, len(assets)*2)
	for i := range assets {
		vals = append(vals, closes[i])
		vals = append(vals, adjCloses[i])
	}
	df, err := data.NewDataFrame(
		[]time.Time{t},
		assets,
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		vals,
	)
	Expect(err).NotTo(HaveOccurred())
	return df
}

// buildMultiDF builds a multi-timestamp DataFrame with MetricClose and AdjClose.
// closeSeries and adjCloseSeries are indexed [time][asset].
// Data is arranged into column-major order as required by NewDataFrame.
func buildMultiDF(times []time.Time, assets []asset.Asset, closeSeries, adjCloseSeries [][]float64) *data.DataFrame {
	T := len(times)
	A := len(assets)
	// Column-major: each (asset, metric) column is T contiguous values.
	// Column order: (a0,close), (a0,adjClose), (a1,close), (a1,adjClose), ...
	vals := make([]float64, 0, T*A*2)
	for ai := range assets {
		// MetricClose column for this asset
		for ti := range times {
			vals = append(vals, closeSeries[ti][ai])
		}
		// AdjClose column for this asset
		for ti := range times {
			vals = append(vals, adjCloseSeries[ti][ai])
		}
	}
	df, err := data.NewDataFrame(
		times,
		assets,
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		vals,
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

// monthSeq returns n monthly timestamps starting from start,
// each one month apart.
func monthSeq(start time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	for i := range n {
		out = append(out, start.AddDate(0, i, 0))
	}
	return out
}

// buildAccountFromEquity creates an Account whose equity curve matches the
// given values exactly using deposit/withdrawal transactions.
func buildAccountFromEquity(equityValues []float64) *portfolio.Account {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	a := portfolio.New(portfolio.WithCash(equityValues[0], time.Time{}))
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	dates := daySeq(start, len(equityValues))

	for i, v := range equityValues {
		if i > 0 {
			diff := v - equityValues[i-1]
			if diff > 0 {
				a.Record(portfolio.Transaction{
					Date:   dates[i],
					Type:   portfolio.DepositTransaction,
					Amount: diff,
				})
			} else if diff < 0 {
				a.Record(portfolio.Transaction{
					Date:   dates[i],
					Type:   portfolio.WithdrawalTransaction,
					Amount: diff,
				})
			}
		}
		df := buildDF(dates[i], []asset.Asset{spy}, []float64{450}, []float64{448})
		a.UpdatePrices(df)
	}

	return a
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
		Type:   portfolio.BuyTransaction,
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
