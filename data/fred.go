package data

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	imports "github.com/rocketlaunchr/dataframe-go/imports"
)

var fredURL = "https://fred.stlouisfed.org"

type fred struct{}

// NewFred Create a new Fred data provider
func NewFred() fred {
	return fred{}
}

// Interface functions

func (f fred) DataType() string {
	return "rate"
}

func (f fred) GetDataForPeriod(symbol string, frequency string,
	metric string, begin time.Time,
	end time.Time) (data *dataframe.DataFrame, err error) {
	// build URL to get data
	url := fmt.Sprintf("%s/graph/fredgraph.csv?mode=fred&id=%s&cosd=%s&coed=%s&fq=%s&fam=avg", fredURL, symbol, begin.Format("2006-01-02"), end.Format("2006-01-02"), frequency)
	//log.Printf("Download from FRED: %s\n", url)

	resp, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP request returned invalid status code: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	tz, err := time.LoadLocation("America/New_York") // New York is the reference time
	if err != nil {
		return nil, err
	}

	res, err := imports.LoadFromCSV(context.TODO(), bytes.NewReader(body), imports.CSVLoadOptions{
		DictateDataType: map[string]interface{}{
			DateIdx: imports.Converter{
				ConcreteType: time.Time{},
				ConverterFunc: func(in interface{}) (interface{}, error) {
					return time.ParseInLocation("2006-01-02", in.(string), tz)
				},
			},
			symbol: imports.Converter{
				ConcreteType: float64(0),
				ConverterFunc: func(in interface{}) (interface{}, error) {
					v, err := strconv.ParseFloat(in.(string), 64)
					if err != nil {
						return math.NaN(), nil
					}
					return v, nil
				},
			},
		},
	})

	return res, err
}
