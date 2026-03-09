package portfolio_test

import (
	"time"

	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// Compile-time interface checks.
var _ portfolio.Portfolio = (*portfolio.Account)(nil)
var _ portfolio.PortfolioManager = (*portfolio.Account)(nil)
var _ portfolio.Selector = portfolio.MaxAboveZero(nil)

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
