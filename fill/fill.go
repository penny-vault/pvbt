package fill

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// FillResult describes the outcome of a fill computation.
type FillResult struct {
	Price    float64
	Partial  bool
	Quantity float64
}

// BaseModel produces the initial fill price from market data.
type BaseModel interface {
	Fill(ctx context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error)
}

// Adjuster modifies a FillResult produced by a BaseModel or prior Adjuster.
type Adjuster interface {
	Adjust(ctx context.Context, order broker.Order, bar *data.DataFrame, current FillResult) (FillResult, error)
}

// DataFetcher provides on-demand data access for models that need more than the current bar.
type DataFetcher interface {
	FetchAt(ctx context.Context, assets []asset.Asset, timestamp time.Time, metrics []data.Metric) (*data.DataFrame, error)
}

// DataFetcherAware is implemented by models that need a DataFetcher injected.
type DataFetcherAware interface {
	SetDataFetcher(DataFetcher)
}

// Pipeline composes a BaseModel with zero or more Adjusters.
type Pipeline struct {
	base      BaseModel
	adjusters []Adjuster
}

// NewPipeline creates a Pipeline from a base model and optional adjusters.
func NewPipeline(base BaseModel, adjusters []Adjuster) *Pipeline {
	return &Pipeline{base: base, adjusters: adjusters}
}

// Fill runs the base model then each adjuster in sequence.
func (pp *Pipeline) Fill(ctx context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error) {
	result, err := pp.FillBase(ctx, order, bar)
	if err != nil {
		return FillResult{}, err
	}

	return pp.Adjust(ctx, order, bar, result)
}

// FillBase runs only the base model and returns its result.
func (pp *Pipeline) FillBase(ctx context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error) {
	return pp.base.Fill(ctx, order, bar)
}

// Adjust runs all adjusters in sequence on the given FillResult.
func (pp *Pipeline) Adjust(ctx context.Context, order broker.Order, bar *data.DataFrame, current FillResult) (FillResult, error) {
	result := current

	for _, adj := range pp.adjusters {
		var err error

		result, err = adj.Adjust(ctx, order, bar, result)
		if err != nil {
			return FillResult{}, err
		}
	}

	return result, nil
}

// SetDataFetcher propagates the fetcher to any base model or adjuster that implements DataFetcherAware.
func (pp *Pipeline) SetDataFetcher(fetcher DataFetcher) {
	if aware, ok := pp.base.(DataFetcherAware); ok {
		aware.SetDataFetcher(fetcher)
	}

	for _, adj := range pp.adjusters {
		if aware, ok := adj.(DataFetcherAware); ok {
			aware.SetDataFetcher(fetcher)
		}
	}
}
