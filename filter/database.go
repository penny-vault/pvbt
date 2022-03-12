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

package filter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"main/database"
	"main/portfolio"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"github.com/jackc/pgsql"
	"github.com/jackc/pgx/v4"

	log "github.com/sirupsen/logrus"
)

type FilterDatabase struct {
	PortfolioID string
	UserID      string
}

func BuildQuery(from string, fields []string, safeFields []string, where map[string]string, order string) (string, []interface{}, error) {
	if strings.Compare(from, "") == 0 {
		return "", nil, errors.New("'from' cannot be empty")
	}
	stmt := &pgsql.SelectStatement{}
	for _, ff := range fields {
		stmt.Select(pgx.Identifier{ff}.Sanitize())
	}

	for _, ff := range safeFields {
		stmt.Select(ff)
	}

	stmt.From(pgx.Identifier{from}.Sanitize())

	for k, v := range where {
		p := strings.SplitN(v, ".", 2)
		if len(p) != 2 {
			return "", nil, errors.New("where clauses must take the form [OP].[value]")
		}
		op, val := p[0], p[1]
		k = pgx.Identifier{k}.Sanitize()
		switch op {
		case "eq":
			stmt.Where(fmt.Sprintf("%s = ?", k), val)
		case "gt":
			stmt.Where(fmt.Sprintf("%s > ?", k), val)
		case "gte":
			stmt.Where(fmt.Sprintf("%s >= ?", k), val)
		case "lt":
			stmt.Where(fmt.Sprintf("%s < ?", k), val)
		case "lte":
			stmt.Where(fmt.Sprintf("%s <= ?", k), val)
		case "neq":
			stmt.Where(fmt.Sprintf("%s <> ?", k), val)
		case "like":
			stmt.Where(fmt.Sprintf("%s like ?", k), val)
		case "ilike":
			stmt.Where(fmt.Sprintf("%s ilike ?", k), val)
		case "in":
			stmt.Where(fmt.Sprintf("%s in ?", k), val)
		case "is":
			stmt.Where(fmt.Sprintf("%s is ?", k), val)
		case "cs":
			stmt.Where(fmt.Sprintf("%s @> ?", k), val)
		case "cd":
			stmt.Where(fmt.Sprintf("%s <@ ?", k), val)
		case "ov":
			stmt.Where(fmt.Sprintf("%s && ?", k), val)
		case "sl":
			stmt.Where(fmt.Sprintf("%s<<?", k), val)
		case "sr":
			stmt.Where(fmt.Sprintf("%s >> ?", k), val)
		case "nxr":
			stmt.Where(fmt.Sprintf("%s &< ?", k), val)
		case "nxl":
			stmt.Where(fmt.Sprintf("%s &> ?", k), val)
		case "adj":
			stmt.Where(fmt.Sprintf("%s -|- ?", k), val)
		case "not":
			stmt.Where(fmt.Sprintf("%s not ?", k), val)
		default:
			return "", nil, errors.New("unrecognized operator")
		}
	}

	if order != "" {
		stmt.Order(order)
	}

	sql, args := pgsql.Build(stmt)
	return sql, args, nil
}

func (f *FilterDatabase) GetMeasurements(field1 string, field2 string, since time.Time) ([]byte, error) {
	where := make(map[string]string)
	fields := []string{"event_date", field1, field2}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sql, args, err := BuildQuery("portfolio_measurement_v1", fields, []string{}, where, "event_date ASC")
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	trx, _ := database.TrxForUser(f.UserID)
	rows, err := trx.Query(context.Background(), sql, args...)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	meas := portfolio.PerformanceMeasurementItemList{
		FieldNames: fields,
		Items:      make([]*portfolio.PerformanceMeasurementItem, 0, 100),
	}

	for rows.Next() {
		var item portfolio.PerformanceMeasurementItem

		err := rows.Scan(&item.Time, &item.Value1, &item.Value2)
		if err != nil {
			log.Warn(err)
			return nil, err
		}
		meas.Items = append(meas.Items, &item)
	}

	data, err := meas.MarshalBinary()
	return data, err
}

