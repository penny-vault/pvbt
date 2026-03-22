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

package montecarlo_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/montecarlo"
)

// buildHistoricalDF builds a small historical DataFrame with a single asset
// and 25 daily trading days starting from 2024-01-02.
func buildHistoricalDF() *data.DataFrame {
	spy := asset.Asset{CompositeFigi: "BBG000BDTBL9", Ticker: "SPY"}

	assets := []asset.Asset{spy}
	metrics := []data.Metric{data.MetricClose}

	// Generate 25 trading days (skip weekends) starting 2024-01-02.
	loc := time.UTC
	start := time.Date(2024, time.January, 2, 16, 0, 0, 0, loc)

	times := make([]time.Time, 0, 25)
	for date := start; len(times) < 25; date = date.AddDate(0, 0, 1) {
		wd := date.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			continue
		}

		times = append(times, date)
	}

	numTimes := len(times)
	closePrices := make([]float64, numTimes)

	for idx := range numTimes {
		closePrices[idx] = 450.0 + float64(idx)*0.5
	}

	cols := [][]float64{closePrices}

	df, err := data.NewDataFrame(times, assets, metrics, data.Daily, cols)
	Expect(err).NotTo(HaveOccurred())

	return df
}

var _ = Describe("MonteCarloStudy", func() {
	var (
		historicalDF *data.DataFrame
		metrics      []data.Metric
		mcs          *montecarlo.MonteCarloStudy
	)

	BeforeEach(func() {
		metrics = []data.Metric{data.MetricClose}
		historicalDF = buildHistoricalDF()
		mcs = montecarlo.New(historicalDF, metrics)
		mcs.StartDate = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		mcs.EndDate = time.Date(2024, 6, 28, 0, 0, 0, 0, time.UTC)
		mcs.InitialDeposit = 10_000
	})

	Describe("Study interface", func() {
		It("satisfies the study.Study interface", func() {
			var _ study.Study = mcs
		})

		It("returns the correct name", func() {
			Expect(mcs.Name()).To(Equal("Monte Carlo Simulation"))
		})

		It("returns a non-empty description", func() {
			Expect(mcs.Description()).NotTo(BeEmpty())
		})
	})

	Describe("EngineCustomizer interface", func() {
		It("satisfies the study.EngineCustomizer interface", func() {
			var _ study.EngineCustomizer = mcs
		})
	})

	Describe("Configurations", func() {
		It("returns the correct number of configs", func() {
			mcs.Simulations = 50

			configs, err := mcs.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(50))
		})

		It("uses the default of 1000 simulations", func() {
			configs, err := mcs.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(1000))
		})

		It("names each config sequentially", func() {
			mcs.Simulations = 3

			configs, err := mcs.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs[0].Name).To(Equal("Path 1"))
			Expect(configs[1].Name).To(Equal("Path 2"))
			Expect(configs[2].Name).To(Equal("Path 3"))
		})

		It("assigns a unique seed to each config", func() {
			mcs.Simulations = 10
			mcs.Seed = 100

			configs, err := mcs.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())

			seeds := make(map[string]bool, len(configs))

			for _, cfg := range configs {
				seedStr, hasSeed := cfg.Metadata["simulation_seed"]
				Expect(hasSeed).To(BeTrue(), "config %q is missing simulation_seed metadata", cfg.Name)

				Expect(seeds[seedStr]).To(BeFalse(), "duplicate seed %q found", seedStr)

				seeds[seedStr] = true
			}
		})

		It("embeds the study name in each config's metadata", func() {
			mcs.Simulations = 3

			configs, err := mcs.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())

			for _, cfg := range configs {
				Expect(cfg.Metadata["study"]).To(Equal("monte-carlo"))
			}
		})

		It("seeds are offset from the base seed by path index", func() {
			mcs.Simulations = 5
			mcs.Seed = 42

			configs, err := mcs.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())

			for pathIdx, cfg := range configs {
				expectedSeed := fmt.Sprintf("%d", 42+uint64(pathIdx))
				Expect(cfg.Metadata["simulation_seed"]).To(Equal(expectedSeed))
			}
		})

		It("sets start, end, and deposit from the study fields", func() {
			configs, err := mcs.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())

			first := configs[0]
			Expect(first.Start).To(Equal(mcs.StartDate))
			Expect(first.End).To(Equal(mcs.EndDate))
			Expect(first.Deposit).To(Equal(mcs.InitialDeposit))
		})
	})

	Describe("EngineOptions", func() {
		It("returns a non-nil slice for a config with a valid simulation_seed", func() {
			cfg := study.RunConfig{
				Name: "Path 1",
				Metadata: map[string]string{
					"simulation_seed": "42",
				},
			}

			opts := mcs.EngineOptions(cfg)
			Expect(opts).NotTo(BeNil())
			Expect(opts).To(HaveLen(1))
		})

		It("returns nil when simulation_seed metadata is absent", func() {
			cfg := study.RunConfig{
				Name:     "No Seed",
				Metadata: map[string]string{},
			}

			opts := mcs.EngineOptions(cfg)
			Expect(opts).To(BeNil())
		})

		It("returns nil when Metadata is nil", func() {
			cfg := study.RunConfig{
				Name: "Nil Metadata",
			}

			opts := mcs.EngineOptions(cfg)
			Expect(opts).To(BeNil())
		})

		It("returns nil when simulation_seed is not a valid uint64", func() {
			cfg := study.RunConfig{
				Name: "Bad Seed",
				Metadata: map[string]string{
					"simulation_seed": "not-a-number",
				},
			}

			opts := mcs.EngineOptions(cfg)
			Expect(opts).To(BeNil())
		})

		It("produces distinct options for configs with different seeds", func() {
			cfg1 := study.RunConfig{
				Name:     "Path 1",
				Metadata: map[string]string{"simulation_seed": "42"},
			}
			cfg2 := study.RunConfig{
				Name:     "Path 2",
				Metadata: map[string]string{"simulation_seed": "43"},
			}

			opts1 := mcs.EngineOptions(cfg1)
			opts2 := mcs.EngineOptions(cfg2)

			Expect(opts1).NotTo(BeNil())
			Expect(opts2).NotTo(BeNil())
			// Each call returns a fresh slice -- they must be independent.
			Expect(&opts1[0]).NotTo(BeIdenticalTo(&opts2[0]))
		})
	})

	Describe("Analyze", func() {
		It("returns an empty report when no results are provided", func() {
			rpt, err := mcs.Analyze([]study.RunResult{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rpt.Title).To(Equal("Monte Carlo Simulation"))
		})
	})
})
