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

import (
	"time"

	"github.com/penny-vault/pvbt/broker"
)

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

type justificationModifier struct{ reason string }

func (justificationModifier) orderModifier() {}

// WithJustification attaches an explanation to the resulting transaction.
func WithJustification(reason string) OrderModifier {
	return justificationModifier{reason: reason}
}

type lotSelectionModifier struct{ method LotSelection }

func (lotSelectionModifier) orderModifier() {}

// WithLotSelection overrides the portfolio default lot selection for this order.
func WithLotSelection(method LotSelection) OrderModifier {
	return lotSelectionModifier{method: method}
}

// --- Bracket and OCO modifiers ---

// ExitTarget describes a single exit condition for a bracket order. Either
// AbsolutePrice or PercentOffset is set, but not both.
type ExitTarget struct {
	AbsolutePrice float64
	PercentOffset float64
}

// StopLossPrice creates an ExitTarget that triggers a stop at the given
// absolute price.
func StopLossPrice(price float64) ExitTarget {
	return ExitTarget{AbsolutePrice: price}
}

// StopLossPercent creates an ExitTarget that triggers a stop at the given
// percentage below the entry price.
func StopLossPercent(pct float64) ExitTarget {
	return ExitTarget{PercentOffset: pct}
}

// TakeProfitPrice creates an ExitTarget that closes the position at the
// given absolute price.
func TakeProfitPrice(price float64) ExitTarget {
	return ExitTarget{AbsolutePrice: price}
}

// TakeProfitPercent creates an ExitTarget that closes the position at the
// given percentage above the entry price.
func TakeProfitPercent(pct float64) ExitTarget {
	return ExitTarget{PercentOffset: pct}
}

// OCOLeg describes one leg of an OCO (one-cancels-other) order pair.
type OCOLeg struct {
	OrderType broker.OrderType
	Price     float64
}

// StopLeg creates an OCOLeg with a Stop order type at the given price.
func StopLeg(price float64) OCOLeg {
	return OCOLeg{OrderType: broker.Stop, Price: price}
}

// LimitLeg creates an OCOLeg with a Limit order type at the given price.
func LimitLeg(price float64) OCOLeg {
	return OCOLeg{OrderType: broker.Limit, Price: price}
}

type bracketModifier struct {
	stopLoss   ExitTarget
	takeProfit ExitTarget
}

func (bracketModifier) orderModifier() {}

// WithBracket attaches a bracket order to the primary order, automatically
// placing a stop-loss and a take-profit exit when the primary order fills.
func WithBracket(stopLoss, takeProfit ExitTarget) OrderModifier {
	return bracketModifier{stopLoss: stopLoss, takeProfit: takeProfit}
}

type ocoModifier struct {
	legA OCOLeg
	legB OCOLeg
}

func (ocoModifier) orderModifier() {}

// OCO submits two exit legs as a one-cancels-other pair: when one leg fills
// the other is automatically cancelled.
func OCO(legA, legB OCOLeg) OrderModifier {
	return ocoModifier{legA: legA, legB: legB}
}
