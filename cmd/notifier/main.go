package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"main/data"
	"main/database"
	"main/portfolio"
	"main/strategies"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	log "github.com/sirupsen/logrus"
)

const (
	daily    = 0x00000010
	weekly   = 0x00000100
	monthly  = 0x00001000
	annually = 0x00010000
)

var disableSend bool = false

func getSavedPortfolios(startDate time.Time) []*portfolio.Portfolio {
	ret := []*portfolio.Portfolio{}
	trx, err := database.TrxForUser("pvapi")
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Fatal("failed to create database transaction for pvapi")
	}
	// get a list of users
	userSQL := `SELECT b.rolname as user FROM (pg_catalog.pg_roles r LEFT JOIN pg_catalog.pg_auth_members m ON r.oid = m.member) JOIN pg_catalog.pg_roles b ON (m.roleid = b.oid) where r.rolname=$1;`
	rows, err := trx.Query(context.Background(), userSQL, "pvapi")
	if err != nil {
		log.WithFields(log.Fields{
			"Query": userSQL,
			"Error": err,
		}).Fatal("failed to execute query to get user accounts with portfolios")
	}

	users := []string{}
	for rows.Next() {
		var userID string
		rows.Scan(&userID)
		users = append(users, userID)
	}
	trx.Commit(context.Background())

	for _, userID := range users {
		trx, err := database.TrxForUser(userID)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":  err,
				"UserID": userID,
			}).Error("failed to create database transaction for user")
			continue
		}
		portfolioSQL := `SELECT id FROM portfolio_v1 WHERE start_date <= $1`
		rows, err := trx.Query(context.Background(), portfolioSQL, startDate)
		if err != nil {
			log.WithFields(log.Fields{
				"UserID": userID,
				"Query":  portfolioSQL,
				"Error":  err,
			}).Error("Could not fetch portfolios for user")
			trx.Rollback(context.Background())
			continue
		}

		for rows.Next() {
			id := uuid.UUID{}
			err := rows.Scan(&id)
			if err != nil {
				log.WithFields(log.Fields{
					"Error": err,
				}).Fatal("database query selecting user portfolio id's failed")
			}
			if p, err := portfolio.LoadFromDB(id, userID); err != nil {
				log.WithFields(log.Fields{
					"Error":       err,
					"PortfolioID": id,
					"UserID":      userID,
				}).Fatal("could not load portfolio from database")
			} else {
				ret = append(ret, p)
			}
		}
		trx.Commit(context.Background())
	}

	return ret
}

func updateSavedPortfolioPerformanceMetrics(p *portfolio.Portfolio, perf *portfolio.Performance) {
	trx, err := database.TrxForUser(p.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"UserID": p.UserID,
			"Error":  err,
		}).Error("Failed to get transaction for user")
	}
	updateSQL := `UPDATE portfolio_v1 SET ytd_return=$1, cagr_3yr=$2, cagr_5yr=$3, cagr_10yr=$4, cagr_since_inception=$5, std_dev=$6, downside_deviation=$7, max_draw_down=$8, avg_draw_down=$9, sharpe_ratio=$10, sortino_ratio=$11, ulcer_index=$12, performance_json=$13 WHERE id=$14`

	// create JSON without transaction and measurement arrays
	trxArray := perf.Transactions
	measArray := perf.Measurements

	perf.Transactions = []portfolio.Transaction{}
	perf.Measurements = []portfolio.PerformanceMeasurement{}

	perfJSON, err := json.Marshal(perf)

	perf.Transactions = trxArray
	perf.Measurements = measArray

	if err != nil {
		log.WithFields(log.Fields{
			"PortfolioID": p.ID,
			"Error":       err,
		}).Error("could not encode performance struct as JSON")
	}
	_, err = trx.Exec(context.Background(),
		updateSQL,
		perf.YTDReturn,
		perf.MetricsBundle.CAGRS.ThreeYear,
		perf.MetricsBundle.CAGRS.FiveYear,
		perf.MetricsBundle.CAGRS.TenYear,
		perf.CagrSinceInception,
		perf.MetricsBundle.StdDev,
		perf.MetricsBundle.DownsideDeviation,
		perf.MetricsBundle.MaxDrawDown.LossPercent,
		perf.MetricsBundle.AvgDrawDown,
		perf.MetricsBundle.SharpeRatio,
		perf.MetricsBundle.SortinoRatio,
		perf.MetricsBundle.UlcerIndexAvg,
		perfJSON,
		p.ID,
	)
	if err != nil {
		log.WithFields(log.Fields{
			"PortfolioID":          p.ID,
			"YTDReturn":            perf.YTDReturn,
			"CagrSinceInception":   perf.CagrSinceInception,
			"PerformanceStartDate": time.Unix(perf.PeriodStart, 0),
			"PerformanceEndDate":   time.Unix(perf.PeriodEnd, 0),
			"Error":                err,
		}).Error("Could not update portfolio performance metrics")
		trx.Rollback(context.Background())
		return
	}

	log.WithFields(log.Fields{
		"PortfolioID":          p.ID,
		"YTDReturn":            perf.YTDReturn,
		"CagrSinceInception":   perf.CagrSinceInception,
		"PerformanceStartDate": time.Unix(perf.PeriodStart, 0),
		"PerformanceEndDate":   time.Unix(perf.PeriodEnd, 0),
	}).Info("Calculated portfolio performance")
	trx.Commit(context.Background())
}

