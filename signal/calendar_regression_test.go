package signal_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

// calendarDataSource emulates a real data provider: Fetch honors the
// requested lookback period against the current date and only returns the
// rows of the backing frame that fall inside [currentDate - lookback,
// currentDate]. Because the backing frame only has rows on trading days,
// a calendar-day lookback returns fewer bars than calendar days -- the
// behavior that hid the indicator lookback bugs behind period-ignoring mocks.
type calendarDataSource struct {
	currentDate time.Time
	frame       *data.DataFrame
}

func (c *calendarDataSource) Fetch(_ context.Context, _ []asset.Asset, lookback portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	start := lookback.Before(c.currentDate)
	return c.frame.Between(start, c.currentDate), nil
}

func (c *calendarDataSource) FetchAt(_ context.Context, _ []asset.Asset, t time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return c.frame.Between(t, t), nil
}

func (c *calendarDataSource) CurrentDate() time.Time { return c.currentDate }

// Compile-time check.
var _ data.DataSource = (*calendarDataSource)(nil)

// tradingTimes returns count timestamps on consecutive trading days
// (skipping Saturdays and Sundays) ending at end, in ascending order.
func tradingTimes(end time.Time, count int) []time.Time {
	times := make([]time.Time, count)
	tt := end

	for ii := count - 1; ii >= 0; ii-- {
		for tt.Weekday() == time.Saturday || tt.Weekday() == time.Sunday {
			tt = tt.AddDate(0, 0, -1)
		}

		times[ii] = tt
		tt = tt.AddDate(0, 0, -1)
	}

	return times
}

