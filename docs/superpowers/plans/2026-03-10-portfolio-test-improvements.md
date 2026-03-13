# Portfolio Test Improvements Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace circular/tautological tests with correctness-validating tests and add edge case coverage for the most impactful gaps in the portfolio package.

**Architecture:** All tests use Ginkgo/Gomega. Test helpers `buildDF`, `buildMultiDF`, `daySeq`, `monthSeq`, `benchAcct`, and `cashAccount` are already defined in `testutil_test.go` and other test files. We add new `Describe`/`Context`/`It` blocks to existing test files -- no new files needed.

**Tech Stack:** Go, Ginkgo v2, Gomega, existing test helpers.

---

## Chunk 1: Replace Circular Tests in account_test.go

The Summary, RiskMetrics, and WithdrawalMetrics tests currently only compare batch method output to individual `PerformanceMetric()` calls. This is circular -- it validates that two code paths return the same value but not that either is correct. Replace each with a test that validates actual computed values against hand-calculated expectations.

### Task 1: Replace Summary circular test with correctness test

**Files:**
- Modify: `portfolio/account_test.go:444-485`

- [ ] **Step 1: Replace the Summary test**

Replace the existing `Describe("Summary")` block (lines 444-485) with a test that builds a known equity curve and checks that each Summary field has the expected hand-calculated value. Use the 6-point equity curve already used by risk_adjusted_metrics_test.go (SPY: [100,105,98,103,97,110], BIL: [100,100.01,100.02,100.03,100.04,100.05]) so we can cross-reference expected values.

```go
var _ = Describe("Summary", func() {
	It("returns correct computed values for a known equity curve", func() {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		spyPrices := []float64{100, 105, 98, 103, 97, 110}
		bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
		n := len(spyPrices)
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0]),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			df := buildDF(times[i],
				[]asset.Asset{spy, bil},
				[]float64{spyPrices[i], bilPrices[i]},
				[]float64{spyPrices[i], bilPrices[i]},
			)
			acct.UpdatePrices(df)
		}

		s := acct.Summary()

		// TWRR = (550/500) - 1 = 0.10
		Expect(s.TWRR).To(BeNumerically("~", 0.10, 1e-9))
		// MaxDrawdown = (485-525)/525 = -0.07619
		Expect(s.MaxDrawdown).To(BeNumerically("~", -0.07619, 1e-4))
		// StdDev ~ 1.3394 (annualized)
		Expect(s.StdDev).To(BeNumerically("~", 1.3394, 1e-3))
		// Sharpe ~ 4.1249
		Expect(s.Sharpe).To(BeNumerically("~", 4.1249, 1e-2))
		// Sortino ~ 58.496
		Expect(s.Sortino).To(BeNumerically("~", 58.496, 0.1))
		// Calmar ~ 1883.2
		Expect(s.Calmar).To(BeNumerically("~", 1883.2, 1.0))
		// MWRR should be positive
		Expect(s.MWRR).To(BeNumerically(">", 0.0))
	})
})
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./portfolio/ -v -run "Summary" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/account_test.go
git commit -m "test(portfolio): replace circular Summary test with correctness assertions"
```

### Task 2: Replace RiskMetrics circular test with correctness test

**Files:**
- Modify: `portfolio/account_test.go:487-534`

- [ ] **Step 1: Replace the RiskMetrics test**

Replace the existing `Describe("RiskMetrics")` block with a test that validates actual computed values. Use the same 6-point equity curve and cross-reference values from benchmark_metrics_test.go.

