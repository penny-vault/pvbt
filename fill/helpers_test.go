package fill_test

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// buildBar creates a single-row DataFrame for one asset with the given metrics.
// Panics on construction error (safe in tests).
func buildBar(date time.Time, aa asset.Asset, metrics map[data.Metric]float64) *data.DataFrame {
	times := []time.Time{date}
	assets := []asset.Asset{aa}

	metricNames := make([]data.Metric, 0, len(metrics))
	columns := make([][]float64, 0, len(metrics))

	for metric, val := range metrics {
		metricNames = append(metricNames, metric)
		columns = append(columns, []float64{val})
	}

	df, err := data.NewDataFrame(times, assets, metricNames, data.Daily, columns)
	if err != nil {
		panic(err)
	}

	return df
}
