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
	var j string
	where := make(map[string]string)
	fields := []string{field1, field2}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sql, args, err := BuildQuery("portfolio_measurement_v1", fields, []string{}, where, "event_date DESC")
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	trx, _ := database.TrxForUser(f.UserID)
	err = trx.QueryRow(context.Background(), fmt.Sprintf(`
	select array_to_json(array_agg(row_to_json(tbl))) as res
    from (
		%s
    ) tbl
	`, sql), args...).Scan(&j)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	meas := portfolio.PerformanceMeasurementItemList{
		FieldNames: fields,
		Items:      make([]*portfolio.PerformanceMeasurementItem, 0, 100),
	}
	err = json.Unmarshal([]byte(j), &meas.Items)
	if err != nil {
		return nil, err
	}

	data, err := meas.MarshalBinary()
	return data, err
}

func (f *FilterDatabase) GetHoldings(frequency string, since time.Time) ([]byte, error) {
	var j string
	where := make(map[string]string)

	var periodReturn string
	switch frequency {
	case "annually":
		periodReturn = "twrr_1yr"
	case "monthly":
		periodReturn = "twrr_1mo"
	case "daily":
		periodReturn = "twrr_1d"
	default:
		periodReturn = "twrr_1mo"
	}
	fields := []string{"event_date", "holdings", periodReturn, "strategy_value"}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sqlTmp, args, err := BuildQuery("portfolio_measurement_v1", fields, []string{"LEAD(event_date) OVER (ORDER BY event_date) as next_date"}, where, "event_date DESC")
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	sql := fmt.Sprintf("SELECT event_date, %s, holdings, strategy_value FROM (%s) AS subq WHERE extract('month' from next_date) != extract('month' from event_date)", periodReturn, sqlTmp)

	trx, _ := database.TrxForUser(f.UserID)
	err = trx.QueryRow(context.Background(), fmt.Sprintf(`
			select array_to_json(array_agg(row_to_json(tbl))) as res
		    from (
				%s
		    ) tbl
			`, sql), args...).Scan(&j)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	h := portfolio.PortfolioHoldingItemList{
		Items: make([]*portfolio.PortfolioHoldingItem, 0, 100),
	}
	err = json.Unmarshal([]byte(j), &h.Items)
	if err != nil {
		return nil, err
	}

	data, err := h.MarshalBinary()
	return data, err
}

func (f *FilterDatabase) GetTransactions(since time.Time) ([]byte, error) {
	var j string
	where := make(map[string]string)

	fields := []string{"event_date", "id", "cleared", "commission", "composite_figi", "justification",
		"transaction_type", "memo", "price_per_share", "num_shares", "source", "source_id", "tags",
		"tax_type", "ticker", "total_value"}

	where["portfolio_id"] = fmt.Sprintf("eq.%s", f.PortfolioID)
	where["event_date"] = fmt.Sprintf("gte.%s", since.Format("2006-01-02T15:04:05.000000-0200"))

	sql, args, err := BuildQuery("portfolio_transaction_v1", fields, []string{""}, where, "event_date DESC")
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	trx, _ := database.TrxForUser(f.UserID)
	err = trx.QueryRow(context.Background(), fmt.Sprintf(`
			select array_to_json(array_agg(row_to_json(tbl))) as res
		    from (
				%s
		    ) tbl
			`, sql), args...).Scan(&j)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	h := portfolio.PortfolioTransactionList{
		Items: make([]*portfolio.Transaction, 0, 100),
	}
	err = json.Unmarshal([]byte(j), &h.Items)
	if err != nil {
		return nil, err
	}

	data, err := h.MarshalBinary()
	return data, err
}
