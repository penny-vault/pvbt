// Copyright 2021-2022
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
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var natsConnection *nats.Conn
var jetStream nats.JetStreamContext

// Connect to the nats server
func Initialize() error {
	var err error
	url := viper.GetString("nats.server")
	credentialsFile := viper.GetString("nats.credentials")
	log.Info().Str("NATSServer", url).Str("Credentials", credentialsFile).Msg("connecting to NATS server")
	if natsConnection, err = nats.Connect(url, nats.UserCredentials(credentialsFile)); err != nil {
		log.Error().Err(err).Msg("could not connect to NATS server")
		return err
	}

	// get jetstream connection
	jetStream, err = natsConnection.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		log.Error().Err(err).Msg("could not create jetstream context")
		return err
	}

	return nil
}
