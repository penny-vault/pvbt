// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
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

package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// types

type PgxIface interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Private

var pool PgxIface

func createUser(userID string) error {
	if userID == "" {
		log.Error("userID cannot be an empty string")
		return errors.New("userID cannot be an empty string")
	}

	log.WithFields(log.Fields{
		"UserID": userID,
	}).Info("creating new role")
	trx, err := pool.Begin(context.Background())
	if err != nil {
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
		}).Error("could not create new transaction")
		return err
	}

	// Make sure the current role is pvapi
	_, err = trx.Exec(context.Background(), "SET ROLE pvapi")
	if err != nil {
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
		}).Error("could not switch to pvapi role")
		trx.Rollback(context.Background())
		return err
	}

	// Create the role
	// NOTE: We have to do our own sanitization because postgresql can only do sanitization on
	// select, insert, update, and delete queries
	ident := pgx.Identifier{userID}
	sql := fmt.Sprintf("CREATE ROLE %s WITH nologin IN ROLE pvuser;", ident.Sanitize())
	_, err = trx.Exec(context.Background(), sql)
	if err != nil {
		trx.Rollback(context.Background())
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
			"Query":  sql,
		}).Error("failed to create role")
		return err
	}

	// Grant privileges
	// NOTE: We have to do our own sanitization because postgresql can only do sanitization on
	// select, insert, update, and delete queries
	sql = fmt.Sprintf("GRANT %s TO pvapi;", ident.Sanitize())
	_, err = trx.Exec(context.Background(), sql)
	if err != nil {
		trx.Rollback(context.Background())
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
			"Query":  sql,
		}).Error("failed to grant priveleges to role")
		return err
	}

	err = trx.Commit(context.Background())
	if err != nil {
		trx.Rollback(context.Background())
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
		}).Error("failed to commit changes")
		return err
	}

	return nil
}

// Public

func SetPool(myPool PgxIface) {
	pool = myPool
}

func Connect() error {
	var err error
	myPool, err := pgxpool.Connect(context.Background(), viper.GetString("database.url"))
	if err != nil {
		return err
	}
	if err = myPool.Ping(context.Background()); err != nil {
		return err
	}
	pool = myPool
	return nil
}

// Create a trx with the appropriate user set
// NOTE: the default use is pvapi which only has enough priveleges to create new roles and switch to them.
// Any kind of real work must be done with a user role which limits access to only that user
func TrxForUser(userID string) (pgx.Tx, error) {
	trx, err := pool.Begin(context.Background())
	if err != nil {
		return nil, err
	}

	// set user
	ident := pgx.Identifier{userID}
	sql := fmt.Sprintf("SET ROLE %s", ident.Sanitize())
	_, err = trx.Exec(context.Background(), sql)
	if err != nil {
		// user doesn't exist -- create it
		log.WithFields(log.Fields{
			"UserID": userID,
			"Error":  err,
		}).Warn("role does not exist")
		trx.Rollback(context.Background())
		err = createUser(userID)
		if err != nil {
			return nil, err
		}
		return TrxForUser(userID)
	}

	return trx, nil
}

// Get a list of users in the pvapi role
func GetUsers() ([]string, error) {
	trx, err := pool.Begin(context.Background())
	if err != nil {
		return nil, err
	}

	sql := `WITH RECURSIVE cte AS (
		SELECT oid FROM pg_roles WHERE rolname = $1
		UNION ALL
			SELECT m.roleid
			FROM cte JOIN pg_auth_members m ON m.member = cte.oid
	)
	SELECT oid::regrole::text AS rolename FROM cte;`
	rows, err := trx.Query(context.Background(), sql, "pvapi")
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"Query": sql,
		}).Warn("get list of database roles failed")
		trx.Rollback(context.Background())
		return nil, err
	}

	users := make([]string, 0, 100)
	for rows.Next() {
		var roleName string
		err := rows.Scan(&roleName)
		if err != nil {
			log.WithFields(log.Fields{
				"Error": err,
				"Query": sql,
			}).Warn("GetUser scan failed")
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
		log.WithFields(log.Fields{
			"Error": err,
			"Query": sql,
		}).Warn("GetUser query read failed")
		trx.Rollback(context.Background())
		return nil, err
	}

	return users, nil
}
