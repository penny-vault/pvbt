package strategies

import (
	"embed"
	"fmt"
	"io/ioutil"
	"main/database"
	"main/strategies/adm"
	"main/strategies/daa"
	"main/strategies/paa"
	"main/strategies/strategy"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
	log "github.com/sirupsen/logrus"
)

//go:embed **/*.md **/*.toml
var resources embed.FS

// StrategyList List of all strategies
var StrategyList = []strategy.StrategyInfo{}

// StrategyMap Map of strategies
var StrategyMap = make(map[string]*strategy.StrategyInfo)

// StrategyMetrics map of updated metrics for each strategy - this is used by the StrategyInfo constrcutors and the GetStrategies endpoint
var StrategyMetricsMap = make(map[string]strategy.StrategyMetrics)

// InitializeStrategyMap configure the strategy map
func InitializeStrategyMap() {
	Register("adm", adm.New)
	Register("daa", daa.New)
	Register("paa", paa.New)
}

func Register(strategyPkg string, factory strategy.StrategyFactory) {
	// read description
	fn := fmt.Sprintf("%s/description.md", strategyPkg)
	file, err := resources.Open(fn)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to open file")
	}
	defer file.Close()
	doc, err := ioutil.ReadAll(file)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to read file")
	}
	longDescription := string(doc)

	// load config file
	fn = fmt.Sprintf("%s/strategy.toml", strategyPkg)
	file, err = resources.Open(fn)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to open file")
	}
	defer file.Close()
	doc, err = ioutil.ReadAll(file)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to read file")
	}

	var strat strategy.StrategyInfo
	err = toml.Unmarshal(doc, &strat)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to parse toml file")
	}

	strat.LongDescription = longDescription
	strat.Factory = factory

	StrategyList = append(StrategyList, strat)
	StrategyMap[strat.Shortcode] = &strat
}

// Ensure all strategies have portfolio entries in the database so metrics are calculated
func LoadStrategyMetricsFromDb() {
	log.Info("refreshing portfolio metrics")
StrategyLoop:
	for ii := range StrategyList {
		strat := StrategyList[ii]

		// check if this strategy already has a portfolio associated with it
		rows, err := database.Conn.Query("SELECT id, cagr_3yr, cagr_5yr, cagr_10yr, std_dev, downside_deviation, max_draw_down, avg_draw_down, sharpe_ratio, sortino_ratio, ulcer_index, ytd_return, cagr_since_inception FROM Portfolio WHERE userid='system' AND name=$1", strat.Name)
		if err != nil {
			log.WithFields(log.Fields{
				"Strategy": strat.Shortcode,
				"Error":    err,
			}).Error("failed to lookup strategy portfolio")
			return
		} else {
			for rows.Next() {
				s := strategy.StrategyMetrics{}
				rows.Scan(&s.ID, &s.CagrThreeYr, &s.CagrFiveYr, &s.CagrTenYr, &s.StdDev, &s.DownsideDeviation, &s.MaxDrawDown, &s.AvgDrawDown, &s.SharpeRatio, &s.SortinoRatio, &s.UlcerIndex, &s.YTDReturn, &s.CagrSinceInception)
				StrategyMetricsMap[strat.Shortcode] = s
				strat.Metrics = s
				continue StrategyLoop
			}
		}

		// It seems this strategy doesn't have a database entry yet -- create one
		portfolioID := uuid.New()
		// build arguments
		argumentsMap := make(map[string]interface{})
		for k, v := range strat.Arguments {
			var output interface{}
			if v.Typecode == "string" {
				output = v.Default
			} else {
				json.Unmarshal([]byte(v.Default), &output)
			}
			argumentsMap[k] = output
		}
		arguments, err := json.Marshal(argumentsMap)
		if err != nil {
			log.WithFields(log.Fields{
				"Shortcode":    strat.Shortcode,
				"StrategyName": strat.Name,
				"Error":        err,
			}).Warn("Unable to build arguments for metrics calculation")
		}
		portfolioSQL := `INSERT INTO Portfolio ("id", "userid", "name", "strategy_shortcode", "arguments", "start_date") VALUES ($1, $2, $3, $4, $5, $6)`
		_, err = database.Conn.Exec(portfolioSQL, portfolioID, "system", strat.Name, strat.Shortcode, string(arguments), time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			log.WithFields(log.Fields{
				"Shortcode":    strat.Shortcode,
				"StrategyName": strat.Name,
				"Error":        err,
			}).Warn("failed to create portfolio for strategy metrics")
		}
	}
}
