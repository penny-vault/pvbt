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

package broker_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("GroupType", func() {
	It("has GroupOCO equal to 1", func() {
		Expect(int(broker.GroupOCO)).To(Equal(1))
	})

	It("has GroupBracket equal to 2", func() {
		Expect(int(broker.GroupBracket)).To(Equal(2))
	})
})

var _ = Describe("GroupRole", func() {
	It("has RoleEntry equal to 1", func() {
		Expect(int(broker.RoleEntry)).To(Equal(1))
	})

	It("has RoleStopLoss equal to 2", func() {
		Expect(int(broker.RoleStopLoss)).To(Equal(2))
	})

	It("has RoleTakeProfit equal to 3", func() {
		Expect(int(broker.RoleTakeProfit)).To(Equal(3))
	})

	It("zero value is distinguishable from all named roles", func() {
		var zeroRole broker.GroupRole
		Expect(zeroRole).NotTo(Equal(broker.RoleEntry))
		Expect(zeroRole).NotTo(Equal(broker.RoleStopLoss))
		Expect(zeroRole).NotTo(Equal(broker.RoleTakeProfit))
	})
})

var _ = Describe("Order", func() {
	It("has GroupID and GroupRole fields", func() {
		order := broker.Order{
			GroupID:   "group-1",
			GroupRole: broker.RoleEntry,
		}
		Expect(order.GroupID).To(Equal("group-1"))
		Expect(order.GroupRole).To(Equal(broker.RoleEntry))
	})

	It("GroupID defaults to empty string", func() {
		var order broker.Order
		Expect(order.GroupID).To(BeEmpty())
	})

	It("GroupRole defaults to zero (no role assigned)", func() {
		var order broker.Order
		Expect(int(order.GroupRole)).To(Equal(0))
	})
})
