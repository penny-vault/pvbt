package dfextras_test

import (
	"context"
	"main/data"
	"main/dfextras"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rocketlaunchr/dataframe-go"
)

var _ = Describe("Dfextras", func() {
	var (
		df1 *dataframe.DataFrame
		df2 *dataframe.DataFrame
		df3 *dataframe.DataFrame
	)

	BeforeEach(func() {
		series1 := dataframe.NewSeriesFloat64("col1", &dataframe.SeriesInit{Size: 5}, []float64{1.0, 2.0, 3.0})
		df1 = dataframe.NewDataFrame(series1)
		tSeries1 := dataframe.NewSeriesTime(data.DateIdx, &dataframe.SeriesInit{Size: 4}, []time.Time{
			time.Date(1982, time.July, 27, 0, 0, 0, 0, time.UTC),
			time.Date(1983, time.July, 27, 0, 0, 0, 0, time.UTC),
			time.Date(1984, time.July, 27, 0, 0, 0, 0, time.UTC),
			time.Date(1985, time.July, 27, 0, 0, 0, 0, time.UTC),
		})
		fSeries1 := dataframe.NewSeriesFloat64("col1", &dataframe.SeriesInit{Size: 4}, []float64{1.0, 2.0, 3.0, 4.0})
		tSeries2 := dataframe.NewSeriesTime(data.DateIdx, &dataframe.SeriesInit{Size: 4}, []time.Time{
			time.Date(1984, time.July, 27, 0, 0, 0, 0, time.UTC),
			time.Date(1985, time.July, 27, 0, 0, 0, 0, time.UTC),
			time.Date(1986, time.July, 27, 0, 0, 0, 0, time.UTC),
			time.Date(1987, time.July, 27, 0, 0, 0, 0, time.UTC),
		})
		fSeries2 := fSeries1.Copy()
		fSeries2.Rename("col2")
		df2 = dataframe.NewDataFrame(tSeries1, fSeries1)
		df3 = dataframe.NewDataFrame(tSeries2, fSeries2)
	})

	Describe("When given a dataframe", func() {
		Context("with float64 series containing NaN's", func() {
			It("should have no NaNs after DropNA is called", func() {
				tmp, err := dfextras.DropNA(context.TODO(), df1)
				df3 := tmp.(*dataframe.DataFrame)
				Expect(err).To(BeNil())
				Expect(df3.NRows()).To(Equal(3))
			})
		})
		Context("and merged with another dataframe", func() {
			It("should have times that cover the full range of both time axis'", func() {
				newDf, err := dfextras.Merge(context.TODO(), data.DateIdx, df2, df3)
				Expect(err).To(BeNil())
				Expect(newDf.NRows()).To(Equal(6))  // Number of rows should be 6
				Expect(newDf.Series).To(HaveLen(3)) // Check number of columns

				// Confirm that the timeAxis has all the expected values
				timeAxisIdx, err := newDf.NameToColumn(data.DateIdx)
				timeAxis := newDf.Series[timeAxisIdx]
				Expect(err).To(BeNil())
				Expect(timeAxis.Value(0).(time.Time)).Should(Equal(time.Date(1982, time.July, 27, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(1).(time.Time)).Should(Equal(time.Date(1983, time.July, 27, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(2).(time.Time)).Should(Equal(time.Date(1984, time.July, 27, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(3).(time.Time)).Should(Equal(time.Date(1985, time.July, 27, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(4).(time.Time)).Should(Equal(time.Date(1986, time.July, 27, 0, 0, 0, 0, time.UTC)))
				Expect(timeAxis.Value(5).(time.Time)).Should(Equal(time.Date(1987, time.July, 27, 0, 0, 0, 0, time.UTC)))

				// Confirm that col1 has all the expected values
				col1Idx, err := newDf.NameToColumn("col1")
				col1 := newDf.Series[col1Idx]
				Expect(err).To(BeNil())
				Expect(col1.Value(0)).Should(Equal(1.0))
				Expect(col1.Value(1)).Should(Equal(2.0))
				Expect(col1.Value(2)).Should(Equal(3.0))
				Expect(col1.Value(3)).Should(Equal(4.0))
				Expect(col1.Value(4)).Should(BeNil())
				Expect(col1.Value(5)).Should(BeNil())

				// Confirm that col2 has all the expected values
				col2Idx, err := newDf.NameToColumn("col2")
				col2 := newDf.Series[col2Idx]
				Expect(err).To(BeNil())
				Expect(col2.Value(0)).Should(BeNil())
				Expect(col2.Value(1)).Should(BeNil())
				Expect(col2.Value(2)).Should(Equal(1.0))
				Expect(col2.Value(3)).Should(Equal(2.0))
				Expect(col2.Value(4)).Should(Equal(3.0))
				Expect(col2.Value(5)).Should(Equal(4.0))
			})
		})
	})

})
