// Copyright 2021-2026
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

package portfolio

import "time"

// Side indicates a buy or sell direction.
type Side int

const (
	Buy Side = iota
	Sell
)

// OrderModifier adjusts the behavior of an order. Modifiers fall into two
// categories: order type (price conditions) and time in force (lifetime).
// Each modifier is an unexported struct with an exported constructor or
// package-level variable.
type OrderModifier interface {
	orderModifier()
}

// --- Order type modifiers ---

type limitModifier struct{ price float64 }

func (limitModifier) orderModifier() {}

// Limit sets a maximum buy price or minimum sell price.
func Limit(price float64) OrderModifier { return limitModifier{price: price} }

type stopModifier struct{ price float64 }

func (stopModifier) orderModifier() {}

// Stop triggers a market order when the price reaches a threshold (stop loss).
func Stop(price float64) OrderModifier { return stopModifier{price: price} }

// --- Time in force modifiers ---

type dayOrderModifier struct{}

func (dayOrderModifier) orderModifier() {}

// DayOrder cancels the order at market close if not executed. This is the
// default time in force when no modifier is specified.
var DayOrder OrderModifier = dayOrderModifier{}

type goodTilCancelModifier struct{}

func (goodTilCancelModifier) orderModifier() {}

// GoodTilCancel keeps the order open until filled or cancelled (up to 180
// days at most brokers).
var GoodTilCancel OrderModifier = goodTilCancelModifier{}

type fillOrKillModifier struct{}

func (fillOrKillModifier) orderModifier() {}

// FillOrKill requires the order to be filled entirely or not at all.
var FillOrKill OrderModifier = fillOrKillModifier{}

type immediateOrCancelModifier struct{}

func (immediateOrCancelModifier) orderModifier() {}

// ImmediateOrCancel fills as many shares as possible immediately and
// cancels the remainder.
var ImmediateOrCancel OrderModifier = immediateOrCancelModifier{}

type onTheOpenModifier struct{}

func (onTheOpenModifier) orderModifier() {}

// OnTheOpen fills only at the opening price.
var OnTheOpen OrderModifier = onTheOpenModifier{}

type onTheCloseModifier struct{}

func (onTheCloseModifier) orderModifier() {}

// OnTheClose fills only at the closing price.
var OnTheClose OrderModifier = onTheCloseModifier{}

type goodTilDateModifier struct{ date time.Time }

func (goodTilDateModifier) orderModifier() {}

// GoodTilDate keeps the order open until a specified date.
func GoodTilDate(t time.Time) OrderModifier { return goodTilDateModifier{date: t} }
