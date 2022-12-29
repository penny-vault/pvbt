// Copyright 2021-2022
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

package handler

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/filter"
	"github.com/penny-vault/pv-api/messenger"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/rs/zerolog/log"
)

type PortfolioResponse struct {
	ID                 uuid.UUID              `json:"id"`
	Name               string                 `json:"name"`
	AccountType        string                 `json:"accountType"`
	AccountNumber      string                 `json:"accountNumber"`
	Brokerage          string                 `json:"brokerage"`
	IsOpen             bool                   `json:"isOpen"`
	Strategy           string                 `json:"strategy"`
	Arguments          map[string]interface{} `json:"arguments"`
	StartDate          int64                  `json:"startDate"`
	TaxLotMethod       string                 `json:"taxLotMethod"`
	BenchmarkTicker    string                 `json:"benchmarkTicker"`
	Status             string                 `json:"status"`
	YTDReturn          pgtype.Float8          `json:"ytdReturn"`
	CAGRSinceInception pgtype.Float8          `json:"cagrSinceInception"`
	Notifications      int                    `json:"notifications"`
	Cagr3Year          pgtype.Float4          `json:"cagr3Year"`
	Cagr5Year          pgtype.Float4          `json:"cagr5Year"`
	Cagr10Year         pgtype.Float4          `json:"cagr10Year"`
	StdDev             pgtype.Float4          `json:"stdDev"`
	DownsideDeviation  pgtype.Float4          `json:"downsideDeviation"`
	MaxDrawDown        pgtype.Float4          `json:"maxDrawDown"`
	AvgDrawDown        pgtype.Float4          `json:"avgDrawDown"`
	SharpeRatio        pgtype.Float4          `json:"sharpeRatio"`
	SortinoRatio       pgtype.Float4          `json:"sortinoRatio"`
	UlcerIndex         pgtype.Float4          `json:"ulcerIndex"`
	NextTradeDate      pgtype.Int4            `json:"nextTradeDate"`
	LastViewed         int64                  `json:"lastViewed"`
	Created            int64                  `json:"created"`
	LastChanged        int64                  `json:"lastChanged"`
}

func NewPortfolioResponse() PortfolioResponse {
	return PortfolioResponse{
		YTDReturn: pgtype.Float8{
			Status: pgtype.Null,
		},
		CAGRSinceInception: pgtype.Float8{
			Status: pgtype.Null,
		},
		Cagr3Year: pgtype.Float4{
			Status: pgtype.Null,
		},
		Cagr5Year: pgtype.Float4{
			Status: pgtype.Null,
		},
		Cagr10Year: pgtype.Float4{
			Status: pgtype.Null,
		},
		StdDev: pgtype.Float4{
			Status: pgtype.Null,
		},
		DownsideDeviation: pgtype.Float4{
			Status: pgtype.Null,
		},
		MaxDrawDown: pgtype.Float4{
			Status: pgtype.Null,
		},
		AvgDrawDown: pgtype.Float4{
			Status: pgtype.Null,
		},
		SharpeRatio: pgtype.Float4{
			Status: pgtype.Null,
		},
		SortinoRatio: pgtype.Float4{
			Status: pgtype.Null,
		},
		UlcerIndex: pgtype.Float4{
			Status: pgtype.Null,
		},
		NextTradeDate: pgtype.Int4{
			Status: pgtype.Null,
		},
	}
}

func invalidJSONValue[T float32 | float64](val T) bool {
	return math.IsNaN(float64(val)) || math.IsInf(float64(val), 0)
}

