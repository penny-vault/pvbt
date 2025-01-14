// Copyright 2021-2025
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

package filter

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/jackc/pgsql"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/rs/zerolog/log"
)

var (
	ErrEmptyFrom       = errors.New("strategy not found")
	ErrMalformedWhere  = errors.New("where clauses must take the form [OP].[value]")
	ErrUnknownOperator = errors.New("unknown operator")
)

type Database struct {
	PortfolioID string
	UserID      string
}

func setWhere(stmt *pgsql.SelectStatement, op string, field string, val string) error {
	switch op {
	case "eq":
		stmt.Where(fmt.Sprintf("%s = ?", field), val)
	case "gt":
		stmt.Where(fmt.Sprintf("%s > ?", field), val)
	case "gte":
		stmt.Where(fmt.Sprintf("%s >= ?", field), val)
	case "lt":
		stmt.Where(fmt.Sprintf("%s < ?", field), val)
	case "lte":
		stmt.Where(fmt.Sprintf("%s <= ?", field), val)
	case "neq":
		stmt.Where(fmt.Sprintf("%s <> ?", field), val)
	case "like":
		stmt.Where(fmt.Sprintf("%s like ?", field), val)
	case "ilike":
		stmt.Where(fmt.Sprintf("%s ilike ?", field), val)
	case "in":
		stmt.Where(fmt.Sprintf("%s in ?", field), val)
	case "is":
		stmt.Where(fmt.Sprintf("%s is ?", field), val)
	case "cs":
		stmt.Where(fmt.Sprintf("%s @> ?", field), val)
	case "cd":
		stmt.Where(fmt.Sprintf("%s <@ ?", field), val)
	case "ov":
		stmt.Where(fmt.Sprintf("%s && ?", field), val)
	case "sl":
		stmt.Where(fmt.Sprintf("%s<<?", field), val)
	case "sr":
		stmt.Where(fmt.Sprintf("%s >> ?", field), val)
	case "nxr":
		stmt.Where(fmt.Sprintf("%s &< ?", field), val)
	case "nxl":
		stmt.Where(fmt.Sprintf("%s &> ?", field), val)
	case "adj":
		stmt.Where(fmt.Sprintf("%s -|- ?", field), val)
	case "not":
		stmt.Where(fmt.Sprintf("%s not ?", field), val)
	default:
		return ErrUnknownOperator
	}
	return nil
}

func BuildQuery(from string, fields []string, safeFields []string, where map[string]string, order string) (string, []interface{}, error) {
	if strings.Compare(from, "") == 0 {
		return "", nil, ErrEmptyFrom
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
			return "", nil, ErrMalformedWhere
		}
		op, val := p[0], p[1]
		k = pgx.Identifier{k}.Sanitize()
		if err := setWhere(stmt, op, k, val); err != nil {
			return "", nil, err
		}
	}

	if order != "" {
		stmt.Order(order)
	}

	sql, args := pgsql.Build(stmt)
	return sql, args, nil
}

func (f *Database) GetMeasurements(field1 string, field2 string, since time.Time) ([]byte, error) {
	ctx := context.Background()
	where := make(map[string]string)
	fields := []string{"event_date", field1, field2}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sql, args, err := BuildQuery("portfolio_measurements", fields, []string{}, where, "event_date ASC")
	if err != nil {
		return nil, err
	}

	trx, _ := database.TrxForUser(ctx, f.UserID)
	rows, err := trx.Query(ctx, sql, args...)
	if err != nil {
		log.Warn().Stack().Err(err).Str("Query", sql).Msg("portfolio_measurements query failed")
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
			log.Warn().Stack().Err(err).Msg("row scan faile")
			return nil, err
		}
		meas.Items = append(meas.Items, &item)
	}

	data, err := meas.MarshalBinary()
	return data, err
}

