// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package data

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/penny-vault/pvbt/asset"
	"github.com/rs/zerolog"
)

// Compile-time interface checks.
var _ BatchProvider = (*PVDataProvider)(nil)
var _ AssetProvider = (*PVDataProvider)(nil)
var _ RatingProvider = (*PVDataProvider)(nil)

// pvdataConfig is the subset of ~/.pvdata.toml we care about.
type pvdataConfig struct {
	DB struct {
		URL string `toml:"url"`
	} `toml:"db"`
}

// PVDataProvider is a BatchProvider that reads from a pv-data
// PostgreSQL database through the canonical preferred views.
type PVDataProvider struct {
	pool      *pgxpool.Pool
	ownsPool  bool
	dimension string
}

// PVDataOption configures a PVDataProvider.
type PVDataOption func(*pvdataOptions)

type pvdataOptions struct {
	dimension  string
	configFile string
}

// WithDimension sets the fundamental dimension filter (default "ARQ").
func WithDimension(dim string) PVDataOption {
	return func(o *pvdataOptions) { o.dimension = dim }
}

// WithConfigFile overrides the default config file path (~/.pvdata.toml).
func WithConfigFile(path string) PVDataOption {
	return func(o *pvdataOptions) { o.configFile = path }
}

// NewPVDataProvider creates a provider that reads from a pv-data database.
// If pool is nil the provider reads ~/.pvdata.toml (or the path set via
// WithConfigFile) for the connection URL and creates its own pool.
func NewPVDataProvider(pool *pgxpool.Pool, opts ...PVDataOption) (*PVDataProvider, error) {
	o := pvdataOptions{
		dimension: "ARQ",
	}
	for _, fn := range opts {
		fn(&o)
	}

	ownsPool := false
	if pool == nil {
		cfgPath := o.configFile
		if cfgPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("pvdata: determine home directory: %w", err)
			}
			cfgPath = filepath.Join(home, ".pvdata.toml")
		}

		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("pvdata: read config %s: %w", cfgPath, err)
		}

		var cfg pvdataConfig
		if err := toml.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("pvdata: parse config %s: %w", cfgPath, err)
		}

		if cfg.DB.URL == "" {
			return nil, fmt.Errorf("pvdata: no db.url in %s", cfgPath)
		}

		pool, err = pgxpool.New(context.Background(), cfg.DB.URL)
		if err != nil {
			return nil, fmt.Errorf("pvdata: connect to database: %w", err)
		}
		ownsPool = true
	}

	return &PVDataProvider{
		pool:      pool,
		ownsPool:  ownsPool,
		dimension: o.dimension,
	}, nil
}

// Provides returns all metrics that PVDataProvider can supply.
func (p *PVDataProvider) Provides() []Metric {
	metrics := make([]Metric, 0, len(metricView))
	for m := range metricView {
		metrics = append(metrics, m)
	}
	return metrics
}

// LookupAsset resolves a ticker to an Asset using the assets view.
func (p *PVDataProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	var a asset.Asset
	err = conn.QueryRow(ctx,
		"SELECT ticker, composite_figi FROM assets WHERE ticker = $1 AND active = true LIMIT 1",
		ticker,
	).Scan(&a.Ticker, &a.CompositeFigi)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("pvdata: lookup asset %q: %w", ticker, err)
	}

	return a, nil
}

