// Copyright 2021 JD Fergason
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"context"
	"database/sql"
	"main/database"
	"main/filter"
	"main/portfolio"
	"math"
	"time"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type PortfolioResponse struct {
	ID                 uuid.UUID              `json:"id"`
	Name               string                 `json:"name"`
	Strategy           string                 `json:"strategy"`
	Arguments          map[string]interface{} `json:"arguments"`
	StartDate          int64                  `json:"startDate"`
	BenchmarkTicker    string                 `json:"benchmarkTicker"`
	YTDReturn          sql.NullFloat64        `json:"ytdReturn"`
	CAGRSinceInception sql.NullFloat64        `json:"cagrSinceInception"`
	Notifications      int                    `json:"notifications"`
	Created            int64                  `json:"created"`
	LastChanged        int64                  `json:"lastChanged"`
}

func CreatePortfolio(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	portfolioID := uuid.New()

	portfolioParams := PortfolioResponse{
		ID: portfolioID,
	}

	if err := json.Unmarshal(c.Body(), &portfolioParams); err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Error("unable to deserialize portfolio params")
		return fiber.ErrBadRequest
	}

	jsonArgs, err := json.Marshal(portfolioParams.Arguments)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Error("unable to re-serialize json arguments")
		return fiber.ErrBadRequest
	}

	portfolioSQL := `INSERT INTO portfolios ("id", "name", "strategy_shortcode", "arguments", "benchmark", "start_date", "temporary", "user_id", "holdings") VALUES ($1, $2, $3, $4, $5, $6, 'f', $7, '{"$CASH": 10000}')`
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Error("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}
	_, err = trx.Exec(context.Background(), portfolioSQL, portfolioID, portfolioParams.Name, portfolioParams.Strategy, jsonArgs, portfolioParams.BenchmarkTicker, time.Unix(portfolioParams.StartDate, 0), userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "CreatePortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Warn("could not save new portfolio")
		trx.Rollback(context.Background())
		return fiber.ErrNotFound
	}

	depositTransactionSQL := `INSERT INTO portfolio_transactions ("portfolio_id", "event_date", "num_shares", "price_per_share", "source", "ticker", "total_value", "transaction_type", "user_id") VALUES ($1, $2, 10000, 1, 'PV', '$CASH', 10000, 'DEPOSIT', $3)`
	_, err = trx.Exec(context.Background(), depositTransactionSQL, portfolioID, time.Unix(portfolioParams.StartDate, 0), userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "CreatePortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Warn("could not save new portfolio transaction")
		trx.Rollback(context.Background())
		return fiber.ErrNotFound
	}
	trx.Commit(context.Background())
	return c.JSON(portfolioParams)
}

// GetPortfolio get a portfolio
// @Description Retrieve a portfolio saved on the server
// @Id GetPortfolio
// @Produce json
// @Param id path string true "id of porfolio to retrieve"
func GetPortfolio(c *fiber.Ctx) error {
	portfolioID := c.Params("id")

	userID := c.Locals("userID").(string)

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolios WHERE id=$1 AND user_id=$2`
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Error("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}
	row := trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID)
	p := PortfolioResponse{}
	err = row.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.StartDate, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Warn("could not scan row from db into Performance struct")
		trx.Rollback(context.Background())
		return fiber.ErrNotFound
	}
	if math.IsNaN(p.YTDReturn.Float64) || math.IsInf(p.YTDReturn.Float64, 0) {
		p.YTDReturn.Float64 = 0
		p.YTDReturn.Valid = false
	}
	if math.IsNaN(p.CAGRSinceInception.Float64) || math.IsInf(p.CAGRSinceInception.Float64, 0) {
		p.CAGRSinceInception.Float64 = 0
		p.CAGRSinceInception.Valid = false
	}
	trx.Commit(context.Background())
	return c.JSON(p)
}

func GetPortfolioPerformance(c *fiber.Ctx) error {
	portfolioIDStr := c.Params("id")
	portfolioID, err := uuid.Parse(portfolioIDStr)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":       "GetPortfolioPerformance",
			"Error":          err,
			"PortfolioIDStr": portfolioIDStr,
		}).Warn("failed to parse portfolio id")
		return fiber.ErrBadRequest
	}

	userID := c.Locals("userID").(string)

	p, err := portfolio.LoadPerformanceFromDB(portfolioID, userID)
	if err != nil {
		return err
	}

	data, err := p.MarshalBinary()
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":       "GetPortfolioPerformance",
			"Error":          err,
			"PortfolioIDStr": portfolioIDStr,
			"Code":           "Could not marshal performance to binary",
		})
	}
	return c.Send(data)
}

func GetPortfolioMeasurements(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)

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
			log.WithFields(log.Fields{
				"Error":      err,
				"DateString": sinceStr,
			}).Warn("could not parse date string")
		}
	}

	data, err := f.GetMeasurements(field1, field2, since)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not retrieve measurements")

		return fiber.ErrBadRequest
	}

	return c.Send(data)
}

func GetPortfolioHoldings(c *fiber.Ctx) error {
	portfolioIDStr := c.Params("id")
	userID := c.Locals("userID").(string)

	f := filter.New(portfolioIDStr, userID)

	frequency := c.Query("frequency", "monthly")
	sinceStr := c.Query("since", "0")
	var since time.Time
	if sinceStr == "0" {
		since = time.Date(1900, 1, 1, 0, 0, 0, 0, time.Local)
	} else {
		var err error
		since, err = time.Parse("2006-01-02T15:04:05", sinceStr)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":      err,
				"DateString": sinceStr,
			}).Warn("could not parse date string")
		}
	}

	data, err := f.GetHoldings(frequency, since)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not retrieve holdings")

		return fiber.ErrBadRequest
	}

	return c.Send(data)
}

func GetPortfolioTransactions(c *fiber.Ctx) error {
	portfolioIDStr := c.Params("id")
	userID := c.Locals("userID").(string)

	f := filter.New(portfolioIDStr, userID)

	sinceStr := c.Query("since", "0")
	var since time.Time
	if sinceStr == "0" {
		since = time.Date(1900, 1, 1, 0, 0, 0, 0, time.Local)
	} else {
		var err error
		since, err = time.Parse("2006-01-02T15:04:05", sinceStr)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":      err,
				"DateString": sinceStr,
			}).Warn("could not parse date string")
		}
	}

	data, err := f.GetTransactions(since)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("could not retrieve transactions")
		return fiber.ErrBadRequest
	}

	return c.Send(data)
}

// ListPortfolios list all portfolios for logged in user
func ListPortfolios(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolios WHERE user_id=$1 AND temporary=false ORDER BY name, created`
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "ListPortfolios",
			"Error":    err,
			"UserID":   userID,
		}).Error("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	rows, err := trx.Query(context.Background(), portfolioSQL, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "ListPortfolios",
			"Error":    err,
			"Query":    portfolioSQL,
		}).Warn("ListPortfolio failed")
		trx.Rollback(context.Background())
		return fiber.ErrInternalServerError
	}

	portfolios := make([]PortfolioResponse, 0, 10)
	for rows.Next() {
		p := PortfolioResponse{}
		err := rows.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.StartDate, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
		if err != nil {
			log.WithFields(log.Fields{
				"Endpoint": "ListPortfolios",
				"Error":    err,
				"Query":    portfolioSQL,
			}).Warn("ListPortfolio scan failed")
			continue
		}
		if math.IsNaN(p.YTDReturn.Float64) || math.IsInf(p.YTDReturn.Float64, 0) {
			p.YTDReturn.Float64 = 0
			p.YTDReturn.Valid = false
		}
		if math.IsNaN(p.CAGRSinceInception.Float64) || math.IsInf(p.CAGRSinceInception.Float64, 0) {
			p.CAGRSinceInception.Float64 = 0
			p.CAGRSinceInception.Valid = false
		}

		portfolios = append(portfolios, p)
	}

	err = rows.Err()
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "ListPortfolio",
			"Error":    err,
			"Query":    portfolioSQL,
		}).Warn("ListPortfolio failed")
		trx.Rollback(context.Background())
		return fiber.ErrInternalServerError
	}

	trx.Commit(context.Background())
	return c.JSON(portfolios)
}

