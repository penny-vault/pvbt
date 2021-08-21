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
}

func New(portfolioID string, userID string) FilterInterface {
	raw, _ := common.CacheGet(fmt.Sprintf("%s:performance", portfolioID))
	if len(raw) > 0 {
		p := portfolio.Performance{}
		_, err := p.Unmarshal(raw)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID": portfolioID,
				"Error":       err,
				"UserID":      userID,
			}).Error("failed to deserialize portfolio")
		}
		return &FilterObject{
			Performance: p,
		}
	}

	db := FilterDatabase{
		PortfolioID: portfolioID,
		UserID:      userID,
	}

	return &db
}