var _ = Describe("Indicators with weekend-skipping trading calendars", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		// A Friday, so any multi-day window spans at least one weekend.
		now = time.Date(2025, 6, 13, 16, 0, 0, 0, time.UTC)
	})

	Describe("StochasticFast", func() {
		It("computes over the requested trading bars despite weekends", func() {
			// 15 trading days spanning three weekends. The last 7 bars are the
			// hand-calculated series; the 8 earlier bars carry extreme values
			// that would corrupt the result if they leaked into the window.
			count := 15
			highs := make([]float64, count)
			lows := make([]float64, count)
			closes := make([]float64, count)

			for ii := range count - 7 {
				highs[ii] = 30
				lows[ii] = 1
				closes[ii] = 2
			}

			copy(highs[count-7:], []float64{12, 11, 13, 14, 12, 15, 13})
			copy(lows[count-7:], []float64{9, 8, 10, 11, 9, 10, 11})
			copy(closes[count-7:], []float64{10, 10, 12, 13, 11, 14, 12})

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
				data.Daily, [][]float64{highs, lows, closes})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			// With the old calendar-day fetch, Days(5) fetched 7 calendar days
			// spanning a weekend (5 trading bars) and always errored.
			result := signal.StochasticFast(ctx, uu, portfolio.Days(5))
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Value(aapl, signal.StochasticKSignal)).To(BeNumerically("~", 50.0, 1e-10))
			Expect(result.Value(aapl, signal.StochasticDSignal)).To(BeNumerically("~", (50.0+600.0/7.0+50.0)/3.0, 1e-10))
		})
	})

	Describe("StochasticSlow", func() {
		It("computes over the requested trading bars despite weekends", func() {
			// Last 7 bars are the tail of the hand-calculated series from the
			// StochasticSlow unit test; earlier bars are extreme values.
			count := 15
			highs := make([]float64, count)
			lows := make([]float64, count)
			closes := make([]float64, count)

			for ii := range count - 7 {
				highs[ii] = 30
				lows[ii] = 1
				closes[ii] = 2
			}

			copy(highs[count-7:], []float64{13, 14, 12, 15, 13, 16, 14})
			copy(lows[count-7:], []float64{10, 11, 9, 10, 11, 12, 10})
			copy(closes[count-7:], []float64{12, 13, 11, 14, 12, 15, 13})

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
				data.Daily, [][]float64{highs, lows, closes})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
			Expect(result.Err()).NotTo(HaveOccurred())

			expectedSlowK := (50.0 + 250.0/3.0 + 50.0) / 3.0
			Expect(result.Value(aapl, signal.StochasticSlowKSignal)).To(BeNumerically("~", expectedSlowK, 1e-6))

			slowK2 := (40.0 + 250.0/3.0 + 50.0) / 3.0
			slowK3 := (250.0/3.0 + 50.0 + 250.0/3.0) / 3.0
			expectedSlowD := (slowK2 + slowK3 + expectedSlowK) / 3.0
			Expect(result.Value(aapl, signal.StochasticSlowDSignal)).To(BeNumerically("~", expectedSlowD, 1e-6))
		})
	})

	Describe("MACD", func() {
		It("produces non-NaN values when the slow window spans weekends", func() {
			// 40 trading bars; a Days(26) calendar fetch only returns ~18
			// trading bars, so the old code emptied the frame and errored.
			count := 40
			prices := make([]float64, count)
			for ii := range prices {
				prices[ii] = 100 + float64(ii)*1.5
			}

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			result := signal.MACD(ctx, uu, portfolio.Days(12), portfolio.Days(26), portfolio.Days(9))
			Expect(result.Err()).NotTo(HaveOccurred())

			macdLine := result.Value(aapl, signal.MACDLineSignal)
			signalLine := result.Value(aapl, signal.MACDSignalLineSignal)
			histogram := result.Value(aapl, signal.MACDHistogramSignal)

			Expect(math.IsNaN(macdLine)).To(BeFalse())
			Expect(math.IsNaN(signalLine)).To(BeFalse())
			Expect(macdLine).To(BeNumerically(">", 0))
			Expect(histogram).To(BeNumerically("~", macdLine-signalLine, 1e-10))
		})
	})

	Describe("MFI", func() {
		buildOHLCV := func(count int) (highs, lows, closes, volumes []float64) {
			highs = make([]float64, count)
			lows = make([]float64, count)
			closes = make([]float64, count)
			volumes = make([]float64, count)

			for ii := range count {
				base := 100 + math.Sin(float64(ii)*0.7)*10
				highs[ii] = base + 2
				lows[ii] = base - 2
				closes[ii] = base + math.Cos(float64(ii))*1.5
				volumes[ii] = 1000 + float64(ii%7)*100
			}

			return highs, lows, closes, volumes
		}

		expectedMFI := func(highs, lows, closes, volumes []float64, startIdx int) float64 {
			tp := make([]float64, len(highs))
			for ii := range tp {
				tp[ii] = (highs[ii] + lows[ii] + closes[ii]) / 3.0
			}

			posFlow := 0.0
			negFlow := 0.0

			for ii := startIdx; ii < len(tp); ii++ {
				mf := tp[ii] * volumes[ii]
				if tp[ii] > tp[ii-1] {
					posFlow += mf
				} else if tp[ii] < tp[ii-1] {
					negFlow += mf
				}
			}

			return 100.0 - 100.0/(1.0+posFlow/negFlow)
		}

		It("uses period.N trading bars of money flow despite weekends", func() {
			count := 30
			highs, lows, closes, volumes := buildOHLCV(count)

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume},
				data.Daily, [][]float64{highs, lows, closes, volumes})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			result := signal.MFI(ctx, uu, portfolio.Days(14))
			Expect(result.Err()).NotTo(HaveOccurred())

			// Exactly the last 14 money flows (bars count-14 .. count-1
			// against their prior bar) should participate.
			expected := expectedMFI(highs, lows, closes, volumes, count-14)
			Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", expected, 1e-10))
		})

		It("preserves month-unit periods instead of coercing to days", func() {
			// 60 trading days ending 2025-06-13 reach back to mid-March, so
			// they fully cover the Months(2) window starting 2025-05-01.
			count := 60
			highs, lows, closes, volumes := buildOHLCV(count)

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume},
				data.Daily, [][]float64{highs, lows, closes, volumes})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			result := signal.MFI(ctx, uu, portfolio.Months(2))
			Expect(result.Err()).NotTo(HaveOccurred())

			// Months(2) at 2025-06-13 covers bars from 2025-05-01 onward,
			// with one extra bar before the window as the TP baseline.
			windowStart := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
			startIdx := 0

			for ii, tt := range times {
				if !tt.Before(windowStart) {
					startIdx = ii
					break
				}
			}

			expected := expectedMFI(highs, lows, closes, volumes, startIdx)
			Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", expected, 1e-10))
		})
	})

	Describe("RSI", func() {
		It("runs Wilder smoothing over history beyond the seed period", func() {
			count := 20
			prices := make([]float64, count)
			for ii := range prices {
				prices[ii] = 50 + math.Sin(float64(ii)*0.9)*5
			}

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			result := signal.RSI(ctx, uu, portfolio.Days(14))
			Expect(result.Err()).NotTo(HaveOccurred())

			// Reference implementation: SMA seed over the first 14 changes,
			// then Wilder smoothing over the remaining 5 changes.
			rsiPeriod := 14
			gains := make([]float64, 0, count-1)
			losses := make([]float64, 0, count-1)

			for ii := 1; ii < count; ii++ {
				change := prices[ii] - prices[ii-1]
				if change > 0 {
					gains = append(gains, change)
					losses = append(losses, 0)
				} else {
					gains = append(gains, 0)
					losses = append(losses, -change)
				}
			}

			avgGain := 0.0
			avgLoss := 0.0

			for ii := range rsiPeriod {
				avgGain += gains[ii]
				avgLoss += losses[ii]
			}

			avgGain /= float64(rsiPeriod)
			avgLoss /= float64(rsiPeriod)

			for ii := rsiPeriod; ii < len(gains); ii++ {
				avgGain = (avgGain*float64(rsiPeriod-1) + gains[ii]) / float64(rsiPeriod)
				avgLoss = (avgLoss*float64(rsiPeriod-1) + losses[ii]) / float64(rsiPeriod)
			}

			expected := 100 - 100/(1+avgGain/avgLoss)
			Expect(result.Value(aapl, signal.RSISignal)).To(BeNumerically("~", expected, 1e-10))
		})
	})

	Describe("ATR", func() {
		It("runs Wilder smoothing over history beyond the seed period", func() {
			count := 12
			highs := make([]float64, count)
			lows := make([]float64, count)
			closes := make([]float64, count)

			for ii := range count {
				base := 100 + math.Sin(float64(ii)*0.8)*6
				highs[ii] = base + 1 + float64(ii%3)
				lows[ii] = base - 1 - float64(ii%2)
				closes[ii] = base + 0.5
			}

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
				data.Daily, [][]float64{highs, lows, closes})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			atrPeriod := 4
			result := signal.ATR(ctx, uu, portfolio.Days(atrPeriod))
			Expect(result.Err()).NotTo(HaveOccurred())

			// Reference implementation: SMA seed over the first 4 true
			// ranges, Wilder smoothing over the remaining 7.
			trValues := make([]float64, count-1)
			for ii := 1; ii < count; ii++ {
				highLow := highs[ii] - lows[ii]
				highPrevClose := math.Abs(highs[ii] - closes[ii-1])
				lowPrevClose := math.Abs(lows[ii] - closes[ii-1])
				trValues[ii-1] = math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
			}

			avgTR := 0.0
			for ii := range atrPeriod {
				avgTR += trValues[ii]
			}

			avgTR /= float64(atrPeriod)

			for ii := atrPeriod; ii < len(trValues); ii++ {
				avgTR = (avgTR*float64(atrPeriod-1) + trValues[ii]) / float64(atrPeriod)
			}

			Expect(result.Value(aapl, signal.ATRSignal)).To(BeNumerically("~", avgTR, 1e-10))
		})
	})

	Describe("KeltnerChannels", func() {
		It("uses a true EMA center line and Wilder ATR with warm-up history", func() {
			count := 12
			highs := make([]float64, count)
			lows := make([]float64, count)
			closes := make([]float64, count)

			for ii := range count {
				base := 100 + math.Sin(float64(ii)*0.8)*6
				highs[ii] = base + 1 + float64(ii%3)
				lows[ii] = base - 1 - float64(ii%2)
				closes[ii] = base + 0.5
			}

			times := tradingTimes(now, count)
			df, err := data.NewDataFrame(times, []asset.Asset{aapl},
				[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
				data.Daily, [][]float64{highs, lows, closes})
			Expect(err).NotTo(HaveOccurred())

			ds := &calendarDataSource{currentDate: now, frame: df}
			uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			period := 4
			result := signal.KeltnerChannels(ctx, uu, portfolio.Days(period), 2.0)
			Expect(result.Err()).NotTo(HaveOccurred())

			// Reference EMA(4): SMA seed over the first 4 closes, then
			// exponential smoothing over the remaining 8.
			alpha := 2.0 / float64(period+1)
			ema := 0.0

			for ii := range period {
				ema += closes[ii]
			}

			ema /= float64(period)

			for ii := period; ii < count; ii++ {
				ema = alpha*closes[ii] + (1-alpha)*ema
			}

			// Reference Wilder ATR(4) over the 11 true ranges.
			trValues := make([]float64, count-1)
			for ii := 1; ii < count; ii++ {
				highLow := highs[ii] - lows[ii]
				highPrevClose := math.Abs(highs[ii] - closes[ii-1])
				lowPrevClose := math.Abs(lows[ii] - closes[ii-1])
				trValues[ii-1] = math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
			}

			avgTR := 0.0
			for ii := range period {
				avgTR += trValues[ii]
			}

			avgTR /= float64(period)

			for ii := period; ii < len(trValues); ii++ {
				avgTR = (avgTR*float64(period-1) + trValues[ii]) / float64(period)
			}

			Expect(result.Value(aapl, signal.KeltnerMiddleSignal)).To(BeNumerically("~", ema, 1e-10))
			Expect(result.Value(aapl, signal.KeltnerUpperSignal)).To(BeNumerically("~", ema+2*avgTR, 1e-10))
			Expect(result.Value(aapl, signal.KeltnerLowerSignal)).To(BeNumerically("~", ema-2*avgTR, 1e-10))
		})
	})
})

