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

package risk_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine/middleware/risk"
)

// helpers to create pointer values inline
func fp(val float64) *float64 { return &val }
func ip(val int) *int         { return &val }

var _ = Describe("RiskConfig", func() {
	Describe("Validate", func() {
		Describe("risk profile validation", func() {
			It("accepts empty profile", func() {
				rc := risk.RiskConfig{Profile: ""}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts conservative profile", func() {
				rc := risk.RiskConfig{Profile: "conservative"}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts moderate profile", func() {
				rc := risk.RiskConfig{Profile: "moderate"}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts aggressive profile", func() {
				rc := risk.RiskConfig{Profile: "aggressive"}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts none profile", func() {
				rc := risk.RiskConfig{Profile: "none"}
				Expect(rc.Validate()).To(Succeed())
			})

			It("rejects unknown profile", func() {
				rc := risk.RiskConfig{Profile: "ultra"}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("unknown profile")))
			})
		})

		Describe("MaxPositionSize validation", func() {
			It("rejects negative value", func() {
				rc := risk.RiskConfig{MaxPositionSize: fp(-0.01)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("max_position_size")))
			})

			It("rejects value greater than 1.0", func() {
				rc := risk.RiskConfig{MaxPositionSize: fp(1.01)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("max_position_size")))
			})

			It("accepts value of 0", func() {
				rc := risk.RiskConfig{MaxPositionSize: fp(0)}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts value of 1.0", func() {
				rc := risk.RiskConfig{MaxPositionSize: fp(1.0)}
				Expect(rc.Validate()).To(Succeed())
			})
		})

		Describe("MaxPositionCount validation", func() {
			It("rejects negative value", func() {
				rc := risk.RiskConfig{MaxPositionCount: ip(-1)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("max_position_count")))
			})

			It("accepts zero", func() {
				rc := risk.RiskConfig{MaxPositionCount: ip(0)}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts positive value", func() {
				rc := risk.RiskConfig{MaxPositionCount: ip(10)}
				Expect(rc.Validate()).To(Succeed())
			})
		})

		Describe("DrawdownCircuitBreaker validation", func() {
			It("rejects negative value", func() {
				rc := risk.RiskConfig{DrawdownCircuitBreaker: fp(-0.01)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("drawdown_circuit_breaker")))
			})

			It("rejects value greater than 1.0", func() {
				rc := risk.RiskConfig{DrawdownCircuitBreaker: fp(1.01)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("drawdown_circuit_breaker")))
			})

			It("accepts value between 0 and 1.0", func() {
				rc := risk.RiskConfig{DrawdownCircuitBreaker: fp(0.15)}
				Expect(rc.Validate()).To(Succeed())
			})
		})

		Describe("VolatilityScalerLookback validation", func() {
			It("rejects zero", func() {
				rc := risk.RiskConfig{VolatilityScalerLookback: ip(0)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("volatility_scaler_lookback")))
			})

			It("rejects negative value", func() {
				rc := risk.RiskConfig{VolatilityScalerLookback: ip(-5)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("volatility_scaler_lookback")))
			})

			It("accepts value >= 1", func() {
				rc := risk.RiskConfig{VolatilityScalerLookback: ip(60)}
				Expect(rc.Validate()).To(Succeed())
			})
		})

		Describe("GrossExposureLimit validation", func() {
			It("rejects negative value", func() {
				rc := risk.RiskConfig{GrossExposureLimit: fp(-0.1)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("gross_exposure_limit")))
			})

			It("accepts zero", func() {
				rc := risk.RiskConfig{GrossExposureLimit: fp(0)}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts positive value", func() {
				rc := risk.RiskConfig{GrossExposureLimit: fp(1.5)}
				Expect(rc.Validate()).To(Succeed())
			})
		})

		Describe("NetExposureLimit validation", func() {
			It("rejects negative value", func() {
				rc := risk.RiskConfig{NetExposureLimit: fp(-0.1)}
				Expect(rc.Validate()).To(MatchError(ContainSubstring("net_exposure_limit")))
			})

			It("accepts zero", func() {
				rc := risk.RiskConfig{NetExposureLimit: fp(0)}
				Expect(rc.Validate()).To(Succeed())
			})

			It("accepts positive value", func() {
				rc := risk.RiskConfig{NetExposureLimit: fp(1.0)}
				Expect(rc.Validate()).To(Succeed())
			})
		})
	})

	Describe("ProfileBaseline", func() {
		It("returns conservative baseline with correct values", func() {
			bl := risk.ProfileBaseline("conservative")
			Expect(bl.Profile).To(Equal("conservative"))
			Expect(bl.MaxPositionSize).NotTo(BeNil())
			Expect(*bl.MaxPositionSize).To(Equal(0.20))
			Expect(bl.DrawdownCircuitBreaker).NotTo(BeNil())
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.10))
			Expect(bl.VolatilityScalerLookback).NotTo(BeNil())
			Expect(*bl.VolatilityScalerLookback).To(Equal(60))
		})

		It("returns moderate baseline with correct values", func() {
			bl := risk.ProfileBaseline("moderate")
			Expect(bl.Profile).To(Equal("moderate"))
			Expect(bl.MaxPositionSize).NotTo(BeNil())
			Expect(*bl.MaxPositionSize).To(Equal(0.25))
			Expect(bl.DrawdownCircuitBreaker).NotTo(BeNil())
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.15))
			Expect(bl.VolatilityScalerLookback).To(BeNil())
		})

		It("returns aggressive baseline with correct values", func() {
			bl := risk.ProfileBaseline("aggressive")
			Expect(bl.Profile).To(Equal("aggressive"))
			Expect(bl.MaxPositionSize).NotTo(BeNil())
			Expect(*bl.MaxPositionSize).To(Equal(0.35))
			Expect(bl.DrawdownCircuitBreaker).NotTo(BeNil())
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.25))
		})

		It("returns zero config for none", func() {
			bl := risk.ProfileBaseline("none")
			Expect(bl.MaxPositionSize).To(BeNil())
			Expect(bl.DrawdownCircuitBreaker).To(BeNil())
		})

		It("returns zero config for empty string", func() {
			bl := risk.ProfileBaseline("")
			Expect(bl.MaxPositionSize).To(BeNil())
			Expect(bl.DrawdownCircuitBreaker).To(BeNil())
		})

		// Cross-reference: verify conservative MaxPositionSize=0.20 matches risk.Conservative.
		It("conservative MaxPositionSize matches risk.Conservative (0.20)", func() {
			bl := risk.ProfileBaseline("conservative")
			Expect(*bl.MaxPositionSize).To(Equal(0.20),
				"conservative MaxPositionSize must stay in sync with risk.Conservative")
		})

		It("conservative DrawdownCircuitBreaker matches risk.Conservative (0.10)", func() {
			bl := risk.ProfileBaseline("conservative")
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.10),
				"conservative DrawdownCircuitBreaker must stay in sync with risk.Conservative")
		})

		It("conservative VolatilityScalerLookback matches risk.Conservative (60)", func() {
			bl := risk.ProfileBaseline("conservative")
			Expect(*bl.VolatilityScalerLookback).To(Equal(60),
				"conservative VolatilityScalerLookback must stay in sync with risk.Conservative")
		})
	})

	Describe("Resolve", func() {
		It("returns baseline when no overrides are set", func() {
			rc := risk.RiskConfig{Profile: "conservative"}
			resolved := rc.Resolve()
			Expect(*resolved.MaxPositionSize).To(Equal(0.20))
			Expect(*resolved.DrawdownCircuitBreaker).To(Equal(0.10))
			Expect(*resolved.VolatilityScalerLookback).To(Equal(60))
		})

		It("overrides MaxPositionSize from baseline", func() {
			rc := risk.RiskConfig{
				Profile:         "conservative",
				MaxPositionSize: fp(0.15),
			}
			resolved := rc.Resolve()
			Expect(*resolved.MaxPositionSize).To(Equal(0.15))
			// Other baseline fields are unaffected
			Expect(*resolved.DrawdownCircuitBreaker).To(Equal(0.10))
		})

		It("overrides DrawdownCircuitBreaker from baseline", func() {
			rc := risk.RiskConfig{
				Profile:                "moderate",
				DrawdownCircuitBreaker: fp(0.20),
			}
			resolved := rc.Resolve()
			Expect(*resolved.DrawdownCircuitBreaker).To(Equal(0.20))
			Expect(*resolved.MaxPositionSize).To(Equal(0.25))
		})

		It("adds new fields not in baseline", func() {
			rc := risk.RiskConfig{
				Profile:            "moderate",
				MaxPositionCount:   ip(10),
				GrossExposureLimit: fp(1.5),
			}
			resolved := rc.Resolve()
			Expect(resolved.MaxPositionCount).NotTo(BeNil())
			Expect(*resolved.MaxPositionCount).To(Equal(10))
			Expect(resolved.GrossExposureLimit).NotTo(BeNil())
			Expect(*resolved.GrossExposureLimit).To(Equal(1.5))
		})

		It("resolves empty profile with overrides to just overrides", func() {
			rc := risk.RiskConfig{
				MaxPositionSize: fp(0.10),
			}
			resolved := rc.Resolve()
			Expect(*resolved.MaxPositionSize).To(Equal(0.10))
			Expect(resolved.DrawdownCircuitBreaker).To(BeNil())
		})
	})
})