```go
var _ = Describe("RiskMetrics", func() {
	It("returns correct computed values for a known equity curve", func() {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		spyPrices := []float64{100, 105, 98, 103, 97, 110}
		bilPrices := []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}
		n := len(spyPrices)
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0]),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)
		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})
		for i := range n {
			df := buildDF(times[i],
				[]asset.Asset{spy, bil},
				[]float64{spyPrices[i], bilPrices[i]},
				[]float64{spyPrices[i], bilPrices[i]},
			)
			acct.UpdatePrices(df)
		}

		rm := acct.RiskMetrics()

		// Portfolio tracks SPY exactly (equity = 5*SPY), so portfolio
		// and benchmark have identical returns -> Beta=1, Alpha=0,
		// TrackingError=0, IR=0, RSquared=1.
		Expect(rm.Beta).To(BeNumerically("~", 1.0, 1e-10))
		Expect(rm.Alpha).To(BeNumerically("~", 0.0, 1e-10))
		Expect(rm.TrackingError).To(BeNumerically("~", 0.0, 1e-10))
		Expect(rm.InformationRatio).To(BeNumerically("~", 0.0, 1e-10))
		Expect(rm.RSquared).To(BeNumerically("~", 1.0, 1e-10))

		// Treynor = (portfolioReturn - rfReturn) / beta
		// = (0.10 - 0.0005) / 1.0 = 0.0995
		Expect(rm.Treynor).To(BeNumerically("~", 0.0995, 1e-4))

		// DownsideDeviation ~ 0.09445
		Expect(rm.DownsideDeviation).To(BeNumerically("~", 0.09445, 1e-3))

		// UlcerIndex, Skewness, ExcessKurtosis, ValueAtRisk should be finite
		Expect(math.IsNaN(rm.UlcerIndex)).To(BeFalse())
		Expect(math.IsNaN(rm.Skewness)).To(BeFalse())
		Expect(math.IsNaN(rm.ExcessKurtosis)).To(BeFalse())
		Expect(math.IsNaN(rm.ValueAtRisk)).To(BeFalse())
	})
})
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./portfolio/ -v -run "RiskMetrics" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/account_test.go
git commit -m "test(portfolio): replace circular RiskMetrics test with correctness assertions"
```

### Task 3: Replace WithdrawalMetrics circular test with correctness test

**Files:**
- Modify: `portfolio/account_test.go:536-568`

- [ ] **Step 1: Replace the WithdrawalMetrics test**

Replace with a test that validates actual values and the ordering invariant (PWR <= SWR <= DWR).

```go
var _ = Describe("WithdrawalMetrics", func() {
	It("returns correct withdrawal rates for a growing equity curve", func() {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		acct := portfolio.New(portfolio.WithCash(100_000))
		price := 100_000.0
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

		for i := range 300 {
			d := start.AddDate(0, 0, i)
			if i > 0 {
				growth := price * 0.0002
				acct.Record(portfolio.Transaction{
					Date:   d,
					Type:   portfolio.DividendTransaction,
					Amount: growth,
				})
				price += growth
			}
			df := buildDF(d, []asset.Asset{spy}, []float64{450 + float64(i)}, []float64{448 + float64(i)})
			acct.UpdatePrices(df)
		}

		wm := acct.WithdrawalMetrics()

		// Growing curve should produce non-zero rates
		Expect(wm.SafeWithdrawalRate).To(BeNumerically(">", 0.0))
		Expect(wm.DynamicWithdrawalRate).To(BeNumerically(">", 0.0))

		// Ordering invariant: PWR <= SWR <= DWR
		Expect(wm.PerpetualWithdrawalRate).To(BeNumerically("<=", wm.SafeWithdrawalRate))
		Expect(wm.SafeWithdrawalRate).To(BeNumerically("<=", wm.DynamicWithdrawalRate))

		// SWR should be approximately 0.063 for this curve (seed=42)
		Expect(wm.SafeWithdrawalRate).To(BeNumerically("~", 0.063, 0.001))
		// PWR should be approximately 0.049
		Expect(wm.PerpetualWithdrawalRate).To(BeNumerically("~", 0.049, 0.001))
	})
})
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./portfolio/ -v -run "WithdrawalMetrics" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/account_test.go
git commit -m "test(portfolio): replace circular WithdrawalMetrics test with correctness assertions"
```

---

## Chunk 2: Tax Metrics Edge Cases

### Task 4: Add tax boundary and edge case tests

**Files:**
- Modify: `portfolio/tax_metrics_test.go`

- [ ] **Step 1: Add edge case tests at end of file**

Add these tests before the final closing `})`:

```go
	Describe("edge cases", func() {
		It("classifies exactly 365 days as STCG (boundary)", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			// Buy on Jan 1, 2023
			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// Sell exactly 365 days later (Jan 1, 2024)
			// holdingDays = 365*24h / 24 = 365.0, code uses > 365, so this is STCG
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    100,
				Price:  120.0,
				Amount: 12_000.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(2_000.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})

		It("classifies 366 days as LTCG (just over boundary)", func() {
			a := portfolio.New(portfolio.WithCash(50_000))

			a.Record(portfolio.Transaction{
				Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			// 366 days later -> holdingDays > 365 -> LTCG
			a.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    100,
				Price:  120.0,
				Amount: 12_000.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.LTCG).To(Equal(2_000.0))
			Expect(tm.STCG).To(Equal(0.0))
		})

		It("buy and sell on same date is STCG", func() {
			a := portfolio.New(portfolio.WithCash(50_000))
			sameDay := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

			a.Record(portfolio.Transaction{
				Date:   sameDay,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    50,
				Price:  100.0,
				Amount: -5_000.0,
			})

			a.Record(portfolio.Transaction{
				Date:   sameDay,
				Asset:  spy,
				Type:   portfolio.SellTransaction,
				Qty:    50,
				Price:  105.0,
				Amount: 5_250.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(250.0))
			Expect(tm.LTCG).To(Equal(0.0))
		})

		It("returns zero TaxMetrics with no transactions", func() {
			a := portfolio.New(portfolio.WithCash(50_000))
			tm := a.TaxMetrics()

			Expect(tm.STCG).To(Equal(0.0))
			Expect(tm.LTCG).To(Equal(0.0))
			Expect(tm.UnrealizedSTCG).To(Equal(0.0))
			Expect(tm.UnrealizedLTCG).To(Equal(0.0))
			Expect(tm.QualifiedDividends).To(Equal(0.0))
			Expect(tm.TaxCostRatio).To(Equal(0.0))
		})

		It("handles multiple assets with interleaved buy/sell", func() {
			a := portfolio.New(portfolio.WithCash(100_000))
			aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

			// Buy SPY
			a.Record(portfolio.Transaction{
				Date:  time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset: spy, Type: portfolio.BuyTransaction,
				Qty: 50, Price: 100.0, Amount: -5_000.0,
			})
			// Buy AAPL
			a.Record(portfolio.Transaction{
				Date:  time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset: aapl, Type: portfolio.BuyTransaction,
				Qty: 30, Price: 150.0, Amount: -4_500.0,
			})
			// Sell SPY (STCG: 50 * (120-100) = 1000)
			a.Record(portfolio.Transaction{
				Date:  time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC),
				Asset: spy, Type: portfolio.SellTransaction,
				Qty: 50, Price: 120.0, Amount: 6_000.0,
			})
			// Buy more AAPL
			a.Record(portfolio.Transaction{
				Date:  time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset: aapl, Type: portfolio.BuyTransaction,
				Qty: 20, Price: 160.0, Amount: -3_200.0,
			})
			// Sell all AAPL (FIFO: 30@150 gain=30*(170-150)=600, 20@160 gain=20*(170-160)=200)
			a.Record(portfolio.Transaction{
				Date:  time.Date(2023, 8, 1, 0, 0, 0, 0, time.UTC),
				Asset: aapl, Type: portfolio.SellTransaction,
				Qty: 50, Price: 170.0, Amount: 8_500.0,
			})

			tm := a.TaxMetrics()
			Expect(tm.STCG).To(Equal(1_800.0)) // 1000 + 600 + 200
			Expect(tm.LTCG).To(Equal(0.0))
		})
	})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "TaxMetrics" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/tax_metrics_test.go
git commit -m "test(portfolio): add tax boundary and interleaved-asset edge cases"
```

---

## Chunk 3: Risk-Adjusted Metrics Edge Cases

### Task 5: Add Sharpe with negative mean return and Sortino with all negative returns

**Files:**
- Modify: `portfolio/risk_adjusted_metrics_test.go`

- [ ] **Step 1: Add edge case tests**

Add these inside the existing `Context("edge cases")` block, after the "all excess returns positive" context:

```go
		Context("negative mean excess return", func() {
			It("returns negative Sharpe", func() {
				// Monotonically declining equity with rising risk-free
				acct := buildAccount(
					[]float64{100, 95, 90, 85, 80},
					[]float64{100, 101, 102, 103, 104},
				)
				val := acct.PerformanceMetric(portfolio.Sharpe).Value()
				Expect(val).To(BeNumerically("<", 0.0))
			})
		})

		Context("all returns negative (declining equity)", func() {
			It("returns non-zero DownsideDeviation", func() {
				// All excess returns negative -> all counted as downside
				acct := buildAccount(
					[]float64{100, 95, 90, 85, 80},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				Expect(acct.PerformanceMetric(portfolio.DownsideDeviation).Value()).To(
					BeNumerically(">", 0.0))
			})

			It("returns non-zero Sortino when all excess returns are negative", func() {
				acct := buildAccount(
					[]float64{100, 95, 90, 85, 80},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				val := acct.PerformanceMetric(portfolio.Sortino).Value()
				// Sortino should be negative (negative mean / positive DD)
				Expect(val).To(BeNumerically("<", 0.0))
			})
		})

		Context("two data points (single return)", func() {
			It("returns StdDev = 0 (sample variance N-1=0)", func() {
				acct := buildAccount(
					[]float64{100, 110},
					[]float64{100, 100.01},
				)
				Expect(acct.PerformanceMetric(portfolio.StdDev).Value()).To(Equal(0.0))
			})

			It("returns Sharpe = 0 (stddev=0 guard)", func() {
				acct := buildAccount(
					[]float64{100, 110},
					[]float64{100, 100.01},
				)
				Expect(acct.PerformanceMetric(portfolio.Sharpe).Value()).To(Equal(0.0))
			})
		})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "Risk-Adjusted" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/risk_adjusted_metrics_test.go
git commit -m "test(portfolio): add negative-return, all-negative, and two-point edge cases"
```

---

## Chunk 4: Trade Metrics Edge Cases

### Task 6: Add trade metrics edge cases

**Files:**
- Modify: `portfolio/trade_metrics_test.go`

- [ ] **Step 1: Add edge case tests before final closing `})`**

```go
	Describe("single winning trade", func() {
		It("returns WinRate=1.0, AverageWin>0, AverageLoss=0", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset: acme, Type: portfolio.BuyTransaction,
				Qty: 10, Price: 100.0, Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:  time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset: acme, Type: portfolio.SellTransaction,
				Qty: 10, Price: 120.0, Amount: 1_200.0,
			})

			tm := a.TradeMetrics()
			Expect(tm.WinRate).To(Equal(1.0))
			Expect(tm.AverageWin).To(Equal(200.0))
			Expect(tm.AverageLoss).To(Equal(0.0))
			Expect(tm.ProfitFactor).To(Equal(0.0))  // no losses -> 0
			Expect(tm.GainLossRatio).To(Equal(0.0)) // no losses -> 0
		})
	})

	Describe("single losing trade", func() {
		It("returns WinRate=0, AverageWin=0, non-zero AverageLoss", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			a.Record(portfolio.Transaction{
				Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset: acme, Type: portfolio.BuyTransaction,
				Qty: 10, Price: 100.0, Amount: -1_000.0,
			})
			a.Record(portfolio.Transaction{
				Date:  time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset: acme, Type: portfolio.SellTransaction,
				Qty: 10, Price: 80.0, Amount: 800.0,
			})

			tm := a.TradeMetrics()
			Expect(tm.WinRate).To(Equal(0.0))
			Expect(tm.AverageWin).To(Equal(0.0))
			Expect(tm.AverageLoss).To(Equal(-200.0))
			Expect(tm.ProfitFactor).To(Equal(0.0))
			Expect(tm.GainLossRatio).To(Equal(0.0))
		})
	})

	Describe("NPositivePeriods with flat equity curve", func() {
		It("returns 0.0 when all returns are zero", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			buildDF := func(t time.Time, price float64) *data.DataFrame {
				df, err := data.NewDataFrame(
					[]time.Time{t},
					[]asset.Asset{acme},
					[]data.Metric{data.MetricClose, data.AdjClose},
					[]float64{price, price},
				)
				Expect(err).NotTo(HaveOccurred())
				return df
			}

			// Flat prices -> zero returns -> no positive periods
			for i := range 5 {
				d := time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC)
				a.UpdatePrices(buildDF(d, 100.0))
			}

			tm := a.TradeMetrics()
			Expect(tm.NPositivePeriods).To(Equal(0.0))
		})
	})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "TradeMetrics" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/trade_metrics_test.go
git commit -m "test(portfolio): add single-trade and flat-curve trade metric edge cases"
```

---

## Chunk 5: Capture and Drawdown Edge Cases