var _ = Describe("Pairs signals with mismatched histories", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		spy  asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		now = time.Date(2025, 6, 13, 16, 0, 0, 0, time.UTC)
	})

	buildFrames := func(primaryLen, refLen int) (*data.DataFrame, *data.DataFrame) {
		times := tradingTimes(now, primaryLen)

		primaryPrices := make([]float64, primaryLen)
		for ii := range primaryPrices {
			primaryPrices[ii] = 150 + float64(ii)*0.75 + math.Sin(float64(ii))*2
		}

		refPrices := make([]float64, refLen)
		for ii := range refPrices {
			refPrices[ii] = 100 + float64(ii)*0.5
		}

		primaryDF, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{primaryPrices})
		Expect(err).NotTo(HaveOccurred())

		refDF, err := data.NewDataFrame(times[primaryLen-refLen:], []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{refPrices})
		Expect(err).NotTo(HaveOccurred())

		return primaryDF, refDF
	}

	It("PairsRatio aligns on common timestamps instead of panicking", func() {
		primaryDF, refDF := buildFrames(20, 12)

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, &mockDataSource{currentDate: now, fetchResult: primaryDF})
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, &mockDataSource{currentDate: now, fetchResult: refDF})

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		zz := result.Value(aapl, "PairsRatio_SPY")
		Expect(math.IsNaN(zz)).To(BeFalse())
		Expect(math.IsInf(zz, 0)).To(BeFalse())
	})

	It("PairsResidual aligns on common timestamps instead of panicking", func() {
		primaryDF, refDF := buildFrames(20, 12)

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, &mockDataSource{currentDate: now, fetchResult: primaryDF})
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, &mockDataSource{currentDate: now, fetchResult: refDF})

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		zz := result.Value(aapl, "PairsResidual_SPY")
		Expect(math.IsNaN(zz)).To(BeFalse())
		Expect(math.IsInf(zz, 0)).To(BeFalse())
	})

	It("returns an error when the frames share no timestamps", func() {
		times := tradingTimes(now, 10)
		oldTimes := tradingTimes(now.AddDate(-2, 0, 0), 10)

		prices := make([]float64, 10)
		for ii := range prices {
			prices[ii] = 100 + float64(ii)
		}

		primaryDF, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		refDF, err := data.NewDataFrame(oldTimes, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, &mockDataSource{currentDate: now, fetchResult: primaryDF})
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, &mockDataSource{currentDate: now, fetchResult: refDF})

		ratio := signal.PairsRatio(ctx, primaryU, portfolio.Days(9), refU)
		Expect(ratio.Err()).To(HaveOccurred())
		Expect(ratio.Err().Error()).To(ContainSubstring("overlapping"))

		residual := signal.PairsResidual(ctx, primaryU, portfolio.Days(9), refU)
		Expect(residual.Err()).To(HaveOccurred())
		Expect(residual.Err().Error()).To(ContainSubstring("overlapping"))
	})
})