// Assets returns all known assets from the database.
func (p *PVDataProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT composite_figi, ticker FROM assets ORDER BY ticker`)
	if err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset
	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, rows.Err()
}

// Fetch retrieves data for the requested assets, metrics, and time range.
func (p *PVDataProvider) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	log := zerolog.Ctx(ctx)
	log.Debug().
		Int("assets", len(req.Assets)).
		Int("metrics", len(req.Metrics)).
		Time("start", req.Start).
		Time("end", req.End).
		Msg("PVDataProvider.Fetch")

	// group requested metrics by view
	viewMetrics := make(map[string][]Metric)
	for _, m := range req.Metrics {
		v, ok := metricView[m]
		if !ok {
			continue
		}
		viewMetrics[v] = append(viewMetrics[v], m)
	}

	// collect composite figis for the WHERE IN clause
	figis := make([]string, len(req.Assets))
	for i, a := range req.Assets {
		figis[i] = a.CompositeFigi
	}

	// accumulate timestamps and per-column data keyed by Unix seconds.
	// We use int64 keys because time.Time equality in Go compares Location
	// pointers, making it unsuitable as a map key for times from different
	// LoadLocation calls.
	type colKey struct {
		figi   string
		metric Metric
	}
	colData := make(map[colKey]map[int64]float64)
	timeSet := make(map[int64]time.Time)

	ensureCol := func(figi string, m Metric) map[int64]float64 {
		k := colKey{figi, m}
		if c, ok := colData[k]; ok {
			return c
		}
		c := make(map[int64]float64)
		colData[k] = c
		return c
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	// fetch from each view that has requested metrics
	if metrics, ok := viewMetrics["eod"]; ok {
		if err := p.fetchEod(ctx, conn, figis, req.Start, req.End, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}
	if metrics, ok := viewMetrics["metrics"]; ok {
		if err := p.fetchMetrics(ctx, conn, figis, req.Start, req.End, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}
	if metrics, ok := viewMetrics["fundamentals"]; ok {
		if err := p.fetchFundamentals(ctx, conn, figis, req.Start, req.End, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}

	// build sorted time axis
	times := make([]time.Time, 0, len(timeSet))
	for _, t := range timeSet {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	log.Debug().
		Int("unique_times", len(times)).
		Int("colData_keys", len(colData)).
		Msg("PVDataProvider.Fetch time axis")

	if len(times) == 0 {
		df, err := NewDataFrame(nil, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("creating empty DataFrame: %w", err)
		}
		return df, nil
	}

	// build time index for fast lookup (by Unix seconds)
	timeIdx := make(map[int64]int, len(times))
	for i, t := range times {
		timeIdx[t.Unix()] = i
	}

	T := len(times)

	// assemble the data slab
	data := make([]float64, T*len(req.Assets)*len(req.Metrics))
	for i := range data {
		data[i] = math.NaN()
	}

	// build metric index
	mIdx := make(map[Metric]int, len(req.Metrics))
	for i, m := range req.Metrics {
		mIdx[m] = i
	}

	// build asset index
	aIdx := make(map[string]int, len(req.Assets))
	for i, a := range req.Assets {
		aIdx[a.CompositeFigi] = i
	}

	M := len(req.Metrics)
	for k, vals := range colData {
		ai, ok := aIdx[k.figi]
		if !ok {
			continue
		}
		mi, ok := mIdx[k.metric]
		if !ok {
			continue
		}
		colStart := (ai*M + mi) * T
		for sec, v := range vals {
			ti := timeIdx[sec]
			data[colStart+ti] = v
		}
	}

	df, err := NewDataFrame(times, req.Assets, req.Metrics, data)
	if err != nil {
		return nil, fmt.Errorf("building DataFrame: %w", err)
	}
	return df, nil
}

// Close releases resources. If the provider created its own pool it is closed.
func (p *PVDataProvider) Close() error {
	if p.ownsPool && p.pool != nil {
		p.pool.Close()
	}
	return nil
}

// -- fetch helpers -----------------------------------------------------------

func (p *PVDataProvider) fetchEod(
	ctx context.Context,
	conn *pgxpool.Conn,
	figis []string,
	start, end time.Time,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	zerolog.Ctx(ctx).Debug().
		Strs("figis", figis).
		Time("start", start).
		Time("end", end).
		Msg("fetchEod query")

	rows, err := conn.Query(ctx,
		`SELECT composite_figi, event_date, open, high, low, close, adj_close, volume, dividend, split_factor
		 FROM eod
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date
		 ORDER BY event_date`,
		figis, start, end,
	)
	if err != nil {
		return fmt.Errorf("pvdata: query eod: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	rowCount := 0
	for rows.Next() {
		rowCount++
		var (
			figi                                 string
			eventDate                            time.Time
			open, high, low, close, adjClose     float64
			volume                               float64
			dividend, splitFactor                 float64
		)
		if err := rows.Scan(&figi, &eventDate, &open, &high, &low, &close, &adjClose, &volume, &dividend, &splitFactor); err != nil {
			return fmt.Errorf("pvdata: scan eod row: %w", err)
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		if want[MetricOpen] {
			ensureCol(figi, MetricOpen)[sec] = open
		}
		if want[MetricHigh] {
			ensureCol(figi, MetricHigh)[sec] = high
		}
		if want[MetricLow] {
			ensureCol(figi, MetricLow)[sec] = low
		}
		if want[MetricClose] {
			ensureCol(figi, MetricClose)[sec] = close
		}
		if want[AdjClose] {
			ensureCol(figi, AdjClose)[sec] = adjClose
		}
		if want[Volume] {
			ensureCol(figi, Volume)[sec] = volume
		}
		if want[Dividend] {
			ensureCol(figi, Dividend)[sec] = dividend
		}
		if want[SplitFactor] {
			ensureCol(figi, SplitFactor)[sec] = splitFactor
		}
	}

	zerolog.Ctx(ctx).Debug().Int("rows", rowCount).Msg("fetchEod result")

	return rows.Err()
}

func (p *PVDataProvider) fetchMetrics(
	ctx context.Context,
	conn *pgxpool.Conn,
	figis []string,
	start, end time.Time,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	rows, err := conn.Query(ctx,
		`SELECT composite_figi, event_date, market_cap, ev, pe, pb, ps, ev_ebit, ev_ebitda, sp500
		 FROM metrics
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date
		 ORDER BY event_date`,
		figis, start, end,
	)
	if err != nil {
		return fmt.Errorf("pvdata: query metrics: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	for rows.Next() {
		var (
			figi                         string
			eventDate                    time.Time
			marketCap, ev                int64
			pe, pb, ps, evEbit, evEbitda float64
			sp500                        bool
		)
		if err := rows.Scan(&figi, &eventDate, &marketCap, &ev, &pe, &pb, &ps, &evEbit, &evEbitda, &sp500); err != nil {
			return fmt.Errorf("pvdata: scan metrics row: %w", err)
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		if want[MarketCap] {
			ensureCol(figi, MarketCap)[sec] = float64(marketCap)
		}
		if want[EnterpriseValue] {
			ensureCol(figi, EnterpriseValue)[sec] = float64(ev)
		}
		if want[PE] {
			ensureCol(figi, PE)[sec] = pe
		}
		if want[PB] {
			ensureCol(figi, PB)[sec] = pb
		}
		if want[PS] {
			ensureCol(figi, PS)[sec] = ps
		}
		if want[EVtoEBIT] {
			ensureCol(figi, EVtoEBIT)[sec] = evEbit
		}
		if want[EVtoEBITDA] {
			ensureCol(figi, EVtoEBITDA)[sec] = evEbitda
		}
		if want[SP500] {
			v := 0.0
			if sp500 {
				v = 1.0
			}
			ensureCol(figi, SP500)[sec] = v
		}
	}

	return rows.Err()
}

func (p *PVDataProvider) fetchFundamentals(
	ctx context.Context,
	conn *pgxpool.Conn,
	figis []string,
	start, end time.Time,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	// build the list of SQL columns we need
	var sqlCols []string
	var metricOrder []Metric
	for _, m := range metrics {
		col, ok := metricColumn[m]
		if !ok {
			continue
		}
		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, m)
	}

	if len(sqlCols) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, %s
		 FROM fundamentals
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date AND dimension = $4
		 ORDER BY event_date`,
		strings.Join(sqlCols, ", "),
	)

	rows, err := conn.Query(ctx, query, figis, start, end, p.dimension)
	if err != nil {
		return fmt.Errorf("pvdata: query fundamentals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var figi string
		var eventDate time.Time

		vals := make([]any, len(sqlCols)+2)
		vals[0] = &figi
		vals[1] = &eventDate
		floatVals := make([]float64, len(sqlCols))
		for i := range sqlCols {
			vals[i+2] = &floatVals[i]
		}

		if err := rows.Scan(vals...); err != nil {
			return fmt.Errorf("pvdata: scan fundamentals row: %w", err)
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		for i, m := range metricOrder {
			ensureCol(figi, m)[sec] = floatVals[i]
		}
	}

	return rows.Err()
}

// RatedAssets returns the set of assets whose most-recent rating (on or before t)
// from the named analyst matches filter. It returns nil, nil when filter has no
// values to match.
func (p *PVDataProvider) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, t time.Time) ([]asset.Asset, error) {
	if len(filter.Values) == 0 {
		return nil, nil
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx,
		`SELECT composite_figi, ticker FROM (
		     SELECT DISTINCT ON (composite_figi) composite_figi, ticker, rating
		     FROM ratings
		     WHERE analyst = $1 AND event_date <= $2
		     ORDER BY composite_figi, event_date DESC
		 ) sub
		 WHERE rating = ANY($3)`,
		analyst, t, filter.Values,
	)
	if err != nil {
		return nil, fmt.Errorf("pvdata: query rated assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset
	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("pvdata: scan rated asset: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, rows.Err()
}

// -- metric mappings ---------------------------------------------------------

// metricView maps each Metric to the database view it comes from.
var metricView = map[Metric]string{
	// eod view
	MetricOpen:  "eod",
	MetricHigh:  "eod",
	MetricLow:   "eod",
	MetricClose: "eod",
	AdjClose:    "eod",
	Volume:      "eod",
	Dividend:    "eod",
	SplitFactor: "eod",

	// metrics view
	MarketCap:       "metrics",
	EnterpriseValue: "metrics",
	PE:              "metrics",
	PB:              "metrics",
	PS:              "metrics",
	EVtoEBIT:        "metrics",
	EVtoEBITDA:      "metrics",
	SP500:           "metrics",

	// fundamentals view
	Revenue:                             "fundamentals",
	CostOfRevenue:                       "fundamentals",
	GrossProfit:                         "fundamentals",
	OperatingExpenses:                   "fundamentals",
	OperatingIncome:                     "fundamentals",
	EBIT:                                "fundamentals",
	EBITDA:                              "fundamentals",
	EBT:                                 "fundamentals",
	ConsolidatedIncome:                  "fundamentals",
	NetIncome:                           "fundamentals",
	NetIncomeCommonStock:                "fundamentals",
	EarningsPerShare:                    "fundamentals",
	EPSDiluted:                          "fundamentals",
	InterestExpense:                     "fundamentals",
	IncomeTaxExpense:                    "fundamentals",
	RandDExpenses:                       "fundamentals",
	SGAExpense:                          "fundamentals",
	ShareBasedCompensation:              "fundamentals",
	DividendsPerShare:                   "fundamentals",
	NetLossIncomeDiscontinuedOperations: "fundamentals",
	NetIncomeToNonControllingInterests:  "fundamentals",
	PreferredDividendsImpact:            "fundamentals",
	TotalAssets:                          "fundamentals",
	CurrentAssets:                        "fundamentals",
	AssetsNonCurrent:                     "fundamentals",
	AverageAssets:                        "fundamentals",
	CashAndEquivalents:                   "fundamentals",
	Inventory:                            "fundamentals",
	Receivables:                          "fundamentals",
	Investments:                          "fundamentals",
	InvestmentsCurrent:                   "fundamentals",
	InvestmentsNonCur:                    "fundamentals",
	Intangibles:                          "fundamentals",
	PPENet:                               "fundamentals",
	TaxAssets:                            "fundamentals",
	TotalLiabilities:                     "fundamentals",
	CurrentLiabilities:                   "fundamentals",
	LiabilitiesNonCurrent:               "fundamentals",
	TotalDebt:                            "fundamentals",
	DebtCurrent:                          "fundamentals",
	DebtNonCurrent:                       "fundamentals",
	Payables:                             "fundamentals",
	DeferredRevenue:                      "fundamentals",
	Deposits:                             "fundamentals",
	TaxLiabilities:                       "fundamentals",
	Equity:                               "fundamentals",
	EquityAvg:                            "fundamentals",
	AccumulatedOtherComprehensiveIncome:  "fundamentals",
	AccumulatedRetainedEarningsDeficit:   "fundamentals",
	FreeCashFlow:                         "fundamentals",
	NetCashFlow:                          "fundamentals",
	NetCashFlowFromOperations:            "fundamentals",
	NetCashFlowFromInvesting:             "fundamentals",
	NetCashFlowFromFinancing:             "fundamentals",
	NetCashFlowBusiness:                  "fundamentals",
	NetCashFlowCommon:                    "fundamentals",
	NetCashFlowDebt:                      "fundamentals",
	NetCashFlowDividend:                  "fundamentals",
	NetCashFlowInvest:                    "fundamentals",
	NetCashFlowFx:                        "fundamentals",
	CapitalExpenditure:                   "fundamentals",
	DepreciationAmortization:             "fundamentals",
	BookValue:                            "fundamentals",
	FreeCashFlowPerShare:                 "fundamentals",
	SalesPerShare:                        "fundamentals",
	TangibleAssetsBookValuePerShare:      "fundamentals",
	ShareFactor:                          "fundamentals",
	SharesBasic:                          "fundamentals",
	WeightedAverageShares:                "fundamentals",
	WeightedAverageSharesDiluted:         "fundamentals",
	FundamentalPrice:                     "fundamentals",
	PE1:                                  "fundamentals",
	PS1:                                  "fundamentals",
	FxUSD:                                "fundamentals",
	GrossMargin:                          "fundamentals",
	EBITDAMargin:                         "fundamentals",
	ProfitMargin:                         "fundamentals",
	ROA:                                  "fundamentals",
	ROE:                                  "fundamentals",
	ROIC:                                 "fundamentals",
	ReturnOnSales:                        "fundamentals",
	AssetTurnover:                        "fundamentals",
	CurrentRatio:                         "fundamentals",
	DebtToEquity:                         "fundamentals",
	DividendYield:                        "fundamentals",
	PayoutRatio:                          "fundamentals",
	InvestedCapital:                      "fundamentals",
	InvestedCapitalAvg:                   "fundamentals",
	TangibleAssetValue:                   "fundamentals",
	WorkingCapital:                       "fundamentals",
	MarketCapFundamental:                 "fundamentals",
}

// metricColumn maps fundamental Metrics to their SQL column names.
var metricColumn = map[Metric]string{
	Revenue:                             "revenues",
	CostOfRevenue:                       "cost_of_revenue",
	GrossProfit:                         "gross_profit",
	OperatingExpenses:                   "operating_expenses",
	OperatingIncome:                     "operating_income",
	EBIT:                                "ebit",
	EBITDA:                              "ebitda",
	EBT:                                 "ebt",
	ConsolidatedIncome:                  "consolidated_income",
	NetIncome:                           "net_income",
	NetIncomeCommonStock:                "net_income_common_stock",
	EarningsPerShare:                    "eps",
	EPSDiluted:                          "eps_diluted",
	InterestExpense:                     "interest_expense",
	IncomeTaxExpense:                    "income_tax_expense",
	RandDExpenses:                       "r_and_d_expenses",
	SGAExpense:                          "selling_general_and_administrative_expense",
	ShareBasedCompensation:              "share_based_compensation",
	DividendsPerShare:                   "dividends_per_basic_common_share",
	NetLossIncomeDiscontinuedOperations: "net_loss_income_discontinued_operations",
	NetIncomeToNonControllingInterests:  "net_income_to_non_controlling_interests",
	PreferredDividendsImpact:            "preferred_dividends_income_statement_impact",
	TotalAssets:                          "total_assets",
	CurrentAssets:                        "current_assets",
	AssetsNonCurrent:                     "assets_non_current",
	AverageAssets:                        "average_assets",
	CashAndEquivalents:                   "cash_and_equivalents",
	Inventory:                            "inventory",
	Receivables:                          "receivables",
	Investments:                          "investments",
	InvestmentsCurrent:                   "investments_current",
	InvestmentsNonCur:                    "investments_non_current",
	Intangibles:                          "intangibles",
	PPENet:                               "property_plant_and_equipment_net",
	TaxAssets:                            "tax_assets",
	TotalLiabilities:                     "total_liabilities",
	CurrentLiabilities:                   "current_liabilities",
	LiabilitiesNonCurrent:               "liabilities_non_current",
	TotalDebt:                            "total_debt",
	DebtCurrent:                          "debt_current",
	DebtNonCurrent:                       "debt_non_current",
	Payables:                             "payables",
	DeferredRevenue:                      "deferred_revenue",
	Deposits:                             "deposits",
	TaxLiabilities:                       "tax_liabilities",
	Equity:                               "equity",
	EquityAvg:                            "equity_avg",
	AccumulatedOtherComprehensiveIncome:  "accumulated_other_comprehensive_income",
	AccumulatedRetainedEarningsDeficit:   "accumulated_retained_earnings_deficit",
	FreeCashFlow:                         "free_cash_flow",
	NetCashFlow:                          "net_cash_flow",
	NetCashFlowFromOperations:            "net_cash_flow_from_operations",
	NetCashFlowFromInvesting:             "net_cash_flow_from_investing",
	NetCashFlowFromFinancing:             "net_cash_flow_from_financing",
	NetCashFlowBusiness:                  "net_cash_flow_business",
	NetCashFlowCommon:                    "net_cash_flow_common",
	NetCashFlowDebt:                      "net_cash_flow_debt",
	NetCashFlowDividend:                  "net_cash_flow_dividend",
	NetCashFlowInvest:                    "net_cash_flow_invest",
	NetCashFlowFx:                        "net_cash_flow_fx",
	CapitalExpenditure:                   "capital_expenditure",
	DepreciationAmortization:             "depreciation_amortization_and_accretion",
	BookValue:                            "book_value_per_share",
	FreeCashFlowPerShare:                 "free_cash_flow_per_share",
	SalesPerShare:                        "sales_per_share",
	TangibleAssetsBookValuePerShare:      "tangible_assets_book_value_per_share",
	ShareFactor:                          "share_factor",
	SharesBasic:                          "shares_basic",
	WeightedAverageShares:                "weighted_average_shares",
	WeightedAverageSharesDiluted:         "weighted_average_shares_diluted",
	FundamentalPrice:                     "price",
	PE1:                                  "pe1",
	PS1:                                  "ps1",
	FxUSD:                                "fx_usd",
	GrossMargin:                          "gross_margin",
	EBITDAMargin:                         "ebitda_margin",
	ProfitMargin:                         "profit_margin",
	ROA:                                  "roa",
	ROE:                                  "roe",
	ROIC:                                 "roic",
	ReturnOnSales:                        "return_on_sales",
	AssetTurnover:                        "asset_turnover",
	CurrentRatio:                         "current_ratio",
	DebtToEquity:                         "debt_to_equity_ratio",
	DividendYield:                        "dividend_yield",
	PayoutRatio:                          "payout_ratio",
	InvestedCapital:                      "invested_capital",
	InvestedCapitalAvg:                   "invested_capital_average",
	TangibleAssetValue:                   "tangible_asset_value",
	WorkingCapital:                       "working_capital",
	MarketCapFundamental:                 "market_capitalization",
}

func metricSet(ms []Metric) map[Metric]bool {
	s := make(map[Metric]bool, len(ms))
	for _, m := range ms {
		s[m] = true
	}
	return s
}

// eodTimestamp converts a database date to the market close timestamp (16:00 Eastern).
func eodTimestamp(t time.Time) time.Time {
	nyc, _ := time.LoadLocation("America/New_York")
	return time.Date(t.Year(), t.Month(), t.Day(), 16, 0, 0, 0, nyc)
}