func saveTransactions(p *portfolio.Portfolio, perf *portfolio.Performance) error {
	startTime := time.Now()
	log.WithFields(log.Fields{
		"NumTransactions": len(perf.Transactions),
		"StartTime":       startTime,
	}).Info("Saving transactions to the database")

	trx, err := database.TrxForUser(p.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"UserID": p.UserID,
			"Error":  err,
		}).Error("Failed to get transaction for user")
	}

	sql := `INSERT INTO portfolio_transaction_v1
			(
				"composite_figi",
				"event_date",
				"justification",
				"num_shares",
				"portfolio_id",
				"price_per_share",
				"sequence_num",
			 	"source",
				"source_id",
				"ticker",
				"total_value",
				"transaction_type"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT ON CONSTRAINT portfolio_transaction_v1_portfolio_id_source_id_key DO NOTHING`

	now := time.Now()
	for idx, transaction := range perf.Transactions {
		// don't save future transactions -- note, transactions must be date ordered so it's
		// safe to bail after we see one
		if transaction.Date.After(now) {
			break
		}

		_, err = trx.Exec(context.Background(), sql,
			transaction.CompositeFIGI,
			transaction.Date,
			transaction.Justification,
			transaction.Shares,
			p.ID,
			transaction.PricePerShare,
			idx,
			transaction.Source,
			transaction.SourceID,
			transaction.Ticker,
			transaction.TotalValue,
			transaction.Kind,
		)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID": p.ID,
				"Source":      transaction.Source,
				"SourceID":    transaction.SourceID,
				"Error":       err,
			}).Warn("could not save transaction to database")
			trx.Rollback(context.Background())
			return err
		}
	}
	trx.Commit(context.Background())

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	log.WithFields(log.Fields{
		"NumTransactions": len(perf.Transactions),
		"EndTime":         endTime,
		"Duration":        duration,
	}).Info("saved transactions to the database")

	return nil
}

func saveMeasurements(p *portfolio.Portfolio, perf *portfolio.Performance) error {
	startTime := time.Now()
	log.WithFields(log.Fields{
		"NumMeasurements": len(perf.Measurements),
		"StartTime":       startTime,
	}).Info("saving measurements to the database")

	trx, err := database.TrxForUser(p.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"UserID": p.UserID,
			"Error":  err,
		}).Error("failed to get transaction for user")
	}

	sql := `INSERT INTO portfolio_measurement_v1
			(
				"event_date",
				"holdings",
				"justification",
				"percent_return",
				"portfolio_id",
				"risk_free_value",
				"total_deposited_to_date",
				"total_withdrawn_to_date",
				"value"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT ON CONSTRAINT portfolio_measurement_v1_pkey
			DO UPDATE SET
				holdings=$2,
				justification=$3,
				percent_return=$4,
				risk_free_value=$6,
				total_deposited_to_date=$7,
				total_withdrawn_to_date=$8,
				value=$9;`

	now := time.Now()
	for _, measurement := range perf.Measurements {
		date := time.Unix(measurement.Time, 0)
		// don't save future performance measurements
		if date.After(now) {
			break
		}

		_, err = trx.Exec(context.Background(), sql,
			time.Unix(measurement.Time, 0),
			measurement.Holdings,
			measurement.Justification,
			measurement.PercentReturn,
			p.ID,
			measurement.RiskFreeValue,
			measurement.TotalDeposited,
			measurement.TotalWithdrawn,
			measurement.Value,
		)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID": p.ID,
				"Error":       err,
			}).Warn("could not save measurement to database")
			trx.Rollback(context.Background())
			return err
		}
	}
	trx.Commit(context.Background())

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	log.WithFields(log.Fields{
		"NumMeasurements": len(perf.Measurements),
		"EndTime":         endTime,
		"Duration":        duration,
	}).Info("saved measurements to the database")

	return nil
}

