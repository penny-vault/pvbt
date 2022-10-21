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

package database

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// types

type PgxIface interface {
	Begin(context.Context) (pgx.Tx, error)
}

var (
	ErrEmptyUserID = errors.New("userID cannot be an empty string")
)

const (
	TWRRYtd    = "twrr_ytd"
	TWRRMtd    = "twrr_mtd"
	TWRRWtd    = "twrr_wtd"
	TWRROneDay = "twrr_1d"
)

// Private

var pool PgxIface
var openTransactions map[string]string

func createUser(ctx context.Context, userID string) error {
	if userID == "" {
		log.Error().Stack().Msg("userID cannot be an empty string")
		return ErrEmptyUserID
	}

	subLog := log.With().Str("UserID", userID).Logger()
	subLog.Info().Msg("creating new role")

	trx, err := pool.Begin(ctx)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("could not create new transaction")
		return err
	}

	// Make sure the current role is pvapi
	_, err = trx.Exec(ctx, "SET ROLE pvapi")
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("could not switch to pvapi role")
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return err
	}

	// Create the role
	// NOTE: We have to do our own sanitization because postgresql can only do sanitization on
	// select, insert, update, and delete queries
	ident := pgx.Identifier{userID}
	sql := fmt.Sprintf("CREATE ROLE %s WITH nologin IN ROLE pvuser;", ident.Sanitize())
	_, err = trx.Exec(ctx, sql)
	if err != nil {
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Str("Query", sql).Msg("could not rollback transaction")
		}
		subLog.Error().Stack().Err(err).Str("Query", sql).Msg("failed to create role")
		return err
	}

	// Grant privileges
	// NOTE: We have to do our own sanitization because postgresql can only do sanitization on
	// select, insert, update, and delete queries
	sql = fmt.Sprintf("GRANT %s TO pvapi;", ident.Sanitize())
	_, err = trx.Exec(ctx, sql)
	if err != nil {
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Str("Query", sql).Msg("could not rollback transaction")
		}
		subLog.Error().Stack().Err(err).Str("Query", sql).Msg("failed to grant privileges to role")
		return err
	}

	err = trx.Commit(ctx)
	if err != nil {
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Str("Query", sql).Msg("could not rollback transaction")
		}
		subLog.Error().Stack().Err(err).Msg("failed to commit changes")
		return err
	}

	return nil
}

// Public

func SetPool(myPool PgxIface) {
	openTransactions = make(map[string]string)
	pool = myPool
}

func Connect(ctx context.Context) error {
	var err error
	myPool, err := pgxpool.Connect(ctx, viper.GetString("database.url"))
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not connect to pool")
		return err
	}
	if err = myPool.Ping(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not ping database server")
		return err
	}
	SetPool(myPool)
	return nil
}

// LogOpenTransactions writes an INFO log for each open transaction
func LogOpenTransactions() {
	for k, v := range openTransactions {
		log.Info().Str("TrxId", k).Str("Caller", v).Msg("open transaction")
	}
}

// TrxForUser creates a transaction with the appropriate user set
// NOTE: the default use is pvapi which only has enough privileges to create new roles and switch to them.
// Any kind of real work must be done with a user role which limits access to only that user
func TrxForUser(ctx context.Context, userID string) (pgx.Tx, error) {
	trx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}

	// record transactions in openTransaction log
	_, file, lineno, ok := runtime.Caller(1)
	caller := fmt.Sprintf("[%v] %s:%d", ok, file, lineno)
	trxID := uuid.New().String()
	openTransactions[trxID] = caller

	wrappedTrx := &PvDbTx{
		id:   trxID,
		user: userID,
		tx:   trx,
	}

	subLog := log.With().Str("UserID", userID).Logger()

	// set user
	ident := pgx.Identifier{userID}
	sql := fmt.Sprintf("SET ROLE %s", ident.Sanitize())
	_, err = wrappedTrx.Exec(ctx, sql)
	if err != nil {
		// user doesn't exist -- create it
		subLog.Warn().Stack().Err(err).Msg("role does not exist")
		if err := wrappedTrx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
			return nil, err
		}
		err = createUser(ctx, userID)
		if err != nil {
			log.Error().Stack().Err(err).Msg("could not create user")
			return nil, err
		}
		return TrxForUser(ctx, userID)
	}

	return wrappedTrx, nil
}

// Get a list of users in the pvapi role
func GetUsers(ctx context.Context) ([]string, error) {
	trx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("could not begin transaction")
		return nil, err
	}

	sql := `WITH RECURSIVE cte AS (
		SELECT oid FROM pg_roles WHERE rolname = $1
		UNION ALL
			SELECT m.roleid
			FROM cte JOIN pg_auth_members m ON m.member = cte.oid
	)
	SELECT oid::regrole::text AS rolename FROM cte;`
	rows, err := trx.Query(ctx, sql, "pvapi")
	if err != nil {
		log.Warn().Stack().Err(err).Str("Query", sql).Msg("get list of database roles failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback tranasaction")
		}
		return nil, err
	}

	users := make([]string, 0, 100)
	for rows.Next() {
		var roleName string
		err := rows.Scan(&roleName)
		if err != nil {
			log.Warn().Stack().Err(err).Str("Query", sql).Msg("GetUser scan failed")
			continue
		}

		roleName = strings.Trim(roleName, "\"")
		if roleName == "pvapi" || roleName == "pvanon" || roleName == "pvhealth" || roleName == "pvuser" {
			continue
		}
		users = append(users, roleName)
	}

	err = rows.Err()
	if err != nil {
		log.Warn().Stack().Err(err).Str("Query", sql).Msg("GetUser query read failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback tranasaction")
		}
		return nil, err
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("could not commit transaction")
	}

	return users, nil
}
