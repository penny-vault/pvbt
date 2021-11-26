// Copyright 2021 JD Fergason
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package data

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"main/common"
	"main/dfextras"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	imports "github.com/rocketlaunchr/dataframe-go/imports"
	log "github.com/sirupsen/logrus"
)

var fredURL = "https://fred.stlouisfed.org"

type fred struct{}

// NewFred Create a new Fred data provider
func NewFred() *fred {
	return &fred{}
}

// Interface functions

func (f *fred) DataType() string {
	return "rate"
}

func (f *fred) GetDataForPeriod(symbols []string, metric string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	res := make([]*dataframe.DataFrame, 0, len(symbols))
	errs := []error{}
	ch := make(chan quoteResult)

	for _, chunk := range partitionArray(symbols, 10) {
		for ii := range chunk {
			go fredDownloadWorker(ch, strings.ToUpper(chunk[ii]), metric, frequency, begin, end, f)
		}

		for range chunk {
			v := <-ch
			if v.Err == nil {
				res = append(res, v.Data)
			} else {
				log.WithFields(log.Fields{
					"Ticker": v.Ticker,
					"Error":  v.Err,
				}).Warn("Cannot download ticker data")
				errs = append(errs, v.Err)
			}
		}
	}

	if len(errs) != 0 {
		return nil, errs[0]
	}

	return dfextras.MergeAndFill(context.Background(), res...)
}

func fredDownloadWorker(result chan<- quoteResult, symbol string, metric string, frequency string, begin time.Time, end time.Time, f *fred) {
	df, err := f.loadDataForPeriod(symbol, metric, frequency, begin, end)
	res := quoteResult{
		Ticker: symbol,
		Data:   df,
		Err:    err,
	}
	result <- res
}

func (f *fred) loadDataForPeriod(symbol string, metric string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
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
			common.DateIdx: imports.Converter{
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
