package handler

import (
	"database/sql"
	"encoding/json"
	"main/database"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx/types"
	log "github.com/sirupsen/logrus"
)

type portfolioCredentials struct {
	Tiingo string `json:"tiingo"`
}

type portfolio struct {
	ID                 uuid.UUID       `json:"id"`
	Name               string          `json:"name"`
	Strategy           string          `json:"strategy"`
	Arguments          types.JSONText  `json:"arguments"`
	YTDReturn          sql.NullFloat64 `json:"ytd_return"`
	CAGRSinceInception sql.NullFloat64 `json:"cagr_since_inception"`
	Notifications      int             `json:"notifications"`
	Created            float64         `json:"created"`
	LastChanged        float64         `json:"lastchanged"`
}

// GetPortfolio get a portfolio
func GetPortfolio(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	userID := claims["sub"].(string)

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, ytd_return, cagr_since_inception, notifications, extract(epoch from created) as created, extract(epoch from lastchanged) as lastchanged FROM portfolio WHERE id=$1 AND userid=$2`
	row := database.Conn.QueryRow(portfolioSQL, portfolioID, userID)
	p := portfolio{}
	err := row.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
	if err != nil {
		log.Warnf("GetPortfolio %s failed: %s", portfolioID, err)
		return fiber.ErrNotFound
	}

	return c.JSON(p)
}

// ListPortfolios list all portfolios for logged in user
func ListPortfolios(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	userID := claims["sub"].(string)

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, ytd_return, cagr_since_inception, notifications, extract(epoch from created) as created, extract(epoch from lastchanged) as lastchanged FROM portfolio WHERE userid=$1 ORDER BY name, created`
	rows, err := database.Conn.Query(portfolioSQL, userID)
	if err != nil {
		log.Warnf("ListPortfolio failed: %s", err)
		return fiber.ErrNotFound
	}

	portfolios := []portfolio{}
	for rows.Next() {
		p := portfolio{}
		err := rows.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
		if err != nil {
			log.Warnf("ListPortfolio failed %s", err)
		}
		portfolios = append(portfolios, p)
	}

	err = rows.Err()
	if err != nil {
		log.Warnf("ListPortfolio failed: %s", err)
		return fiber.ErrNotFound
	}

	return c.JSON(portfolios)
}

// CreatePortfolio new portfolio
func CreatePortfolio(c *fiber.Ctx) error {
	// get tiingo token and userID from jwt claims
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	tiingoToken := claims["https://pennyvault.com/tiingo_token"].(string)
	userID := claims["sub"].(string)
	credentials := portfolioCredentials{
		Tiingo: tiingoToken,
	}
	credentialsJSON, _ := json.Marshal(credentials)

	params := portfolio{}
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		log.Warnf("Bad request: %s", err)
		return fiber.ErrBadRequest
	}

	// Save to database
	portfolioID := uuid.New()
	portfolioSQL := `INSERT INTO Portfolio ("id", "userid", "name", "strategy_shortcode", "arguments", "credentials") VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := database.Conn.Exec(portfolioSQL, portfolioID, userID, params.Name, params.Strategy, params.Arguments, credentialsJSON)
	if err != nil {
		log.Warnf("Failed to create portfolio for %s: %s", params.Strategy, err)
		return fiber.ErrBadRequest
	}

	return c.JSON(portfolio{
		ID:       portfolioID,
		Name:     params.Name,
		Strategy: params.Strategy,
	})
}

// UpdatePortfolio update portfolio
func UpdatePortfolio(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	userID := claims["sub"].(string)

	params := portfolio{}
	if err := json.Unmarshal(c.Body(), &params); err != nil {
		log.Warnf("UpdatePortfolio bad request: %s, for portfolio: %s", err, portfolioID)
		return fiber.ErrBadRequest
	}

	portfolioSQL := `SELECT id, name, strategy_shortcode, arguments, ytd_return, cagr_since_inception, notifications, extract(epoch from created) as created, extract(epoch from lastchanged) as lastchanged FROM portfolio WHERE id=$1 AND userid=$2`
	row := database.Conn.QueryRow(portfolioSQL, portfolioID, userID)
	p := portfolio{}
	err := row.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
	if err != nil {
		log.Warnf("UpdatePortfolio %s failed: %s", portfolioID, err)
		return fiber.ErrNotFound
	}

	if params.Name == "" {
		params.Name = p.Name
	}

	if params.Notifications == 0 {
		params.Notifications = p.Notifications
	}

	updateSQL := `UPDATE Portfolio SET name=$1, notifications=$2 WHERE id=$3 AND userid=$4`
	_, err = database.Conn.Exec(updateSQL, params.Name, params.Notifications, portfolioID, userID)
	if err != nil {
		log.Warnf("UpdatePortfolio SQL update failed: %s for portfolio: %s", err, portfolioID)
		return fiber.ErrInternalServerError
	}

	row = database.Conn.QueryRow(portfolioSQL, portfolioID, userID)
	p = portfolio{}
	err = row.Scan(&p.ID, &p.Name, &p.Strategy, &p.Arguments, &p.YTDReturn, &p.CAGRSinceInception, &p.Notifications, &p.Created, &p.LastChanged)
	if err != nil {
		log.Warnf("UpdatePortfolio %s failed: %s", portfolioID, err)
		return fiber.ErrInternalServerError
	}

	return c.JSON(p)
}

// DeletePortfolio delete portfolio
func DeletePortfolio(c *fiber.Ctx) error {
	portfolioID := c.Params("id")
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	userID := claims["sub"].(string)

	deleteSQL := "DELETE FROM Portfolio WHERE id=$1 AND userid=$2"
	_, err := database.Conn.Exec(deleteSQL, portfolioID, userID)
	if err != nil {
		log.Warnf("DeletePortfolio delete failed: %s, for portfolio: %s", err, portfolioID)
		return fiber.ErrInternalServerError
	}

	return c.JSON(fiber.Map{"status": "success"})
}