// UpdatePortfolio loads the portfolio from the database and updates it with the values passed
// via the PATCH command
func UpdatePortfolio(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)

	params := PortfolioResponse{}
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "UpdatePortfolio",
			"Error":       err,
			"PortfolioID": portfolioID,
		}).Warn("UpdatePortfolio bad request")
		return fiber.ErrBadRequest
	}

	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "UpdatePortfolio",
			"Error":    err,
			"UserID":   userID,
		}).Error("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolios WHERE id=$1 AND user_id=$2`
	row := trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID)
	p := PortfolioResponse{}
	err = row.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.StartDate, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "UpdatePortfolio",
			"PortfolioID": portfolioID,
			"Error":       err,
			"Body":        string(c.Body()),
			"Query":       portfolioSQL,
		}).Warn("UpdatePortfolio failed")
		trx.Rollback(context.Background())
		return fiber.ErrNotFound
	}

	if params.Name == "" {
		params.Name = p.Name
	}

	if params.Notifications == 0 {
		params.Notifications = p.Notifications
	}

	updateSQL := `UPDATE portfolio SET name=$1, notifications=$2 WHERE id=$3 AND user_id=$4`
	_, err = trx.Exec(context.Background(), updateSQL, params.Name, params.Notifications, portfolioID, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "UpdatePortfolio",
			"PortfolioID": portfolioID,
			"Error":       err,
			"Body":        string(c.Body()),
			"Query":       portfolioSQL,
		}).Warnf("UpdatePortfolio SQL update failed")
		trx.Rollback(context.Background())
		return fiber.ErrInternalServerError
	}

	row = trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID)
	p = PortfolioResponse{}
	err = row.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.StartDate, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "UpdatePortfolio",
			"PortfolioID": portfolioID,
			"Error":       err,
			"Body":        string(c.Body()),
			"Query":       portfolioSQL,
		}).Warnf("UpdatePortfolio failed")
		trx.Rollback(context.Background())
		return fiber.ErrInternalServerError
	}

	trx.Commit(context.Background())
	return c.JSON(p)
}

// DeletePortfolio delete portfolio
func DeletePortfolio(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)

	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "DeletePortfolio",
			"Error":    err,
			"UserID":   userID,
		}).Error("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	deleteSQL := "DELETE FROM portfolios WHERE id=$1 AND user_id=$2"
	_, err = trx.Exec(context.Background(), deleteSQL, portfolioID, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "DeletePortfolio",
			"Query":    deleteSQL,
			"Error":    err,
		}).Warn("DeletePortfolio failed")
		trx.Rollback(context.Background())
		return fiber.ErrInternalServerError
	}

	err = trx.Commit(context.Background())
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "DeletePortfolio",
			"Query":    deleteSQL,
			"Error":    err,
		}).Warn("DeletePortfolio failed")
		trx.Rollback(context.Background())
		return fiber.ErrInternalServerError
	}

	return c.JSON(fiber.Map{"status": "success"})
}