func (f *FilterDatabase) GetHoldings(frequency string, since time.Time) ([]byte, error) {
	where := make(map[string]string)

	var periodReturn string
	switch frequency {
	case "annually":
		periodReturn = "twrr_ytd"
	case "monthly":
		periodReturn = "twrr_mtd"
	case "weekly":
		periodReturn = "twrr_wtd"
	case "daily":
		periodReturn = "twrr_1d"
	default:
		periodReturn = "twrr_mtd"
	}
	fields := []string{"event_date", "holdings", periodReturn, "justification", "strategy_value"}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sqlTmp, args, err := BuildQuery("portfolio_measurement_v1", fields, []string{"LEAD(event_date) OVER (ORDER BY event_date) as next_date", "LEAD(justification) OVER (ORDER BY event_date) as next_justification"}, where, "event_date DESC")
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	var querySQL string
	switch frequency {
	case "annually":
		querySQL = fmt.Sprintf("SELECT event_date, %s, holdings, justification, strategy_value, next_justification FROM (%s) AS subq WHERE extract('year' from next_date) != extract('year' from event_date) or next_date is null ORDER BY event_date ASC", periodReturn, sqlTmp)
	case "monthly":
		querySQL = fmt.Sprintf("SELECT event_date, %s, holdings, justification, strategy_value, next_justification FROM (%s) AS subq WHERE extract('month' from next_date) != extract('month' from event_date) or next_date is null ORDER BY event_date ASC", periodReturn, sqlTmp)
	case "weekly":
		querySQL = fmt.Sprintf("SELECT event_date, %s, holdings, justification, strategy_value, next_justification FROM (%s) AS subq WHERE extract('week' from next_date) != extract('week' from event_date) or next_date is null ORDER BY event_date ASC", periodReturn, sqlTmp)
	case "daily":
		querySQL = fmt.Sprintf("SELECT event_date, %s, holdings, justification, strategy_value, next_justification FROM (%s) AS subq WHERE extract('doy' from next_date) != extract('doy' from event_date) or next_date is null ORDER BY event_date ASC", periodReturn, sqlTmp)
	default:
		querySQL = fmt.Sprintf("SELECT event_date, %s, holdings, justification, strategy_value, next_justification FROM (%s) AS subq WHERE extract('month' from next_date) != extract('month' from event_date) or next_date is null ORDER BY event_date ASC", periodReturn, sqlTmp)
	}

	trx, _ := database.TrxForUser(f.UserID)
	rows, err := trx.Query(context.Background(), querySQL, args...)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	h := portfolio.PortfolioHoldingItemList{
		Items: make([]*portfolio.PortfolioHoldingItem, 0, 100),
	}

	var useJustification sql.NullString

	for rows.Next() {
		var item portfolio.PortfolioHoldingItem
		var holdings sql.NullString
		var justification sql.NullString
		var nextJustification sql.NullString

		item.Predicted = false
		err := rows.Scan(&item.Time, &item.PercentReturn, &holdings, &justification, &item.Value, &nextJustification)
		if err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Error("Could not create PortfolioHoldingItem")
			return nil, err
		}

		if holdings.Valid {
			err := json.Unmarshal([]byte(holdings.String), &item.Holdings)
			if err != nil {
				log.WithFields(log.Fields{
					"EventDate": item.Time,
					"Error":     err,
				}).Error("Could not unmarshal json holdings")
			}
		}

		if useJustification.Valid {
			err := json.Unmarshal([]byte(useJustification.String), &item.Justification)
			if err != nil {
				log.WithFields(log.Fields{
					"EventDate": item.Time,
					"Error":     err,
				}).Error("Could not unmarshal justification")
			}
		} else if justification.Valid {
			err := json.Unmarshal([]byte(justification.String), &item.Justification)
			if err != nil {
				log.WithFields(log.Fields{
					"EventDate": item.Time,
					"Error":     err,
				}).Error("Could not unmarshal justification")
			}
		}

		useJustification = nextJustification

		h.Items = append(h.Items, &item)
	}

	// add predicted holding item
	var predicted portfolio.PortfolioHoldingItem
	var predictedRaw []byte
	err = trx.QueryRow(context.Background(), "SELECT predicted_bytes FROM portfolio_v1 WHERE id=$1", f.PortfolioID).Scan(&predictedRaw)

	if err != nil {
		log.Warn(err)
		return nil, err
	}
	err = predicted.UnmarshalBinary(predictedRaw)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	switch frequency {
	case "annually":
		predicted.Time = predicted.Time.AddDate(1, 0, 0)
	case "monthly":
		predicted.Time = predicted.Time.AddDate(0, 1, 0)
	}

	h.Items = append(h.Items, &predicted)

	data, err := h.MarshalBinary()
	return data, err
}

func (f *FilterDatabase) GetTransactions(since time.Time) ([]byte, error) {
	where := make(map[string]string)
	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time

	fields := []string{"event_date", "id", "cleared", "commission", "composite_figi", "justification",
		"transaction_type", "memo", "price_per_share", "num_shares", "source", "source_id", "tags",
		"tax_type", "ticker", "total_value"}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sqlQuery, args, err := BuildQuery("portfolio_transaction_v1", fields, []string{}, where, "event_date ASC")
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	trx, _ := database.TrxForUser(f.UserID)
	rows, err := trx.Query(context.Background(), sqlQuery, args...)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	h := portfolio.PortfolioTransactionList{
		Items: make([]*portfolio.Transaction, 0, 100),
	}

	for rows.Next() {
		var trx portfolio.Transaction
		err := rows.Scan(&trx.Date, &trx.ID, &trx.Cleared, &trx.Commission, &trx.CompositeFIGI, &trx.Justification, &trx.Kind, &trx.Memo, &trx.PricePerShare, &trx.Shares, &trx.Source, &trx.SourceID, &trx.Tags, &trx.TaxDisposition, &trx.Ticker, &trx.TotalValue)
		trx.Date = time.Date(trx.Date.Year(), trx.Date.Month(), trx.Date.Day(), 16, 0, 0, 0, tz)
		if err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Info("Error scanning transaction")
		}
		h.Items = append(h.Items, &trx)
	}

	data, err := h.MarshalBinary()
	return data, err
}
