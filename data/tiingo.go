package data

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	imports "github.com/rocketlaunchr/dataframe-go/imports"
)

type tiingo struct {
	apikey string
}

var tiingoTickersURL = "https://apimedia.tiingo.com/docs/tiingo/daily/supported_tickers.zip"
var tiingoAPI = "https://api.tiingo.com"

// NewTiingo Create a new Tiingo data provider
func NewTiingo(key string) tiingo {
	return tiingo{
		apikey: key,
	}
}

// Interface functions

func (t tiingo) DataType() string {
	return "security"
}

func (t tiingo) GetDataForPeriod(symbol string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	// build URL to get data
	url := fmt.Sprintf("%s/tiingo/daily/%s/prices?startDate=%s&endDate=%s&format=csv&resampleFreq=%s", tiingoAPI, symbol, begin.Format("2006-01-02"), end.Format("2006-01-02"), frequency)
	log.Printf("Download from Tiingo: %s\n", url)

	resp, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	floatConverter := imports.Converter{
		ConcreteType: float64(0),
		ConverterFunc: func(in interface{}) (interface{}, error) {
			v, err := strconv.ParseFloat(in.(string), 64)
			if err != nil {
				return math.NaN(), nil
			}
			return v, nil
		},
	}

	res, err := imports.LoadFromCSV(context.TODO(), bytes.NewReader(body), imports.CSVLoadOptions{
		DictateDataType: map[string]interface{}{
			"date": imports.Converter{
				ConcreteType: time.Time{},
				ConverterFunc: func(in interface{}) (interface{}, error) {
					return time.Parse("2006-01-02", in.(string))
				},
			},
			"open":      floatConverter,
			"high":      floatConverter,
			"low":       floatConverter,
			"close":     floatConverter,
			"volume":    floatConverter,
			"adjOpen":   floatConverter,
			"adjHigh":   floatConverter,
			"adjLow":    floatConverter,
			"adjClose":  floatConverter,
			"adjVolume": floatConverter,
		},
	})

	return res, err
}
