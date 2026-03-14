package signal_test

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// mockDataSource implements universe.DataSource for signal tests.
type mockDataSource struct {
	currentDate time.Time
	fetchResult *data.DataFrame
}

func (m *mockDataSource) Fetch(_ context.Context, _ []asset.Asset, _ portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	return m.fetchResult, nil
}

func (m *mockDataSource) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return m.fetchResult, nil
}

func (m *mockDataSource) CurrentDate() time.Time { return m.currentDate }

// Compile-time check.
var _ universe.DataSource = (*mockDataSource)(nil)

// errorDataSource always returns an error.
type errorDataSource struct {
	err error
}

func (e *errorDataSource) Fetch(_ context.Context, _ []asset.Asset, _ portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	return nil, e.err
}

func (e *errorDataSource) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return nil, e.err
}

func (e *errorDataSource) CurrentDate() time.Time { return time.Time{} }

// Compile-time check.
var _ universe.DataSource = (*errorDataSource)(nil)
