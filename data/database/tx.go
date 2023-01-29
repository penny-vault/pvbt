// Copyright 2021-2023
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

// Wrapper around a pgx transaction to help debug if transactions are leaking

import (
	"context"
	"errors"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog/log"
)

var (
	ErrUnsupported = errors.New("unsupported function")
)

type PvDbTx struct {
	id   string
	user string
	tx   pgx.Tx
}

// Begin starts a pseudo nested transaction.
func (t *PvDbTx) Begin(ctx context.Context) (pgx.Tx, error) {
	log.Panic().Msg("sub-transactions not supported in pvdb")
	return nil, ErrUnsupported
}

// BeginFunc starts a pseudo nested transaction and executes f. If f does not return an err the pseudo nested
// transaction will be committed. If it does then it will be rolled back.
func (t *PvDbTx) BeginFunc(ctx context.Context, f func(pgx.Tx) error) (err error) {
	log.Panic().Msg("sub-transactions not supported in pvdb")
	return ErrUnsupported
}

// Commit commits the transaction if this is a real transaction or releases the savepoint if this is a pseudo nested
// transaction. Commit will return ErrTxClosed if the Tx is already closed, but is otherwise safe to call multiple
// times. If the commit fails with a rollback status (e.g. the transaction was already in a broken state) then
// ErrTxCommitRollback will be returned.
func (t *PvDbTx) Commit(ctx context.Context) error {
	// remove id from tracking
	delete(openTransactions, t.id)
	return t.tx.Commit(ctx)
}

// Rollback rolls back the transaction if this is a real transaction or rolls back to the savepoint if this is a
// pseudo nested transaction. Rollback will return ErrTxClosed if the Tx is already closed, but is otherwise safe to
// call multiple times. Hence, a defer tx.Rollback() is safe even if tx.Commit() will be called first in a non-error
// condition. Any other failure of a real transaction will result in the connection being closed.
func (t *PvDbTx) Rollback(ctx context.Context) error {
	delete(openTransactions, t.id)
	return t.tx.Rollback(ctx)
}

func (t *PvDbTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return t.tx.CopyFrom(ctx, tableName, columnNames, rowSrc)
}

func (t *PvDbTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return t.tx.SendBatch(ctx, b)
}
func (t *PvDbTx) LargeObjects() pgx.LargeObjects {
	return t.tx.LargeObjects()
}

func (t *PvDbTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return t.tx.Prepare(ctx, name, sql)
}

func (t *PvDbTx) Exec(ctx context.Context, sql string, arguments ...interface{}) (commandTag pgconn.CommandTag, err error) {
	return t.tx.Exec(ctx, sql, arguments...)
}

func (t *PvDbTx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return t.tx.Query(ctx, sql, args...)
}

func (t *PvDbTx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return t.tx.QueryRow(ctx, sql, args...)
}

func (t *PvDbTx) QueryFunc(ctx context.Context, sql string, args []interface{}, scans []interface{}, f func(pgx.QueryFuncRow) error) (pgconn.CommandTag, error) {
	return t.tx.QueryFunc(ctx, sql, args, scans, f)
}

// Conn returns the underlying *Conn that on which this transaction is executing.
func (t *PvDbTx) Conn() *pgx.Conn {
	return t.tx.Conn()
}