func (pr *PortfolioResponse) Sanitize() {
	if invalidJSONValue(pr.YTDReturn.Float) {
		pr.YTDReturn.Float = 0
		pr.YTDReturn.Status = pgtype.Null
	}

	if invalidJSONValue(pr.CAGRSinceInception.Float) {
		pr.CAGRSinceInception.Float = 0
		pr.CAGRSinceInception.Status = pgtype.Null
	}

	if invalidJSONValue(pr.Cagr3Year.Float) {
		pr.Cagr3Year.Float = 0
		pr.Cagr3Year.Status = pgtype.Null
	}

	if invalidJSONValue(pr.Cagr5Year.Float) {
		pr.Cagr5Year.Float = 0
		pr.Cagr5Year.Status = pgtype.Null
	}

	if invalidJSONValue(pr.Cagr10Year.Float) {
		pr.Cagr10Year.Float = 0
		pr.Cagr10Year.Status = pgtype.Null
	}

	if invalidJSONValue(pr.StdDev.Float) {
		pr.StdDev.Float = 0
		pr.StdDev.Status = pgtype.Null
	}

	if invalidJSONValue(pr.DownsideDeviation.Float) {
		pr.DownsideDeviation.Float = 0
		pr.DownsideDeviation.Status = pgtype.Null
	}

	if invalidJSONValue(pr.MaxDrawDown.Float) {
		pr.MaxDrawDown.Float = 0
		pr.MaxDrawDown.Status = pgtype.Null
	}

	if invalidJSONValue(pr.AvgDrawDown.Float) {
		pr.AvgDrawDown.Float = 0
		pr.AvgDrawDown.Status = pgtype.Null
	}

	if invalidJSONValue(pr.SharpeRatio.Float) {
		pr.SharpeRatio.Float = 0
		pr.SharpeRatio.Status = pgtype.Null
	}

	if invalidJSONValue(pr.SortinoRatio.Float) {
		pr.SortinoRatio.Float = 0
		pr.SortinoRatio.Status = pgtype.Null
	}

	if invalidJSONValue(pr.UlcerIndex.Float) {
		pr.UlcerIndex.Float = 0
		pr.UlcerIndex.Status = pgtype.Null
	}
}

func CreatePortfolio(c *fiber.Ctx) error {
	ctx := context.Background()
	userID := c.Locals("userID").(string)
	portfolioID := uuid.New()

	subLog := log.With().Str("Endpoint", "GetPortfolio").Str("PortfolioID", portfolioID.String()).Str("UserID", userID).Logger()

	portfolioParams := NewPortfolioResponse()
	portfolioParams.ID = portfolioID

	if err := json.Unmarshal(c.Body(), &portfolioParams); err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to deserialize portfolio params")
		return fiber.ErrBadRequest
	}

	jsonArgs, err := json.Marshal(portfolioParams.Arguments)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to re-serialize json arguments")
		return fiber.ErrBadRequest
	}

	portfolioSQL := fmt.Sprintf(`INSERT INTO portfolios (
		"id",
		"name",
		"account_number",
		"brokerage",
		"account_type",
		"is_open",
		"tax_lot_method",
		"strategy_shortcode",
		"arguments",
		"benchmark",
		"start_date",
		"temporary",
		"user_id",
		"holdings"
	) VALUES (
		$1,
		$2,
		$3,
		$4,
		$5,
		't',
		$6,
		$7,
		$8,
		$9,
		$10,
		'f',
		$11,
		'{"%s": 10000}'
	)`, data.CashAsset)
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}
	_, err = trx.Exec(ctx,
		portfolioSQL,
		portfolioID,                             // $1
		portfolioParams.Name,                    // $2
		portfolioParams.AccountNumber,           // $3
		portfolioParams.Brokerage,               // $4
		portfolioParams.AccountType,             // $5
		portfolioParams.TaxLotMethod,            // $6
		portfolioParams.Strategy,                // $7
		jsonArgs,                                // $8
		portfolioParams.BenchmarkTicker,         // $9
		time.Unix(portfolioParams.StartDate, 0), // $10
		userID,                                  // $11
	)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not save new portfolio")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrNotFound
	}

	depositTransactionSQL := `INSERT INTO portfolio_transactions ("portfolio_id", "event_date", "num_shares", "price_per_share", "source", "ticker", "total_value", "transaction_type", "user_id") VALUES ($1, $2, 10000, 1, 'PV', '$CASH', 10000, 'DEPOSIT', $3)`
	_, err = trx.Exec(ctx, depositTransactionSQL, portfolioID, time.Unix(portfolioParams.StartDate, 0), userID)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not save new portfolio transaction")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrNotFound
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit database transaction")
	}

	// create a new portfolio simulation request
	if err := messenger.CreateSimulationRequest(userID, portfolioID); err != nil {
		return fiber.ErrFailedDependency
	}

	return c.JSON(portfolioParams)
}

