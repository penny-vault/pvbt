package handler

import (
	"context"
	"database/sql"
	"fmt"
	"main/database"
	"main/filter"
	"main/portfolio"
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
	YTDReturn          sql.NullFloat64        `json:"ytdReturn"`
	CAGRSinceInception sql.NullFloat64        `json:"cagrSinceInception"`
	Notifications      int                    `json:"notifications"`
	Created            int64                  `json:"created"`
	LastChanged        int64                  `json:"lastChanged"`
}

// GetPortfolio get a portfolio
// @Description Retrieve a portfolio saved on the server
// @Id GetPortfolio
// @Produce json
// @Param id path string true "id of porfolio to retrieve"
func GetPortfolio(c *fiber.Ctx) error {
	portfolioID := c.Params("id")

	userID := c.Locals("userID").(string)

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolio_v1 WHERE id=$1 AND user_id=$2`
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
	return c.JSON(p)
}

func GetPortfolioMeasurements(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	userID := c.Locals("userID").(string)

	f := filter.New(portfolioID, userID)

	field1 := c.Query("field1", "strategy_growth_of_10k")
	field2 := c.Query("field2", "benchmark_growth_of_10k")

	sinceStr := c.Query("since", "0")

	s := time.Now()

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
	e := time.Now()
	fmt.Printf("query duration: %.4f\n", e.Sub(s).Seconds())

	return c.Send(data)
}

func GetPortfolioTransactions(c *fiber.Ctx) error {
	portfolioIDStr := c.Params("id")
	portfolioID, err := uuid.Parse(portfolioIDStr)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":       "GetPortfolioMeasurements",
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
	return c.JSON(p)
}

// ListPortfolios list all portfolios for logged in user
func ListPortfolios(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolio_v1 WHERE user_id=$1 ORDER BY name, created`
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

// CreatePortfolio new portfolio
func CreatePortfolio(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	params := PortfolioResponse{}
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		log.Warnf("Bad request: %s", err)
		return fiber.ErrBadRequest
	}

	// Save to database
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "CreatePortfolio",
			"Error":    err,
			"UserID":   userID,
		}).Error("unable to get database transaction for user")
		return fiber.ErrServiceUnavailable
	}

	portfolioID := uuid.New()
	portfolioSQL := `INSERT INTO portfolio_v1 ("id", "user_id", "name", "strategy_shortcode", "arguments", "start_date") VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = trx.Exec(context.Background(), portfolioSQL, portfolioID, userID, params.Name, params.Strategy, params.Arguments, time.Unix(params.StartDate, 0))
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "CreatePortfolio",
			"Strategy": params.Strategy,
			"Error":    err,
			"Body":     string(c.Body()),
			"Query":    portfolioSQL,
		}).Warn("Failed to create portfolio")
		trx.Rollback(context.Background())
		return fiber.ErrBadRequest
	}

	err = trx.Commit(context.Background())
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint": "CreatePortfolio",
			"Strategy": params.Strategy,
			"Error":    err,
			"Body":     string(c.Body()),
			"Query":    portfolioSQL,
		}).Warn("Failed to create portfolio")
		trx.Rollback(context.Background())
		return fiber.ErrInternalServerError
	}
	return c.JSON(PortfolioResponse{
		ID:        portfolioID,
		Name:      params.Name,
		StartDate: params.StartDate,
		Strategy:  params.Strategy,
		Arguments: params.Arguments,
	})
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

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, ytd_return, cagr_since_inception, notifications, extract(epoch from created)::int as created, extract(epoch from lastchanged)::int as lastchanged FROM portfolio_v1 WHERE id=$1 AND user_id=$2`
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

	updateSQL := `UPDATE portfolio_v1 SET name=$1, notifications=$2 WHERE id=$3 AND user_id=$4`
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

	deleteSQL := "DELETE FROM portfolio_v1 WHERE id=$1 AND user_id=$2"
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