func updateTransactions(p *portfolio.Portfolio, through time.Time) error {
	log.WithFields(log.Fields{
		"Portfolio": p.ID,
	}).Info("Computing portfolio performance")

	var manager data.Manager
	if p.UserID == "pvuser" {
		// System user is not in auth0 - expect an environment variable with the tiingo token to use
		manager = data.NewManager(map[string]string{
			"tiingo": os.Getenv("SYSTEM_TIINGO_TOKEN"),
		})
	} else {
		u, err := getUser(p.UserID)
		if err != nil {
			return err
		}
		manager = data.NewManager(map[string]string{
			"tiingo": u.TiingoToken,
		})
	}

	manager.Begin = p.StartDate
	manager.End = through
	manager.Frequency = data.FrequencyMonthly

	return p.UpdateTransactions(&manager, through)
}

func datesEqual(d1 time.Time, d2 time.Time) bool {
	year, month, day := d1.Date()
	d1 = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	year, month, day = d2.Date()
	d2 = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return d1.Equal(d2)
}

func lastTradingDayOfWeek(today time.Time, manager *data.Manager) bool {
	lastDay, err := manager.LastTradingDayOfWeek(today)
	if err != nil {
		return false
	}
	return datesEqual(today, lastDay)
}

func lastTradingDayOfMonth(today time.Time, manager *data.Manager) bool {
	lastDay, err := manager.LastTradingDayOfMonth(today)
	if err != nil {
		return false
	}
	return datesEqual(today, lastDay)
}

func lastTradingDayOfYear(today time.Time, manager *data.Manager) bool {
	lastDay, err := manager.LastTradingDayOfYear(today)
	if err != nil {
		return false
	}
	return datesEqual(today, lastDay)
}

func processNotifications(forDate time.Time, p *portfolio.Portfolio, perf *portfolio.Performance) {
	if p.UserID == "pvuser" {
		return
	}

	u, err := getUser(p.UserID)
	if err != nil {
		return
	}

	toSend := []string{}

	manager := data.NewManager(map[string]string{
		"tiingo": u.TiingoToken,
	})
	manager.Begin = p.StartDate

	if (p.Notifications & daily) == daily {
		toSend = append(toSend, "Daily")
	}
	if (p.Notifications & weekly) == weekly {
		// only send on Friday
		if lastTradingDayOfWeek(forDate, &manager) {
			toSend = append(toSend, "Weekly")
		}
	}
	if (p.Notifications & monthly) == monthly {
		if lastTradingDayOfMonth(forDate, &manager) {
			toSend = append(toSend, "Monthly")
		}
	}
	if (p.Notifications & annually) == annually {
		if lastTradingDayOfYear(forDate, &manager) {
			toSend = append(toSend, "Annually")
		}
	}

	for _, freq := range toSend {
		log.Infof("Send %s notification for portfolio %s", freq, p.ID)
		message, err := buildEmail(forDate, freq, p, perf, u)
		if err != nil {
			continue
		}

		statusCode, messageIDs, err := sendEmail(message)
		if err != nil {
			continue
		}

		log.WithFields(log.Fields{
			"Function":   "cmd/notifier/main.go:processNotifications",
			"StatusCode": statusCode,
			"MessageID":  messageIDs,
			"Portfolio":  p.ID,
			"UserId":     u.ID,
			"UserEmail":  u.Email,
		}).Infof("Sent %s email to %s", freq, u.Email)
	}
}

func periodReturn(forDate time.Time, frequency string, p *portfolio.Portfolio,
	perf *portfolio.Performance) string {
	var ret float64
	switch frequency {
	case "Daily":
		ret = perf.OneDayReturn(forDate, p)
	case "Weekly":
		ret = perf.OneWeekReturn(forDate, p)
	case "Monthly":
		ret = perf.OneMonthReturn(forDate)
	case "Annually":
		ret = perf.YTDReturn
	}
	return formatReturn(ret)
}

func formatDate(forDate time.Time) string {
	dateStr := forDate.Format("2 Jan 2006")
	dateStr = strings.ToUpper(dateStr)
	if forDate.Day() < 10 {
		dateStr = fmt.Sprintf("0%s", dateStr)
	}
	return dateStr
}

func formatReturn(ret float64) string {
	sign := "+"
	if ret < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%.2f%%", sign, ret*100)
}