func (f *Database) GetHoldings(frequency dataframe.Frequency, since time.Time) ([]byte, error) {
	ctx := context.Background()
	where := make(map[string]string)

	subLog := log.With().Str("Frequency", string(frequency)).Time("Since", since).Logger()
	subLog.Info().Msg("GetHoldings from database")

	periodReturnField := getPeriodReturnFieldForFrequency(frequency)
	fields := []string{"event_date", "holdings", periodReturnField, "justification", "strategy_value"}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sqlTmp, args, err := BuildQuery("portfolio_measurements", fields, []string{"LEAD(event_date) OVER (ORDER BY event_date) as next_date"}, where, "event_date DESC")
	if err != nil {
		return nil, err
	}

	var querySQL string
	switch frequency {
	case dataframe.Annually:
		querySQL = fmt.Sprintf("SELECT event_date, %s, LAG(holdings, 1) OVER (ORDER BY event_date) holdings, justification, strategy_value FROM (%s) AS subq WHERE extract('year' from next_date) != extract('year' from event_date) or next_date is null ORDER BY event_date ASC", periodReturnField, sqlTmp)
	case dataframe.Monthly:
		querySQL = fmt.Sprintf("SELECT event_date, %s, LAG(holdings, 1) OVER (ORDER BY event_date) holdings, justification, strategy_value FROM (%s) AS subq WHERE extract('month' from next_date) != extract('month' from event_date) or next_date is null ORDER BY event_date ASC", periodReturnField, sqlTmp)
	case dataframe.Weekly:
		querySQL = fmt.Sprintf("SELECT event_date, %s, LAG(holdings, 1) OVER (ORDER BY event_date) holdings, justification, strategy_value FROM (%s) AS subq WHERE extract('week' from next_date) != extract('week' from event_date) or next_date is null ORDER BY event_date ASC", periodReturnField, sqlTmp)
	case dataframe.Daily:
		querySQL = fmt.Sprintf("SELECT event_date, %s, LAG(holdings, 1) OVER (ORDER BY event_date) holdings, justification, strategy_value FROM (%s) AS subq WHERE extract('doy' from next_date) != extract('doy' from event_date) or next_date is null ORDER BY event_date ASC", periodReturnField, sqlTmp)
	default:
		querySQL = fmt.Sprintf("SELECT event_date, %s, LAG(holdings, 1) OVER (ORDER BY event_date) holdings, justification, strategy_value FROM (%s) AS subq WHERE extract('month' from next_date) != extract('month' from event_date) or next_date is null ORDER BY event_date ASC", periodReturnField, sqlTmp)
	}

	trx, _ := database.TrxForUser(ctx, f.UserID)
	rows, err := trx.Query(ctx, querySQL, args...)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("get measurements database query failed")
		return nil, err
	}

	h := portfolio.PortfolioHoldingItemList{
		Items: make([]*portfolio.PortfolioHoldingItem, 0, 100),
	}

	var lastJustification sql.NullString

	for rows.Next() {
		var item portfolio.PortfolioHoldingItem
		var holdings sql.NullString
		var justification sql.NullString

		item.Predicted = false
		err := rows.Scan(&item.Time, &item.PercentReturn, &holdings, &justification, &item.Value)
		if err != nil {
			subLog.Error().Stack().Err(err).Msg("could not create PortfolioHoldingItem")
			return nil, err
		}

		if holdings.Valid {
			err := json.Unmarshal([]byte(holdings.String), &item.Holdings)
			if err != nil {
				subLog.Error().Stack().Err(err).Time("EventDate", item.Time).Msg("could not unmarshal json holdings")
			}
		}

		if lastJustification.Valid {
			err := json.Unmarshal([]byte(lastJustification.String), &item.Justification)
			if err != nil {
				subLog.Error().Stack().Err(err).Time("EventDate", item.Time).Msg("could not unmarshal justification")
			}
		}

		lastJustification = justification
		h.Items = append(h.Items, &item)
	}

	// add predicted holding item
	var predicted portfolio.PortfolioHoldingItem
	var predictedRaw []byte
	err = trx.QueryRow(context.Background(), "SELECT predicted_bytes FROM portfolios WHERE id=$1", f.PortfolioID).Scan(&predictedRaw)

	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("query predicted value failed")
		return nil, err
	}
	err = predicted.UnmarshalBinary(predictedRaw)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("failed to unmarshal predicted data structure")
		return nil, err
	}

	nyc := common.GetTimezone()
	switch frequency {
	case dataframe.Annually:
		predicted.Time = time.Date(predicted.Time.Year()+1, predicted.Time.Month(), 1, 16, 0, 0, 0, nyc)
	case dataframe.Monthly:
		predicted.Time = time.Date(predicted.Time.Year(), predicted.Time.Month()+1, 1, 16, 0, 0, 0, nyc)
	default:
		// no adjustment necessary for other frequencies
	}

	h.Items = append(h.Items, &predicted)

	data, err := h.MarshalBinary()
	return data, err
}

func (f *Database) GetTransactions(since time.Time) ([]byte, error) {
	ctx := context.Background()
	subLog := log.With().Time("Since", since).Logger()
	where := make(map[string]string)
	tz := common.GetTimezone()
	fields := []string{"event_date", "id", "cleared", "commission", "composite_figi", "justification",
		"transaction_type", "memo", "price_per_share", "num_shares", "gain_loss", "source", "source_id", "tags",
		"tax_type", "ticker", "total_value"}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sqlQuery, args, err := BuildQuery("portfolio_transactions", fields, []string{}, where, "event_date ASC")
	if err != nil {
		return nil, err
	}

	trx, _ := database.TrxForUser(ctx, f.UserID)
	rows, err := trx.Query(ctx, sqlQuery, args...)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("portfolio_transactions query failed")
		return nil, err
	}

	h := portfolio.PortfolioTransactionList{
		Items: make([]*portfolio.Transaction, 0, 100),
	}

	for rows.Next() {
		var t portfolio.Transaction

		var compositeFIGI pgtype.Text
		var memo pgtype.Text
		var taxDisposition pgtype.Text
		var sourceID pgtype.Bytea

		err := rows.Scan(
			&t.Date,
			&t.ID,
			&t.Cleared,
			&t.Commission,
			&compositeFIGI,
			&t.Justification,
			&t.Kind,
			&memo,
			&t.PricePerShare,
			&t.Shares,
			&t.GainLoss,
			&t.Source,
			&sourceID,
			&t.Tags,
			&taxDisposition,
			&t.Ticker,
			&t.TotalValue,
		)

		if err != nil {
			subLog.Error().Stack().Err(err).Msg("error scanning transaction")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return nil, err
		}

		if compositeFIGI.Status == pgtype.Present {
			t.CompositeFIGI = compositeFIGI.String
		}
		if memo.Status == pgtype.Present {
			t.Memo = memo.String
		}
		if taxDisposition.Status == pgtype.Present {
			t.TaxDisposition = taxDisposition.String
		}
		if sourceID.Status == pgtype.Present {
			t.SourceID = hex.EncodeToString(sourceID.Bytes)
		}

		t.Date = time.Date(t.Date.Year(), t.Date.Month(), t.Date.Day(), 16, 0, 0, 0, tz)
		h.Items = append(h.Items, &t)
	}

	data, err := h.MarshalBinary()

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit transaction to database")
	}

	return data, err
}