### Task 7: Add capture ratio edge cases

**Files:**
- Modify: `portfolio/capture_drawdown_metrics_test.go`

- [ ] **Step 1: Add edge case tests before final closing `})`**

```go
	Describe("Capture ratio edge cases", func() {
		It("handles portfolio and benchmark moving in opposite directions", func() {
			// Portfolio goes up when benchmark goes down, and vice versa
			// Equity:    [10000, 9000,  11000, 10000]
			// Benchmark: [100,   110,   95,    105  ]
			//
			// Portfolio returns:  [-0.1,   0.2222, -0.0909]
			// Benchmark returns:  [ 0.1,  -0.1364,  0.1053]
			//
			// Up benchmark periods: i=0 (pRet=-0.1, bRet=0.1), i=2 (pRet=-0.0909, bRet=0.1053)
			// Portfolio is negative in up-benchmark periods -> negative upside capture
			a := cashAccount(
				[]float64{10000, 9000, 11000, 10000},
				[]float64{100, 110, 95, 105},
			)

			v := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			// Upside capture should be negative (portfolio declines when benchmark rises)
			Expect(v).To(BeNumerically("<", 0.0))
		})

		It("returns 0 for two data points with no up benchmark periods", func() {
			// One return, benchmark falls
			a := cashAccount(
				[]float64{10000, 10500},
				[]float64{100, 95},
			)
			v := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(v).To(Equal(0.0))
		})
	})

	Describe("AvgDrawdown edge cases", func() {
		It("handles flat equity curve", func() {
			a := cashAccount(
				[]float64{10000, 10000, 10000, 10000},
				[]float64{100, 105, 110, 115},
			)
			v := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(v).To(Equal(0.0))
		})

		It("handles continuous drawdown without recovery", func() {
			// Equity: [10000, 9000, 8000, 7000]
			// One episode: peak=10000, trough at i=3: (7000-10000)/10000 = -0.3
			a := cashAccount(
				[]float64{10000, 9000, 8000, 7000},
				[]float64{100, 95, 90, 85},
			)
			v := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(v).To(BeNumerically("~", -0.3, 1e-9))
		})
	})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "Capture and Drawdown" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/capture_drawdown_metrics_test.go
git commit -m "test(portfolio): add capture ratio and drawdown edge cases"
```

---

## Chunk 6: Specialized Metrics Edge Cases

### Task 8: Add KRatio negative slope and KellerRatio boundary tests

**Files:**
- Modify: `portfolio/specialized_metrics_test.go`

- [ ] **Step 1: Add edge case tests before final closing `})`**

```go
	Describe("KRatio edge cases", func() {
		It("returns negative KRatio for a declining curve", func() {
			// Equity: [100, 95, 90, 85, 80] -- monotonically declining
			// logVAMI has negative slope -> KRatio should be negative
			a := buildAccountFromEquity([]float64{100, 95, 90, 85, 80})
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically("<", 0.0))
		})

		It("returns positive KRatio for a rising curve", func() {
			a := buildAccountFromEquity([]float64{100, 105, 110, 115, 120})
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically(">", 0.0))
		})
	})

	Describe("KellerRatio edge cases", func() {
		It("returns zero when max drawdown is exactly 50%", func() {
			// equity: [100, 200, 100] -> maxDD = (100-200)/200 = -0.5 = 50%
			// Code uses maxDD >= 0.5 guard -> returns 0
			a := buildAccountFromEquity([]float64{100, 200, 100})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns non-zero when max drawdown is just under 50%", func() {
			// equity: [100, 200, 101] -> maxDD = (101-200)/200 = -0.495 < 50%
			// totalReturn = 0.01 > 0
			a := buildAccountFromEquity([]float64{100, 200, 101})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically(">", 0))
		})
	})

	Describe("UlcerIndex edge cases", func() {
		It("returns correct value for flat-then-drop curve", func() {
			// [100, 100, 100, 90] -> dd = [0, 0, 0, -0.1]
			// sumSq = 0.01, mean = 0.01/4 = 0.0025
			// UI = sqrt(0.0025) = 0.05
			a := buildAccountFromEquity([]float64{100, 100, 100, 90})
			result := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(result).To(BeNumerically("~", 0.05, 1e-9))
		})
	})

	Describe("ValueAtRisk edge cases", func() {
		It("returns the only return for a 2-point equity curve", func() {
			// 1 return -> idx = floor(0.05 * 1) = 0 -> sorted[0]
			a := buildAccountFromEquity([]float64{100, 90})
			result := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(result).To(BeNumerically("~", -0.1, 1e-9))
		})
	})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "Specialized" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/specialized_metrics_test.go
git commit -m "test(portfolio): add KRatio, KellerRatio, UlcerIndex, VaR edge cases"
```