// Email utilizing dynamic transactional templates
// Note: you must customize subject line of the dynamic template itself
// Note: you may not use substitutions with dynamic templates
func buildEmail(forDate time.Time, frequency string, p *portfolio.Portfolio,
	perf *portfolio.Performance, to *User) ([]byte, error) {
	if !to.Verified {
		log.WithFields(log.Fields{
			"Function": "cmd/notifier/main.go:sendEmail",
			"UserId":   to.ID,
		}).Warn("Refusing to send email to unverified email address")
		return nil, errors.New("refusing to send email to unverified email address")
	}

	from := User{
		Name:  "Penny Vault",
		Email: "notify@pennyvault.com",
	}

	m := mail.NewV3Mail()

	e := mail.NewEmail(from.Name, from.Email)
	m.SetFrom(e)

	// TODO - figure out best place to encode this -- hardcoded value here is probably not the best
	m.SetTemplateID("d-69e0989795c24f348959cf399024bd54")

	person := mail.NewPersonalization()
	tos := []*mail.Email{
		mail.NewEmail(to.Name, to.Email),
	}
	person.AddTos(tos...)

	person.SetDynamicTemplateData("portfolioName", p.Name)
	if strat, ok := strategies.StrategyMap[p.StrategyShortcode]; ok {
		person.SetDynamicTemplateData("strategy", strat.Name)
	}

	person.SetDynamicTemplateData("frequency", frequency)
	person.SetDynamicTemplateData("forDate", formatDate(forDate))
	person.SetDynamicTemplateData("currentAsset", perf.CurrentAsset)

	person.SetDynamicTemplateData("periodReturn", periodReturn(forDate, frequency, p, perf))
	person.SetDynamicTemplateData("ytdReturn", formatReturn(perf.YTDReturn))

	m.AddPersonalizations(person)
	return mail.GetRequestBody(m), nil
}

func sendEmail(message []byte) (statusCode int, messageID []string, err error) {
	// if we are testing then disableSend is set
	if disableSend {
		log.WithFields(log.Fields{
			"Message": string(message),
		}).Warn("Skipping email send")
		return 304, []string{}, nil
	}

	request := sendgrid.GetRequest(os.Getenv("SENDGRID_API_KEY"), "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = message

	response, err := sendgrid.API(request)
	if err != nil {
		log.Error(err)
		return -1, nil, err
	}

	return response.StatusCode, response.Headers["X-Message-Id"], nil
}

func validRunDay(today time.Time) bool {
	isWeekday := !(today.Weekday() == time.Saturday || today.Weekday() == time.Sunday)
	isHoliday := false
	// Christmas:
	// (today.Day() == 25 && today.Month() == time.December)
	return isWeekday && !isHoliday
}

// ------------------
// main method

func main() {
	testFlag := flag.Bool("test", false, "test the notifier and don't send notifications")
	limitFlag := flag.Int("limit", 0, "limit the number of portfolios to process")
	dateFlag := flag.String("date", "-1", "date to run notifier for")
	portfolioFlag := flag.String("portfolio", "all", "run portfolio specified by id, or 'all' to run all portfolios")
	debugFlag := flag.Bool("debug", false, "turn on debug logging")
	flag.Parse()

	// configure logging
	if *debugFlag {
		log.SetReportCaller(true)
	}

	var forDate time.Time
	if *dateFlag == "-1" {
		tz, _ := time.LoadLocation("America/New_York")
		forDate = time.Now().In(tz).AddDate(0, 0, -1)
	} else {
		var err error
		forDate, err = time.Parse("2006-01-02", *dateFlag)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Infof("Running for date %s", forDate.String())

	// Check if it's a valid run day
	if !validRunDay(forDate) {
		log.Fatal("Exiting because it is a holiday, or not a weekday")
	}

	disableSend = *testFlag

	// setup database
	err := database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	data.InitializeDataManager()
	log.Info("Initialized data framework")

	strategies.InitializeStrategyMap()
	log.Info("Initialized strategy map")

	var portfolioID uuid.UUID
	if *portfolioFlag != "all" {
		portfolioID, err = uuid.Parse(*portfolioFlag)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID": *portfolioFlag,
				"Error":       err,
			}).Fatal("Cannot parse portfolio ID")
		}
	}

	// get a list of all portfolios
	savedPortfolios := getSavedPortfolios(forDate)
	log.WithFields(log.Fields{
		"NumPortfolios": len(savedPortfolios),
	}).Info("Got saved portfolios")
	for ii, p := range savedPortfolios {
		if *portfolioFlag != "all" && portfolioID == p.ID {
			err = updateTransactions(p, forDate)
			if err != nil {
				log.WithFields(log.Fields{
					"PortfolioID": p.ID,
					"Error":       err,
				}).Error("Failed to update portfolio transactions")
				continue
			}
			perf, err := p.CalculatePerformance(forDate)
			if err != nil {
				log.WithFields(log.Fields{
					"Error":       err,
					"PortfolioID": p.ID,
					"Function":    "portfolio.CalculatePerformance",
					"ForDate":     forDate.String(),
				}).Error("Failed to calculate portfolio performance")
			}
			perf.BuildMetricsBundle()
			updateSavedPortfolioPerformanceMetrics(p, perf)
			saveTransactions(p, perf)
			saveMeasurements(p, perf)
			processNotifications(forDate, p, perf)
			if *limitFlag != 0 && *limitFlag >= ii {
				break
			}
		}
	}
}
