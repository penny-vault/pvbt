package filter

import (
	"fmt"
	"main/common"
	"main/portfolio"
	"time"

	log "github.com/sirupsen/logrus"
)

type FilterInterface interface {
	GetMeasurements(field1 string, field2 string, since time.Time) ([]byte, error)
	GetHoldings(frequency string, since time.Time) ([]byte, error)
	GetTransactions(since time.Time) ([]byte, error)
}

func getPortfolio(portfolioID string) *portfolio.Portfolio {
	raw, _ := common.CacheGet(portfolioID)
	if len(raw) > 0 {
		port := portfolio.Portfolio{}
		_, err := port.Unmarshal(raw)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID": portfolioID,
				"Error":       err,
			}).Error("failed to deserialize portfolio")
			return nil
		}
		return &port
	}
	return nil
}

func getPerformance(portfolioID string) *portfolio.Performance {
	raw, _ := common.CacheGet(fmt.Sprintf("%s:performance", portfolioID))
	if len(raw) > 0 {
		perf := portfolio.Performance{}
		_, err := perf.Unmarshal(raw)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID": portfolioID,
				"Error":       err,
			}).Error("failed to deserialize portfolio")
			return nil
		}
		return &perf
	}
	return nil
}

func New(portfolioID string, userID string) FilterInterface {
	var perf *portfolio.Performance
	port := getPortfolio(portfolioID)
	if port != nil {
		perf = getPerformance(portfolioID)
		return &FilterObject{
			Performance: perf,
			Portfolio:   port,
		}
	}

	db := FilterDatabase{
		PortfolioID: portfolioID,
		UserID:      userID,
	}

	return &db
}
