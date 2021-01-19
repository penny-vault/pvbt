package data

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type tiingo struct {
	apikey string
}

var tiingoSymbols = map[string]Asset{}
var tiingoTickersURL = "https://apimedia.tiingo.com/docs/tiingo/daily/supported_tickers.zip"
var tiingoAPI = "https://api.tiingo.com"

// NewTiingo Create a new Tiingo data provider
func NewTiingo(key string) tiingo {
	return tiingo{
		apikey: key,
	}
}

// Parse tickers

// SyncTickers Download tickers from Tiingo
func SyncTickers() {
	log.Printf("[tiingo] Downloading %s\n", tiingoTickersURL)
	csvData, err := downloadZipFile(tiingoTickersURL)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("[tiingo] Parsing tickers CSV")
	err = parseCSV(csvData)
	if err != nil {
		log.Println(err)
	}
	log.Printf("[tiingo] Found %d tickers\n", len(tiingoSymbols))
}

func parseCSV(data []byte) error {
	r := csv.NewReader(bytes.NewReader(data))

	records, err := r.ReadAll()
	if err != nil {
		return err
	}

	// each record consists of [ticker, exchange, kind, currency, startDate, endDate]
	for ii := 1; ii < len(records); ii++ {
		record := records[ii]
		var startDate time.Time
		if record[4] != "" {
			startDate, err = time.Parse("2006-01-02", record[4])
			if err != nil {
				log.Printf("[tiingo] Could not parse date: '%s'\n", record[4])
			}
		}

		asset := Asset{
			Ticker:    record[0],
			Exchange:  record[1],
			Kind:      record[2],
			Currency:  record[3],
			StartDate: startDate,
		}

		tiingoSymbols[asset.Ticker] = asset
	}

	return nil
}

func downloadZipFile(url string) (data []byte, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}

	// Read all the files from zip archive - and return the first
	for _, zipFile := range zipReader.File {
		unzippedFileBytes, err := readZipFile(zipFile)
		if err != nil {
			return nil, err
		}

		return unzippedFileBytes, nil
	}

	return nil, errors.New("no files in downloaded zip")
}

func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

// Interface functions

func (t tiingo) HasKey(symbol string) bool {
	return false
}

/*
func (t tiingo) GetDataForPeriod(symbols []string, frequency string, begin time.Time, end time.Time) qframe.QFrame {
	return qframe.New(map[string]interface{})
}
*/
