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

package portfolio

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/penny-vault/pv-api/strategies"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/spf13/viper"
)

type NotificationFrequency int32

const (
	NotifyDaily    NotificationFrequency = 0x00000010
	NotifyWeekly   NotificationFrequency = 0x00000100
	NotifyMonthly  NotificationFrequency = 0x00001000
	NotifyAnnually NotificationFrequency = 0x00010000
)

type Notification struct {
	ForDate      time.Time
	ForFrequency NotificationFrequency
	Holdings     map[string]float64
	Portfolio    *Portfolio
	PeriodReturn float64
	YTDReturn    float64
}

func (nf NotificationFrequency) String() string {
	switch nf {
	case NotifyDaily:
		return "Daily"
	case NotifyWeekly:
		return "Weekly"
	case NotifyMonthly:
		return "Monthly"
	case NotifyAnnually:
		return "Annually"
	default:
		return "Unknown NotificationFrequency"
	}
}

// RequestedNotificationsForDate evaluates a portfolios notifications against the requested date
// and returns those notifications that are valid for the given date.
func (pm *Model) RequestedNotificationsForDate(forDate time.Time) []NotificationFrequency {
	frequencies := make([]NotificationFrequency, 0)
	notificationNames := make([]string, 0)

	if NotificationFrequency(pm.Portfolio.Notifications&int32(NotifyDaily)) == NotifyDaily {
		tradeCron, err := tradecron.New("@open", tradecron.RegularHours)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("daily notifications is specified but could not create tradecron instance")
		}
		isTradeDay, err := tradeCron.IsTradeDay(forDate)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("daily notifications is specified but could not evaluate if forDate is a trading day")
		}
		if isTradeDay {
			frequencies = append(frequencies, NotifyDaily)
			notificationNames = append(notificationNames, "Daily")
		}
	}

	if NotificationFrequency(pm.Portfolio.Notifications&int32(NotifyWeekly)) == NotifyWeekly {
		tradeCron, err := tradecron.New("@weekend", tradecron.RegularHours)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("weekly notifications is specified but could not create tradecron instance")
		}
		isTradeDay, err := tradeCron.IsTradeDay(forDate)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("weekly notifications is specified but could not evaluate if forDate is a trading day")
		}
		if isTradeDay {
			frequencies = append(frequencies, NotifyWeekly)
			notificationNames = append(notificationNames, "Weekly")
		}
	}

	if NotificationFrequency(pm.Portfolio.Notifications&int32(NotifyMonthly)) == NotifyMonthly {
		tradeCron, err := tradecron.New("@monthend", tradecron.RegularHours)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("monthly notifications is specified but could not create tradecron instance")
		}
		isTradeDay, err := tradeCron.IsTradeDay(forDate)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("monthly notifications is specified but could not evaluate if forDate is a trading day")
		}
		if isTradeDay {
			frequencies = append(frequencies, NotifyMonthly)
			notificationNames = append(notificationNames, "Monthly")
		}
	}

	if NotificationFrequency(pm.Portfolio.Notifications&int32(NotifyAnnually)) == NotifyAnnually {
		tradeCron, err := tradecron.New("@monthend * * * 12 *", tradecron.RegularHours)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("annually notifications is specified but could not create tradecron instance")
		}
		isTradeDay, err := tradeCron.IsTradeDay(forDate)
		if err != nil {
			log.Error().Err(err).Time("forDate", forDate).Msg("annually notifications is specified but could not evaluate if forDate is a trading day")
		}
		if isTradeDay {
			frequencies = append(frequencies, NotifyAnnually)
			notificationNames = append(notificationNames, "Annually")
		}
	}

	log.Debug().Str("PortfolioID", hex.EncodeToString(pm.Portfolio.ID)).Strs("Notifications", notificationNames).Msg("enabled notifications")

	return frequencies
}