// GetPortfolio get a portfolio
// @Description Retrieve a portfolio saved on the server
// @Id GetPortfolio
// @Produce json
// @Param id path string true "id of porfolio to retrieve"
func GetPortfolio(c *fiber.Ctx) error {
	ctx := context.Background()
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)
	subLog := log.With().Str("Endpoint", "GetPortfolio").Str("PortfolioID", portfolioID).Str("UserID", userID).Logger()

	portfolioSQL := `SELECT id, name, account_number, brokerage, account_type, is_open, tax_lot_method, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from last_viewed)::int as last_viewed, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolios WHERE id=$1 AND user_id=$2`
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}
	row := trx.QueryRow(ctx, portfolioSQL, portfolioID, userID)
	p := NewPortfolioResponse()
	err = row.Scan(&p.ID, &p.Name, &p.AccountNumber, &p.Brokerage, &p.AccountType, &p.IsOpen, &p.TaxLotMethod, &p.Strategy, &p.Arguments, &p.StartDate, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.LastViewed, &p.Created, &p.LastChanged)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not scan row from db into Performance struct")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrNotFound
	}

	p.Sanitize()

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit database transaction")
	}

	return c.JSON(p)
}

func GetPortfolioPerformance(c *fiber.Ctx) error {
	ctx := context.Background()
	portfolioIDStr := c.Params("id")
	subLog := log.With().Str("PortfolioID", portfolioIDStr).Str("Endpoint", "GetPortfolioPerformance").Logger()
	portfolioID, err := uuid.Parse(portfolioIDStr)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("failed to parse portfolio id")
		return fiber.ErrBadRequest
	}

	userID := c.Locals("userID").(string)

	p, err := portfolio.LoadPerformanceFromDB(ctx, portfolioID, userID)
	if err != nil {
		return err
	}

	data, err := p.MarshalBinary()
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not marshal performance to binary")
	}
	return c.Send(data)
}

func GetPortfolioMeasurements(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)

	subLog := log.With().Str("PortfolioID", portfolioID).Str("UserID", userID).Logger()

	f := filter.New(portfolioID, userID)

	field1 := c.Query("field1", "strategy_growth_of_10k")
	field2 := c.Query("field2", "benchmark_growth_of_10k")

	sinceStr := c.Query("since", "0")

	where := make(map[string]string)
	req := c.Request()
	req.URI().QueryArgs().VisitAll(func(key, value []byte) {
		k := string(key)
		if k != "field1" && k != "field2" && k != "offset" && k != "limit" {
			where[k] = string(value)
		}
	})

	var since time.Time
	if sinceStr == "0" {
		since = time.Date(1900, 1, 1, 0, 0, 0, 0, time.Local)
	} else {
		var err error
		since, err = time.Parse("2006-01-02T15:04:05", sinceStr)
		if err != nil {
			subLog.Warn().Stack().Err(err).Msg("could not parse date string")
		}
	}

	data, err := f.GetMeasurements(field1, field2, since)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not retrieve measurements")
		return fiber.ErrBadRequest
	}

	return c.Send(data)
}

func GetPortfolioHoldings(c *fiber.Ctx) error {
	portfolioIDStr := c.Params("id")
	userID := c.Locals("userID").(string)
	frequency := dataframe.Frequency(c.Query("frequency", "Monthly"))
	sinceStr := c.Query("since", "0")
	subLog := log.With().Str("PortfolioID", portfolioIDStr).Str("UserID", userID).Str("Frequency", string(frequency)).Str("Since", sinceStr).Logger()

	f := filter.New(portfolioIDStr, userID)

	var since time.Time
	if sinceStr == "0" {
		since = time.Date(1900, 1, 1, 0, 0, 0, 0, time.Local)
	} else {
		var err error
		since, err = time.Parse("2006-01-02T15:04:05", sinceStr)
		if err != nil {
			subLog.Warn().Stack().Err(err).Msg("could not parse since string")
		}
	}

	data, err := f.GetHoldings(frequency, since)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not retrieve holdings")
		return fiber.ErrBadRequest
	}

	return c.Send(data)
}

