package newrelicapi

import (
	"os"

	"github.com/newrelic/go-agent/v3/newrelic"
	log "github.com/sirupsen/logrus"
)

var NewRelicApp *newrelic.Application = nil

func InitializeNewRelic() {
	// setup new relic if the environment variables are set
	if os.Getenv("NEW_RELIC_LICENSE_KEY") != "" {
		var err error
		NewRelicApp, err = newrelic.NewApplication(
			newrelic.ConfigAppName("pv-api"),
			newrelic.ConfigLicense(os.Getenv("NEW_RELIC_LICENSE_KEY")),
		)
		if err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Warn("Failed to initialize New Relic monitorying")
		} else {
			log.Info("Initialized New Relic API")
		}
	} else {
		log.Warn(`Skipping NewRelic configuration because env["NEW_RELIC_LICENSE_KEY"] is empty`)
	}
}