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

type savedStrategy struct {
	ID            uuid.UUID
	UserID        string
	Name          string
	Strategy      string
	Arguments     map[string]json.RawMessage
	StartDate     int64
	Notifications int
}

var disableSend bool = false

func getSavedPortfolios(startDate time.Time) []*savedStrategy {
	ret := []*savedStrategy{}
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
		portfolioSQL := `SELECT id, user_id, name, strategy_shortcode, arguments, extract(epoch from start_date)::int as start_date, notifications FROM portfolio_v1 WHERE start_date <= $1`
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
			p := savedStrategy{}
			err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Strategy, &p.Arguments, &p.StartDate, &p.Notifications)
			if err != nil {
				log.Fatalf("Database query error in notifier: %s", err)
			}
			ret = append(ret, &p)
		}
		trx.Commit(context.Background())
	}

	return ret
}

func updateSavedPortfolioPerformanceMetrics(s *savedStrategy, perf *portfolio.Performance) {
	trx, err := database.TrxForUser(s.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"UserID": s.UserID,
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
			"PortfolioID": s.ID,
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
		s.ID,
	)
	if err != nil {
		log.WithFields(log.Fields{
			"PortfolioID":          s.ID,
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
		"PortfolioID":          s.ID,
		"YTDReturn":            perf.YTDReturn,
		"CagrSinceInception":   perf.CagrSinceInception,
		"PerformanceStartDate": time.Unix(perf.PeriodStart, 0),
		"PerformanceEndDate":   time.Unix(perf.PeriodEnd, 0),
	}).Info("Calculated portfolio performance")
	trx.Commit(context.Background())
}

func saveTransactions(s *savedStrategy, perf *portfolio.Performance) error {
	startTime := time.Now()
	log.WithFields(log.Fields{
		"NumTransactions": len(perf.Transactions),
		"StartTime":       startTime,
	}).Info("Saving transactions to the database")

	trx, err := database.TrxForUser(s.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"UserID": s.UserID,
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
			s.ID,
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
				"PortfolioID": s.ID,
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
	}).Info("Saved transactions to the database")

	return nil
}

func saveMeasurements(s *savedStrategy, perf *portfolio.Performance) error {
	startTime := time.Now()
	log.WithFields(log.Fields{
		"NumMeasurements": len(perf.Measurements),
		"StartTime":       startTime,
	}).Info("Saving measurements to the database")

	trx, err := database.TrxForUser(s.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"UserID": s.UserID,
			"Error":  err,
		}).Error("Failed to get transaction for user")
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
			s.ID,
			measurement.RiskFreeValue,
			measurement.TotalDeposited,
			measurement.TotalWithdrawn,
			measurement.Value,
		)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID": s.ID,
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
	}).Info("Saved measurements to the database")

	return nil
}

func computePortfolioPerformance(p *savedStrategy, through time.Time) (*portfolio.Portfolio, error) {
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
			return nil, err
		}
		manager = data.NewManager(map[string]string{
			"tiingo": u.TiingoToken,
		})
	}

	manager.Begin = time.Unix(p.StartDate, 0)
	manager.End = through
	manager.Frequency = data.FrequencyMonthly

	if strategy, ok := strategies.StrategyMap[p.Strategy]; ok {
		stratObject, err := strategy.Factory(p.Arguments)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		computedPortfolio, err := stratObject.Compute(&manager)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		return computedPortfolio, nil
	}

	log.WithFields(log.Fields{
		"Portfolio": p.ID,
		"Strategy":  p.Strategy,
	}).Error("portfolio strategy not found")
	return nil, errors.New("strategy not found")
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

func processNotifications(forDate time.Time, s *savedStrategy, p *portfolio.Portfolio, perf *portfolio.Performance) {
	if s.UserID == "pvuser" {
		return
	}

	u, err := getUser(s.UserID)
	if err != nil {
		return
	}

	toSend := []string{}

	manager := data.NewManager(map[string]string{
		"tiingo": u.TiingoToken,
	})
	manager.Begin = time.Unix(s.StartDate, 0)

	if (s.Notifications & daily) == daily {
		toSend = append(toSend, "Daily")
	}
	if (s.Notifications & weekly) == weekly {
		// only send on Friday
		if lastTradingDayOfWeek(forDate, &manager) {
			toSend = append(toSend, "Weekly")
		}
	}
	if (s.Notifications & monthly) == monthly {
		if lastTradingDayOfMonth(forDate, &manager) {
			toSend = append(toSend, "Monthly")
		}
	}
	if (s.Notifications & annually) == annually {
		if lastTradingDayOfYear(forDate, &manager) {
			toSend = append(toSend, "Annually")
		}
	}

	for _, freq := range toSend {
		log.Infof("Send %s notification for portfolio %s", freq, s.ID)
		message, err := buildEmail(forDate, freq, s, p, perf, u)
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
			"Portfolio":  s.ID,
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
func buildEmail(forDate time.Time, frequency string, s *savedStrategy,
	p *portfolio.Portfolio, perf *portfolio.Performance, to *User) ([]byte, error) {
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

	person.SetDynamicTemplateData("portfolioName", s.Name)
	if strat, ok := strategies.StrategyMap[s.Strategy]; ok {
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
	for ii, s := range savedPortfolios {
		if *portfolioFlag != "all" && portfolioID == s.ID {
			p, err := computePortfolioPerformance(s, forDate)
			if err != nil {
				log.WithFields(log.Fields{
					"PortfolioID": s.ID,
					"Error":       err,
				}).Error("Failed to compute portfolio performance")
				continue
			}
			perf, err := p.CalculatePerformance(forDate)
			if err != nil {
				log.WithFields(log.Fields{
					"Error":       err,
					"PortfolioID": s.ID,
					"Function":    "portfolio.CalculatePerformance",
					"ForDate":     forDate.String(),
				}).Error("Failed to calculate portfolio performance")
			}
			perf.BuildMetricsBundle()
			updateSavedPortfolioPerformanceMetrics(s, &perf)
			saveTransactions(s, &perf)
			saveMeasurements(s, &perf)
			processNotifications(forDate, s, p, &perf)
			if *limitFlag != 0 && *limitFlag >= ii {
				break
			}
		}
	}
}
