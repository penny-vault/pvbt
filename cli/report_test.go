// Copyright 2021-2026
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

package cli

import (
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// writeBacktestDB builds a small account with a two-day equity curve and
// persists it to a SQLite file, returning the path. It mirrors what a real
// backtest writes via Account.ToSQLite so the report command has a faithful
// fixture to render from.
func writeBacktestDB(dir string) string {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

	acct := portfolio.New(
		portfolio.WithCash(4500, time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)),
		portfolio.WithAllMetrics(),
	)
	acct.SetMetadata(portfolio.MetaStrategyName, "test")

	t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	acct.Record(portfolio.Transaction{
		Date:   t0,
		Asset:  spy,
		Type:   asset.BuyTransaction,
		Qty:    10,
		Price:  450.0,
		Amount: -4500.0,
	})

	buildClose := func(t time.Time, price float64) *data.DataFrame {
		df, err := data.NewDataFrame(
			[]time.Time{t},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.AdjClose},
			data.Daily,
			[][]float64{{price}, {price}},
		)
		Expect(err).NotTo(HaveOccurred())

		return df
	}

	acct.UpdatePrices(buildClose(t0, 450.0))
	acct.UpdatePrices(buildClose(time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), 455.0))

	path := filepath.Join(dir, "test-backtest.db")
	Expect(acct.ToSQLite(path)).To(Succeed())

	return path
}

var _ = Describe("report command", func() {
	It("renders a report from a saved backtest database", func() {
		dbPath := writeBacktestDB(GinkgoT().TempDir())

		strategy := &testStrategy{}
		rootCmd, cleanup := newRootCmd(strategy)
		defer cleanup()

		var out strings.Builder
		rootCmd.SetOut(&out)
		rootCmd.SetArgs([]string{"report", dbPath})

		Expect(rootCmd.Execute()).To(Succeed())
		Expect(out.Len()).To(BeNumerically(">", 0))
	})

	It("returns an error when the database file does not exist", func() {
		strategy := &testStrategy{}
		rootCmd, cleanup := newRootCmd(strategy)
		defer cleanup()

		missing := filepath.Join(GinkgoT().TempDir(), "does-not-exist.db")
		rootCmd.SetArgs([]string{"report", missing})

		err := rootCmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("read backtest database"))
	})

	It("requires exactly one argument", func() {
		strategy := &testStrategy{}
		rootCmd, cleanup := newRootCmd(strategy)
		defer cleanup()

		rootCmd.SetArgs([]string{"report"})
		Expect(rootCmd.Execute()).To(HaveOccurred())
	})
})
