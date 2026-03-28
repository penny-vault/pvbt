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

package summary_test

import (
	"bytes"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/cli/summary"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// buildTestAccount constructs a portfolio.Account with a known equity curve
// by depositing/withdrawing cash and calling UpdatePrices at each step.
// The resulting Account has a fully populated internal perfData, so all
// PortfolioStats methods (AnnualReturns, DrawdownDetails, MonthlyReturns,
// etc.) function correctly.
func buildTestAccount(dates []time.Time, equityValues []float64) *portfolio.Account {
	Expect(len(dates)).To(Equal(len(equityValues)))
	Expect(len(dates)).To(BeNumerically(">=", 2))

	acct := portfolio.New(portfolio.WithCash(equityValues[0], dates[0]))
	acct.SetMetadata(portfolio.MetaStrategyName, "TestStrategy")
	acct.SetMetadata(portfolio.MetaStrategyVersion, "1.0.0")
	acct.SetMetadata(portfolio.MetaRunInitialCash, "10000")
	acct.SetMetadata(portfolio.MetaRunElapsed, "1s")

	// Build an empty single-row price DataFrame for UpdatePrices.
	// It just needs a valid timestamp; the Account will record
	// cash as the total equity since there are no holdings.
	buildPriceDF := func(ts time.Time) *data.DataFrame {
		dummyAsset := asset.Asset{CompositeFigi: "_DUMMY_", Ticker: "_DUMMY_"}
		df, err := data.NewDataFrame(
			[]time.Time{ts},
			[]asset.Asset{dummyAsset},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{0}},
		)
		Expect(err).NotTo(HaveOccurred())

		return df
	}

	// Record the first data point.
	acct.UpdatePrices(buildPriceDF(dates[0]))

	// For each subsequent date, adjust cash to reach the target equity
	// value, then call UpdatePrices to record the new equity.
	for idx := 1; idx < len(dates); idx++ {
		delta := equityValues[idx] - equityValues[idx-1]
		if delta != 0 {
			txType := asset.DepositTransaction
			amount := delta

			if delta < 0 {
				txType = asset.WithdrawalTransaction
			}

			acct.Record(portfolio.Transaction{
				Date:   dates[idx],
				Type:   txType,
				Amount: amount,
			})
		}

		acct.UpdatePrices(buildPriceDF(dates[idx]))
	}

	return acct
}

var _ = BeforeSuite(func() {
	log.Logger = zerolog.New(GinkgoWriter).With().Timestamp().Logger()
})

var _ = Describe("Render", func() {
	var (
		dates        []time.Time
		equityValues []float64
	)

	BeforeEach(func() {
		dates = []time.Time{
			time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 4, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		}
		equityValues = []float64{10_000, 10_200, 10_500, 10_100, 10_800, 11_000, 11_500}
	})

	It("produces non-empty output without error", func() {
		acct := buildTestAccount(dates, equityValues)

		var buf bytes.Buffer
		err := summary.Render(acct, &buf)
		Expect(err).NotTo(HaveOccurred())
		Expect(buf.String()).NotTo(BeEmpty())
	})

	It("contains expected section headings", func() {
		acct := buildTestAccount(dates, equityValues)

		var buf bytes.Buffer
		err := summary.Render(acct, &buf)
		Expect(err).NotTo(HaveOccurred())

		output := buf.String()

		expectedSections := []string{
			"Performance",
			"Recent Returns",
			"Returns",
			"Annual Returns",
			"Risk Metrics",
			"Top Drawdowns",
			"Monthly Returns",
			"Trade Summary",
		}

		for _, section := range expectedSections {
			Expect(output).To(ContainSubstring(section),
				"expected output to contain section heading %q", section)
		}
	})
})
