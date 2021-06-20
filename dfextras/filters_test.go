package dfextras_test

import (
	"main/data"
	"main/dfextras"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rocketlaunchr/dataframe-go"
)

var _ = Describe("Filters", func() {
	var (
		df1 *dataframe.DataFrame
	)

	BeforeEach(func() {
		series1 := dataframe.NewSeriesFloat64("col1", &dataframe.SeriesInit{Size: 4}, []float64{1.0, 2.0, 3.0, 4.0, 5.0})
		tSeries1 := dataframe.NewSeriesTime(data.DateIdx, &dataframe.SeriesInit{Size: 4}, []time.Time{
			time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, time.February, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, time.March, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, time.April, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, time.May, 1, 0, 0, 0, 0, time.UTC),
		})
		df1 = dataframe.NewDataFrame(tSeries1, series1)
	})

	Describe("When computing the SMA", func() {
		Context("with 5 values and lookback of 2 months", func() {
			It("should have a response with 4 values", func() {
				sma, err := dfextras.SMA(2, df1)
				Expect(err).To(BeNil())
				Expect(sma.NRows()).To(Equal(4))

				// Confirm that the timeAxis has all the expected values
				timeAxisIdx, err := sma.NameToColumn(data.DateIdx)
				timeAxis := sma.Series[timeAxisIdx]
				Expect(err).To(BeNil())
				Expect(timeAxis.Value(0).(time.Time)).Should(Equal(time.Date(2021, time.February, 1, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(1).(time.Time)).Should(Equal(time.Date(2021, time.March, 1, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(2).(time.Time)).Should(Equal(time.Date(2021, time.April, 1, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(3).(time.Time)).Should(Equal(time.Date(2021, time.May, 1, 0, 0, 0, 0, time.UTC)))

				// Confirm that col1 has all the expected values
				col1Idx, err := sma.NameToColumn("col1_SMA")
				col1 := sma.Series[col1Idx]
				Expect(err).To(BeNil())
				Expect(col1.Value(0)).Should(Equal(1.5))
				Expect(col1.Value(1)).Should(Equal(2.5))
				Expect(col1.Value(2)).Should(Equal(3.5))
				Expect(col1.Value(3)).Should(Equal(4.5))
			})
		})

		Context("with 5 values and lookback of 3 months", func() {
			It("should have a response with 3 values", func() {
				sma, err := dfextras.SMA(3, df1)
				Expect(err).To(BeNil())
				Expect(sma.NRows()).To(Equal(3))

				// Confirm that the timeAxis has all the expected values
				timeAxisIdx, err := sma.NameToColumn(data.DateIdx)
				timeAxis := sma.Series[timeAxisIdx]
				Expect(err).To(BeNil())
				Expect(timeAxis.Value(0).(time.Time)).Should(Equal(time.Date(2021, time.March, 1, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(1).(time.Time)).Should(Equal(time.Date(2021, time.April, 1, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(2).(time.Time)).Should(Equal(time.Date(2021, time.May, 1, 0, 0, 0, 0, time.UTC)))

				// Confirm that col1 has all the expected values
				col1Idx, err := sma.NameToColumn("col1_SMA")
				col1 := sma.Series[col1Idx]
				Expect(err).To(BeNil())
				Expect(col1.Value(0)).Should(Equal(2.0))
				Expect(col1.Value(1)).Should(Equal(3.0))
				Expect(col1.Value(2)).Should(Equal(4.0))
			})
		})

		Context("with 5 values and lookback of 5 months", func() {
			It("should have a response with 1 values", func() {
				sma, err := dfextras.SMA(5, df1)
				Expect(err).To(BeNil())
				Expect(sma.NRows()).To(Equal(1))

				// Confirm that the timeAxis has all the expected values
				timeAxisIdx, err := sma.NameToColumn(data.DateIdx)
				timeAxis := sma.Series[timeAxisIdx]
				Expect(err).To(BeNil())
				Expect(timeAxis.Value(0).(time.Time)).Should(Equal(time.Date(2021, time.May, 1, 0, 0, 0, 0, time.UTC)))

				// Confirm that col1 has all the expected values
				col1Idx, err := sma.NameToColumn("col1_SMA")
				col1 := sma.Series[col1Idx]
				Expect(err).To(BeNil())
				Expect(col1.Value(0)).Should(Equal(3.0))
			})
		})

		Context("with 5 values and lookback of 6 months", func() {
			It("should return an error response", func() {
				sma, err := dfextras.SMA(6, df1)
				Expect(err).ToNot(BeNil())
				Expect(sma).To(BeNil())
			})
		})

		Context("with 5 values and lookback of 0 months", func() {
			It("should return an error response", func() {
				sma, err := dfextras.SMA(0, df1)
				Expect(err).ToNot(BeNil())
				Expect(sma).To(BeNil())
			})
		})
	})
})