func (pm *Model) NotificationsForDate(forDate time.Time, perf *Performance) []*Notification {
	// get notification frequencies for date
	frequencies := pm.RequestedNotificationsForDate(forDate)
	notifications := make([]*Notification, 0, len(frequencies))

	// process for each frequency
	for _, freq := range frequencies {
		measurement, err := LoadMeasurementFromDB(pm.Portfolio.ID, pm.Portfolio.UserID, forDate)
		if err != nil {
			log.Error().Err(err).Str("NotificationFrequency", freq.String()).Msg("could not retrieve measurement for portfolio")
			continue
		}
		notifications = append(notifications, &Notification{
			ForDate:      forDate,
			ForFrequency: freq,
			Holdings:     pm.holdings,
			Portfolio:    pm.Portfolio,
			PeriodReturn: getReturnForNotification(measurement, freq),
			YTDReturn:    perf.PortfolioReturns.TWRRYTD,
		})
	}
	return notifications
}

func (n *Notification) SendEmail(userFullName string, emailAddress string) error {
	subLog := log.With().Str("UserFullName", userFullName).Str("EmailAddress", emailAddress).Str("PortfolioName", n.Portfolio.Name).Str("PortfolioID", hex.EncodeToString(n.Portfolio.ID)).Logger()
	subLog.Info().Msg("sending notification e-mail")
	m := mail.NewV3Mail()

	e := mail.NewEmail(viper.GetString("email.name"), viper.GetString("email.address"))
	m.SetFrom(e)

	m.SetTemplateID(viper.GetString("sendgrid.template"))

	person := mail.NewPersonalization()
	toList := []*mail.Email{
		mail.NewEmail(userFullName, emailAddress),
	}
	person.AddTos(toList...)

	person.SetDynamicTemplateData("portfolioName", n.Portfolio.Name)
	strategy := n.Portfolio.StrategyShortcode
	if strat, ok := strategies.StrategyMap[strategy]; ok {
		person.SetDynamicTemplateData("strategy", strat.Name)
	}

	person.SetDynamicTemplateData("frequency", n.ForFrequency.String())
	person.SetDynamicTemplateData("forDate", n.ForDate.Format(viper.GetString("email.date_format")))
	person.SetDynamicTemplateData("currentAsset", holdingsString(n.Holdings))

	person.SetDynamicTemplateData("periodReturn", formatPercent(n.PeriodReturn))
	person.SetDynamicTemplateData("ytdReturn", formatPercent(n.YTDReturn))

	m.AddPersonalizations(person)
	msgBody := mail.GetRequestBody(m)

	request := sendgrid.GetRequest(viper.GetString("sendgrid.apikey"), "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = msgBody

	response, err := sendgrid.API(request)
	if err != nil {
		log.Error().Err(err).Int("StatusCode", response.StatusCode).Msg("could not send message")
		return err
	}

	log.Info().Str("ToAddress", emailAddress).Int("StatusCode", response.StatusCode).Strs("MessageID", response.Headers["X-Message-Id"]).Msg("sent notification email")
	return nil
}

func holdingsString(h map[string]float64) string {
	assets := make([]string, 0, len(h))
	for k := range h {
		assets = append(assets, k)
	}

	return strings.Join(assets, ", ")
}

func formatPercent(percent float64) string {
	return fmt.Sprintf("%.2f%%", percent*100)
}

func getReturnForNotification(measurement *PerformanceMeasurement, freq NotificationFrequency) float64 {
	switch freq {
	case NotifyDaily:
		return float64(measurement.TWRROneDay)
	case NotifyWeekly:
		return float64(measurement.TWRRWeekToDate)
	case NotifyMonthly:
		return float64(measurement.TWRRMonthToDate)
	case NotifyAnnually:
		return float64(measurement.TWRRYearToDate)
	default:
		log.Error().Str("NotificationFrequency", freq.String()).Msg("unknown frequency")
		return 0.0
	}
}
