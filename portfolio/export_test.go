package portfolio

// export_test.go exposes unexported helpers for use by portfolio_test tests.

var (
	ExportReturns            = returns
	ExportExcessReturns      = excessReturns
	ExportAnnualizationFactor = annualizationFactor
	ExportCovariance         = covariance
	ExportWindowSlice        = windowSlice
	ExportWindowSliceTimes   = windowSliceTimes
	ExportDrawdownSeries     = drawdownSeries
	ExportCagr               = cagr
	ExportVariance           = variance
	ExportStddev             = stddev
	ExportMean               = mean
)
