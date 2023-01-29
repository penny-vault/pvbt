// Copyright 2021-2023
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

package messenger

import (
	"errors"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/penny-vault/pv-api/common"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type SimulationRequest struct {
	UserID      string `json:"user_id"`
	PortfolioID string `json:"portfolio_id"`
	RequestTime string `json:"request_time"`
}

// GetSimulationRequet returns a single simulation request message
func GetSimulationRequest() (*nats.Msg, error) {
	sub, err := jetStream.PullSubscribe(viper.GetString("nats.requests_subject"), viper.GetString("nats.requests_consumer"))
	if err != nil {
		log.Error().Err(err).Msg("could not connect to durable consumer (note: make sure the consumer already exists)")
		return nil, err
	}

	msgs, err := sub.Fetch(1)
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			log.Error().Err(err).Msg("could not fetch new messages")
		} else {
			log.Warn().Msg("no requests available in queue")
			return nil, nil
		}
		return nil, err
	}

	if len(msgs) == 0 {
		log.Info().Msg("no simulation requests in queue")
		return nil, nil
	}

	return msgs[0], nil
}

func CreateSimulationRequest(userID string, portfolioID uuid.UUID) error {
	var err error
	nyc := common.GetTimezone()

	subject := viper.GetString("nats.requests_subject")

	req := SimulationRequest{
		UserID:      userID,
		PortfolioID: portfolioID.String(),
		RequestTime: time.Now().In(nyc).String(),
	}

	jsonReq, err := json.Marshal(req)
	if err != nil {
		log.Error().Err(err).Msg("could not serialize request to JSON")
		return err
	}

	if _, err := jetStream.Publish(subject, jsonReq); err != nil {
		log.Error().Err(err).Msg("could not publish a simulation request")
		return err
	}

	return nil
}
