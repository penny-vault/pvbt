package universe_test

import (
	"context"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// mockDataSource implements universe.DataSource for testing.
type mockDataSource struct {
	currentDate time.Time
	fetchCalled bool
	fetchAssets []asset.Asset
	fetchPeriod portfolio.Period
	fetchResult *data.DataFrame
}

func (m *mockDataSource) Fetch(_ context.Context, assets []asset.Asset, lookback portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	m.fetchCalled = true
	m.fetchAssets = assets
	m.fetchPeriod = lookback
	return m.fetchResult, nil
}

func (m *mockDataSource) FetchAt(_ context.Context, assets []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	m.fetchCalled = true
	m.fetchAssets = assets
	return m.fetchResult, nil
}

func (m *mockDataSource) CurrentDate() time.Time {
	return m.currentDate
}

func TestStaticUniverseWindow(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
	goog := asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}

	now := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	emptyDF, _ := data.NewDataFrame(nil, nil, nil, nil)

	ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
	u := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

	_, err := u.Window(portfolio.Months(3), data.MetricClose)
	if err != nil {
		t.Fatalf("Window returned error: %v", err)
	}

	if !ds.fetchCalled {
		t.Fatal("expected Fetch to be called")
	}
	if len(ds.fetchAssets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(ds.fetchAssets))
	}
	if ds.fetchPeriod != portfolio.Months(3) {
		t.Fatalf("expected Months(3), got %+v", ds.fetchPeriod)
	}
}

func TestStaticUniverseAt(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}

	now := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	emptyDF, _ := data.NewDataFrame(nil, nil, nil, nil)

	ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
	u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

	_, err := u.At(now, data.MetricClose)
	if err != nil {
		t.Fatalf("At returned error: %v", err)
	}

	if !ds.fetchCalled {
		t.Fatal("expected FetchAt to be called")
	}
}

func TestStaticUniverseWindowNoDataSource(t *testing.T) {
	u := universe.NewStatic("AAPL")
	_, err := u.Window(portfolio.Months(3), data.MetricClose)
	if err == nil {
		t.Fatal("expected error when no data source is set")
	}
}
