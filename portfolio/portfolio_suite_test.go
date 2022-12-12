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

package portfolio_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

func TestPortfolio(t *testing.T) {
	// setup logging
	//nolint:reassign
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	log.Logger = log.With().Caller().Logger()
	log.Logger = log.Output(GinkgoWriter)

	RegisterFailHandler(Fail)

	log.Logger = log.Output(GinkgoWriter)
	RegisterFailHandler(Fail)

	dbPool, err := pgxmock.NewConn()
	Expect(err).To(BeNil())
	database.SetPool(dbPool)

	pgxmockhelper.MockManager(dbPool)
	data.GetManagerInstance()

	RunSpecs(t, "Portfolio Suite")
}