func GetPortfolioTransactions(c *fiber.Ctx) error {
	portfolioIDStr := c.Params("id")
	userID := c.Locals("userID").(string)
	sinceStr := c.Query("since", "0")
	subLog := log.With().Str("PortfolioID", portfolioIDStr).Str("UserID", userID).Str("Since", sinceStr).Logger()
	f := filter.New(portfolioIDStr, userID)

	var since time.Time
	if sinceStr == "0" {
		since = time.Date(1900, 1, 1, 0, 0, 0, 0, time.Local)
	} else {
		var err error
		since, err = time.Parse("2006-01-02T15:04:05", sinceStr)
		if err != nil {
			subLog.Warn().Stack().Err(err).Msg("could not parse date string")
		}
	}

	data, err := f.GetTransactions(since)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not retrieve transactions")
		return fiber.ErrBadRequest
	}

	return c.Send(data)
}

// ListPortfolios list all portfolios for logged in user
func ListPortfolios(c *fiber.Ctx) error {
	ctx := context.Background()
	userID := c.Locals("userID").(string)
	subLog := log.With().Str("UserID", userID).Str("Endpoint", "ListPortfolios").Logger()

	portfolioSQL := `SELECT
		id,
		name,
		strategy_shortcode,
		arguments,
		extract(epoch from start_date)::int as start_date,
		benchmark,
		account_type,
		account_number,
		brokerage,
		is_open,
		tax_lot_method,
		ytd_return,
		cagr_since_inception,
		notifications,
		status,
		cagr_3yr,
		cagr_5yr,
		cagr_10yr,
		std_dev,
		downside_deviation,
		max_draw_down,
		avg_draw_down,
		sharpe_ratio,
		sortino_ratio,
		ulcer_index,
		extract(epoch from next_trade_date)::int as next_trade_date,
		extract(epoch from last_viewed)::int as last_viewed,
		extract(epoch from created)::int as created,
		extract(epoch from lastchanged)::int as lastchanged
	FROM portfolios WHERE user_id=$1 AND temporary=false ORDER BY name, created`
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	rows, err := trx.Query(ctx, portfolioSQL, userID)
	if err != nil {
		subLog.Warn().Stack().Err(err).Str("Query", portfolioSQL).Msg("database query failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	portfolios := make([]PortfolioResponse, 0, 10)
	for rows.Next() {
		p := NewPortfolioResponse()
		err := rows.Scan(
			&p.ID,                 // 0
			&p.Name,               // 1
			&p.Strategy,           // 2
			&p.Arguments,          // 3
			&p.StartDate,          // 4
			&p.BenchmarkTicker,    // 5
			&p.AccountType,        // 6
			&p.AccountNumber,      // 7
			&p.Brokerage,          // 8
			&p.IsOpen,             // 9
			&p.TaxLotMethod,       // 10
			&p.YTDReturn,          // 11
			&p.CAGRSinceInception, // 12
			&p.Notifications,      // 13
			&p.Status,             // 14
			&p.Cagr3Year,          // 15
			&p.Cagr5Year,          // 16
			&p.Cagr10Year,         // 17
			&p.StdDev,             // 18
			&p.DownsideDeviation,  // 19
			&p.MaxDrawDown,        // 20
			&p.AvgDrawDown,        // 21
			&p.SharpeRatio,        // 22
			&p.SortinoRatio,       // 23
			&p.UlcerIndex,         // 24
			&p.NextTradeDate,      // 25
			&p.LastViewed,         // 26
			&p.Created,            // 27
			&p.LastChanged,        // 28
		)
		if err != nil {
			subLog.Warn().Stack().Err(err).Str("Query", portfolioSQL).Msg("ListPortfolio scan failed")
			continue
		}
		p.Sanitize()
		portfolios = append(portfolios, p)
	}

	err = rows.Err()
	if err != nil {
		subLog.Warn().Stack().Str("Query", portfolioSQL).Msg("ListPortfolio failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit database transaction")
	}

	return c.JSON(portfolios)
}

// UpdatePortfolio loads the portfolio from the database and updates it with the values passed
// via the PATCH command
func UpdatePortfolio(c *fiber.Ctx) error {
	ctx := context.Background()
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)
	subLog := log.With().Str("PortfolioID", portfolioID).Str("UserID", userID).Str("Endpoint", "UpdatePortfolio").Logger()

	params := NewPortfolioResponse()
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		subLog.Warn().Stack().Err(err).Msg("unmarshal request JSON failed")
		return fiber.ErrBadRequest
	}

	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	portfolioSQL := `SELECT id, name, account_type, account_number, brokerage, is_open, tax_lot_method, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from last_viewed)::int as last_viewed, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolios WHERE id=$1 AND user_id=$2`
	row := trx.QueryRow(ctx, portfolioSQL, portfolioID, userID)
	p := NewPortfolioResponse()

	err = row.Scan(&p.ID, &p.Name, &p.AccountType, &p.AccountNumber, &p.Brokerage, &p.IsOpen, &p.TaxLotMethod, &p.Strategy, &p.Arguments, &p.StartDate, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.LastViewed, &p.Created, &p.LastChanged)
	if err != nil {
		subLog.Warn().Stack().Err(err).Bytes("Body", c.Body()).Str("Query", portfolioSQL).Msg("select portfolio info failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrNotFound
	}

	p.Sanitize()

	if params.Name == "" {
		params.Name = p.Name
	}

	if params.Notifications == 0 {
		params.Notifications = p.Notifications
	}

	if params.AccountType == "" {
		params.AccountType = p.AccountType
	}

	if params.AccountNumber == "" {
		params.AccountNumber = p.AccountNumber
	}

	if params.Brokerage == "" {
		params.Brokerage = p.Brokerage
	}

	if params.TaxLotMethod == "" {
		params.TaxLotMethod = p.TaxLotMethod
	}

	updateSQL := `UPDATE portfolios SET name=$1, notifications=$2, account_type=$3, account_number=$4, brokerage=$5, tax_lot_method=$6 WHERE id=$7 AND user_id=$8`
	_, err = trx.Exec(ctx, updateSQL, params.Name, params.Notifications, params.AccountType, params.AccountNumber, params.Brokerage, params.TaxLotMethod, portfolioID, userID)
	if err != nil {
		subLog.Warn().Stack().Err(err).Bytes("Body", c.Body()).Str("Query", updateSQL).Msg("update portfolio settings failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	p = NewPortfolioResponse()
	err = trx.QueryRow(ctx, portfolioSQL, portfolioID, userID).
		Scan(&p.ID, &p.Name, &p.AccountType, &p.AccountNumber, &p.Brokerage, &p.IsOpen, &p.TaxLotMethod, &p.Strategy, &p.Arguments, &p.StartDate, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.LastViewed, &p.Created, &p.LastChanged)
	if err != nil {
		subLog.Warn().Stack().Err(err).Str("Query", portfolioSQL).Bytes("Body", c.Body()).Msg("sacn portfolio statistics failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	p.Sanitize()

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit database transaction")
	}
	return c.JSON(p)
}

// DeletePortfolio delete portfolio
func DeletePortfolio(c *fiber.Ctx) error {
	ctx := context.Background()
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)
	subLog := log.With().Str("PortfolioID", portfolioID).Str("UserID", userID).Str("Endpoint", "DeletePortfolio").Logger()

	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Msg("could not get database transaction")
		return fiber.ErrServiceUnavailable
	}

	deleteSQL := "DELETE FROM portfolios WHERE id=$1 AND user_id=$2"
	_, err = trx.Exec(context.Background(), deleteSQL, portfolioID, userID)
	if err != nil {
		subLog.Warn().Stack().Str("Query", deleteSQL).Err(err).Msg("could not delete portfolio from db")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	err = trx.Commit(context.Background())
	if err != nil {
		subLog.Error().Stack().Err(err).Str("Query", deleteSQL).Msg("could not commit transaction")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return fiber.ErrInternalServerError
	}

	return c.JSON(fiber.Map{"status": "success"})
}