---

## Chunk 7: Selector and Weighting Edge Cases

### Task 9: Add selector Inf value handling test

**Files:**
- Modify: `portfolio/selector_test.go`

- [ ] **Step 1: Add Inf test before final closing `})`**

```go
	It("treats +Inf as above zero and selects it", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				math.Inf(1), // SPY = +Inf
				5,           // AAPL = 5
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		// +Inf > 5, so SPY should be selected
		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("SPY001"))
	})

	It("treats -Inf as not above zero and falls back", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				math.Inf(-1), // SPY = -Inf
				math.Inf(-1), // AAPL = -Inf
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero([]asset.Asset{bil})
		result := sel.Select(df)

		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0].CompositeFigi).To(Equal("BIL001"))
	})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "MaxAboveZero" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/selector_test.go
git commit -m "test(portfolio): add Inf/-Inf signal value edge cases for MaxAboveZero"
```

### Task 10: Add WeightedBySignal mixed-value edge case

**Files:**
- Modify: `portfolio/weighting_test.go`

- [ ] **Step 1: Add mixed positive/negative test before final closing `})`**

```go
	It("ignores negative values and weights positive values proportionally", func() {
		bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MarketCap},
			[]float64{300, -50, 100},
		)
		Expect(err).ToNot(HaveOccurred())

		plan := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(plan).To(HaveLen(1))

		// Only positive values count: SPY=300, BIL=100, total=400
		// AAPL has negative signal -> weight 0
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[bil]).To(Equal(0.25))
		Expect(plan[0].Members[aapl]).To(Equal(0.0))
	})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "WeightedBySignal" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/weighting_test.go
git commit -m "test(portfolio): add mixed positive/negative signal weighting edge case"
```

---

## Chunk 8: Withdrawal Metrics Edge Cases

### Task 11: Add withdrawal metrics edge cases for flat and declining curves

**Files:**
- Modify: `portfolio/withdrawal_metrics_test.go`

- [ ] **Step 1: Add edge case tests**

Add a new `Describe` for declining equity before the "ordering invariant" block:

```go
	Describe("declining equity curve", func() {
		buildDecliningAccount := func() *portfolio.Account {
			a := portfolio.New(portfolio.WithCash(100_000))
			start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
			price := 100_000.0

			for i := range 300 {
				d := start.AddDate(0, 0, i)
				if i > 0 {
					loss := price * 0.0001
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.WithdrawalTransaction,
						Amount: -loss,
					})
					price -= loss
				}
				df := buildDF(d, []asset.Asset{spy}, []float64{450 - float64(i)*0.1}, []float64{448 - float64(i)*0.1})
				a.UpdatePrices(df)
			}

			return a
		}

		It("SafeWithdrawalRate is lower than for a growing curve", func() {
			declining := buildDecliningAccount()
			growing := buildModerateAccount()

			declSWR := portfolio.SafeWithdrawalRate.Compute(declining, nil)
			growSWR := portfolio.SafeWithdrawalRate.Compute(growing, nil)

			Expect(declSWR).To(BeNumerically("<", growSWR))
		})

		It("PerpetualWithdrawalRate is 0 for declining curve", func() {
			a := buildDecliningAccount()
			Expect(portfolio.PerpetualWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
		})
	})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./portfolio/ -v -run "Withdrawal" -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/withdrawal_metrics_test.go
git commit -m "test(portfolio): add declining equity curve withdrawal metric edge cases"
```

---

## Chunk 9: Final verification

### Task 12: Run full test suite

- [ ] **Step 1: Run all portfolio tests**

Run: `go test ./portfolio/ -v -count=1`
Expected: All tests PASS (original 213 + new tests)

- [ ] **Step 2: Verify no regressions across entire project**

Run: `go test ./... -count=1`
Expected: All tests PASS
