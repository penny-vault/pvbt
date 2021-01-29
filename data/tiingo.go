package data

import (
	"bytes"
	"context"
	"errors"
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

func (t tiingo) GetDataForPeriod(symbol string, metric string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	// build URL to get data
	var url string
	nullTime := time.Time{}
	if begin == nullTime || end == nullTime {
		url = fmt.Sprintf("%s/tiingo/daily/%s/prices?format=csv&resampleFreq=%s&token=%s", tiingoAPI, symbol, frequency, t.apikey)
	} else {
		url = fmt.Sprintf("%s/tiingo/daily/%s/prices?startDate=%s&endDate=%s&format=csv&resampleFreq=%s&token=%s", tiingoAPI, symbol, begin.Format("2006-01-02"), end.Format("2006-01-02"), frequency, t.apikey)
	}
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

	err = nil
	var timeSeries dataframe.Series
	var valueSeries dataframe.Series

	timeSeriesIdx, err := res.NameToColumn("date")
	if err != nil {
		return nil, errors.New("Cannot find time series")
	}

	timeSeries = res.Series[timeSeriesIdx]
	timeSeries.Rename("DATE")

	switch metric {
	case MetricOpen:
		valueSeriesIdx, err := res.NameToColumn("open")
		if err != nil {
			return nil, errors.New("open metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricHigh:
		valueSeriesIdx, err := res.NameToColumn("high")
		if err != nil {
			return nil, errors.New("high metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricLow:
		valueSeriesIdx, err := res.NameToColumn("low")
		if err != nil {
			return nil, errors.New("low metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricClose:
		valueSeriesIdx, err := res.NameToColumn("close")
		if err != nil {
			return nil, errors.New("close metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricVolume:
		valueSeriesIdx, err := res.NameToColumn("volume")
		if err != nil {
			return nil, errors.New("volume metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricAdjustedOpen:
		valueSeriesIdx, err := res.NameToColumn("adjOpen")
		if err != nil {
			return nil, errors.New("Adjusted open metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricAdjustedHigh:
		valueSeriesIdx, err := res.NameToColumn("adjHigh")
		if err != nil {
			return nil, errors.New("Adjusted high metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricAdjustedLow:
		valueSeriesIdx, err := res.NameToColumn("adjLow")
		if err != nil {
			return nil, errors.New("Adjusted low metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	case MetricAdjustedClose:
		valueSeriesIdx, err := res.NameToColumn("adjClose")
		if err != nil {
			return nil, errors.New("Adjsuted close metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx]
	default:
		return nil, errors.New("Un-supported metric")
	}

	if err != nil {
		return nil, err
	}

	valueSeries.Rename(symbol)
	df := dataframe.NewDataFrame(timeSeries, valueSeries)

	return df, err
}
