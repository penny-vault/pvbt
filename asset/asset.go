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

package asset

import "time"

// AssetType identifies the class of a financial instrument.
type AssetType string

const (
	AssetTypeCommonStock AssetType = "CS"
	AssetTypeETF         AssetType = "ETF"
	AssetTypeETN         AssetType = "ETN"
	AssetTypeMutualFund  AssetType = "MF"
	AssetTypeCEF         AssetType = "CEF"
	AssetTypeADR         AssetType = "ADRC"
	AssetTypeFRED        AssetType = "FRED"
	AssetTypeSynthetic   AssetType = "SYNTH"
)

// Exchange identifies the primary listing exchange for an asset.
type Exchange string

const (
	ExchangeNYSE   Exchange = "NYSE"
	ExchangeNASDAQ Exchange = "NASDAQ"
	ExchangeBATS   Exchange = "BATS"
	ExchangeFRED   Exchange = "FRED"
)

// NormalizeExchange maps raw exchange strings (e.g. MIC codes or alternative
// names) to a canonical Exchange value.
func NormalizeExchange(raw string) Exchange {
	switch raw {
	case "NYSE", "XNYS", "NYSE ARCA", "NYSE MKT", "ARCX", "XASE", "AMEX":
		return ExchangeNYSE
	case "NASDAQ", "XNAS", "NMFQS":
		return ExchangeNASDAQ
	case "BATS":
		return ExchangeBATS
	case "FRED":
		return ExchangeFRED
	default:
		return Exchange(raw)
	}
}

// Sector identifies the GICS-style sector classification of an asset.
type Sector string

const (
	SectorBasicMaterials        Sector = "Basic Materials"
	SectorCommunicationServices Sector = "Communication Services"
	SectorConsumerCyclical      Sector = "Consumer Cyclical"
	SectorConsumerDefensive     Sector = "Consumer Defensive"
	SectorEnergy                Sector = "Energy"
	SectorFinancialServices     Sector = "Financial Services"
	SectorHealthcare            Sector = "Healthcare"
	SectorIndustrials           Sector = "Industrials"
	SectorRealEstate            Sector = "Real Estate"
	SectorTechnology            Sector = "Technology"
	SectorUtilities             Sector = "Utilities"
)

// Industry identifies the industry classification of an asset.
type Industry string

const (
	IndustryAdvertisingAgencies                Industry = "Advertising Agencies"
	IndustryAerospaceDefense                   Industry = "Aerospace & Defense"
	IndustryAgriculturalInputs                 Industry = "Agricultural Inputs"
	IndustryAirlines                           Industry = "Airlines"
	IndustryAirportsAirServices                Industry = "Airports & Air Services"
	IndustryAluminum                           Industry = "Aluminum"
	IndustryApparelManufacturing               Industry = "Apparel Manufacturing"
	IndustryApparelRetail                      Industry = "Apparel Retail"
	IndustryApplicationSoftware                Industry = "Application Software"
	IndustryAssetManagement                    Industry = "Asset Management"
	IndustryAutoTruckDealerships               Industry = "Auto & Truck Dealerships"
	IndustryAutoManufacturers                  Industry = "Auto Manufacturers"
	IndustryAutoParts                          Industry = "Auto Parts"
	IndustryBanksDiversified                   Industry = "Banks\u2014Diversified"
	IndustryBanksRegional                      Industry = "Banks\u2014Regional"
	IndustryBeveragesBrewers                   Industry = "Beverages\u2014Brewers"
	IndustryBeveragesNonAlcoholic              Industry = "Beverages\u2014Non-Alcoholic"
	IndustryBeveragesWineriesDistilleries      Industry = "Beverages\u2014Wineries & Distilleries"
	IndustryBiotechnology                      Industry = "Biotechnology"
	IndustryBroadcasting                       Industry = "Broadcasting"
	IndustryBuildingMaterials                  Industry = "Building Materials"
	IndustryBuildingProductsEquipment          Industry = "Building Products & Equipment"
	IndustryBusinessEquipmentSupplies          Industry = "Business Equipment & Supplies"
	IndustryCapitalMarkets                     Industry = "Capital Markets"
	IndustryChemicals                          Industry = "Chemicals"
	IndustryCokingCoal                         Industry = "Coking Coal"
	IndustryCommunicationEquipment             Industry = "Communication Equipment"
	IndustryComputerHardware                   Industry = "Computer Hardware"
	IndustryConfectioners                      Industry = "Confectioners"
	IndustryConglomerates                      Industry = "Conglomerates"
	IndustryConsultingServices                 Industry = "Consulting Services"
	IndustryConsumerElectronics                Industry = "Consumer Electronics"
	IndustryCopper                             Industry = "Copper"
	IndustryCreditServices                     Industry = "Credit Services"
	IndustryDepartmentStores                   Industry = "Department Stores"
	IndustryDiagnosticsResearch                Industry = "Diagnostics & Research"
	IndustryDiscountStores                     Industry = "Discount Stores"
	IndustryDrugManufacturersGeneral           Industry = "Drug Manufacturers\u2014General"
	IndustryDrugManufacturersSpecialtyGeneric  Industry = "Drug Manufacturers\u2014Specialty & Generic"
	IndustryEducationTrainingServices          Industry = "Education & Training Services"
	IndustryElectricalEquipmentParts           Industry = "Electrical Equipment & Parts"
	IndustryElectronicComponents               Industry = "Electronic Components"
	IndustryElectronicGamingMultimedia         Industry = "Electronic Gaming & Multimedia"
	IndustryElectronicsComputerDistribution    Industry = "Electronics & Computer Distribution"
	IndustryEngineeringConstruction            Industry = "Engineering & Construction"
	IndustryEntertainment                      Industry = "Entertainment"
	IndustryFarmHeavyConstructionMachinery     Industry = "Farm & Heavy Construction Machinery"
	IndustryFarmProducts                       Industry = "Farm Products"
	IndustryFinancialConglomerates             Industry = "Financial Conglomerates"
	IndustryFinancialDataStockExchanges        Industry = "Financial Data & Stock Exchanges"
	IndustryFoodDistribution                   Industry = "Food Distribution"
	IndustryFootwearAccessories                Industry = "Footwear & Accessories"
	IndustryFurnishingsFixturesAppliances      Industry = "Furnishings, Fixtures & Appliances"
	IndustryGambling                           Industry = "Gambling"
	IndustryGold                               Industry = "Gold"
	IndustryGroceryStores                      Industry = "Grocery Stores"
	IndustryHealthInformationServices          Industry = "Health Information Services"
	IndustryHealthcarePlans                    Industry = "Healthcare Plans"
	IndustryHomeImprovementRetail              Industry = "Home Improvement Retail"
	IndustryHouseholdPersonalProducts          Industry = "Household & Personal Products"
	IndustryIndustrialDistribution             Industry = "Industrial Distribution"
	IndustryInformationTechnologyServices      Industry = "Information Technology Services"
	IndustryInfrastructureOperations           Industry = "Infrastructure Operations"
	IndustryInsuranceBrokers                   Industry = "Insurance Brokers"
	IndustryInsuranceDiversified               Industry = "Insurance\u2014Diversified"
	IndustryInsuranceLife                      Industry = "Insurance\u2014Life"
	IndustryInsurancePropertyCasualty          Industry = "Insurance\u2014Property & Casualty"
	IndustryInsuranceReinsurance               Industry = "Insurance\u2014Reinsurance"
	IndustryInsuranceSpecialty                 Industry = "Insurance\u2014Specialty"
	IndustryIntegratedFreightLogistics         Industry = "Integrated Freight & Logistics"
	IndustryInternetContentInformation         Industry = "Internet Content & Information"
	IndustryInternetRetail                     Industry = "Internet Retail"
	IndustryLeisure                            Industry = "Leisure"
	IndustryLodging                            Industry = "Lodging"
	IndustryLumberWoodProduction               Industry = "Lumber & Wood Production"
	IndustryLuxuryGoods                        Industry = "Luxury Goods"
	IndustryMarineShipping                     Industry = "Marine Shipping"
	IndustryMedicalCareFacilities              Industry = "Medical Care Facilities"
	IndustryMedicalDevices                     Industry = "Medical Devices"
	IndustryMedicalDistribution                Industry = "Medical Distribution"
	IndustryMedicalInstrumentsSupplies         Industry = "Medical Instruments & Supplies"
	IndustryMetalFabrication                   Industry = "Metal Fabrication"
	IndustryMortgageFinance                    Industry = "Mortgage Finance"
	IndustryOilGasDrilling                     Industry = "Oil & Gas Drilling"
	IndustryOilGasEP                           Industry = "Oil & Gas E&P"
	IndustryOilGasEquipmentServices            Industry = "Oil & Gas Equipment & Services"
	IndustryOilGasIntegrated                   Industry = "Oil & Gas Integrated"
	IndustryOilGasMidstream                    Industry = "Oil & Gas Midstream"
	IndustryOilGasRefiningMarketing            Industry = "Oil & Gas Refining & Marketing"
	IndustryOtherIndustrialMetalsMining        Industry = "Other Industrial Metals & Mining"
	IndustryOtherPreciousMetalsMining          Industry = "Other Precious Metals & Mining"
	IndustryPackagedFoods                      Industry = "Packaged Foods"
	IndustryPackagingContainers                Industry = "Packaging & Containers"
	IndustryPaperPaperProducts                 Industry = "Paper & Paper Products"
	IndustryPersonalServices                   Industry = "Personal Services"
	IndustryPharmaceuticalRetailers            Industry = "Pharmaceutical Retailers"
	IndustryPollutionTreatmentControls         Industry = "Pollution & Treatment Controls"
	IndustryPublishing                         Industry = "Publishing"
	IndustryRailroads                          Industry = "Railroads"
	IndustryRealEstateServices                 Industry = "Real Estate Services"
	IndustryRealEstateDevelopment              Industry = "Real Estate\u2014Development"
	IndustryRealEstateDiversified              Industry = "Real Estate\u2014Diversified"
	IndustryRecreationalVehicles               Industry = "Recreational Vehicles"
	IndustryREITDiversified                    Industry = "REIT\u2014Diversified"
	IndustryREITHealthcareFacilities           Industry = "REIT\u2014Healthcare Facilities"
	IndustryREITHotelMotel                     Industry = "REIT\u2014Hotel & Motel"
	IndustryREITIndustrial                     Industry = "REIT\u2014Industrial"
	IndustryREITMortgage                       Industry = "REIT\u2014Mortgage"
	IndustryREITOffice                         Industry = "REIT\u2014Office"
	IndustryREITResidential                    Industry = "REIT\u2014Residential"
	IndustryREITRetail                         Industry = "REIT\u2014Retail"
	IndustryREITSpecialty                      Industry = "REIT\u2014Specialty"
	IndustryRentalLeasingServices              Industry = "Rental & Leasing Services"
	IndustryResidentialConstruction            Industry = "Residential Construction"
	IndustryResortsCasinos                     Industry = "Resorts & Casinos"
	IndustryRestaurants                        Industry = "Restaurants"
	IndustryScientificTechnicalInstruments     Industry = "Scientific & Technical Instruments"
	IndustrySecurityProtectionServices         Industry = "Security & Protection Services"
	IndustrySemiconductorEquipmentMaterials    Industry = "Semiconductor Equipment & Materials"
	IndustrySemiconductors                     Industry = "Semiconductors"
	IndustryShellCompanies                     Industry = "Shell Companies"
	IndustrySilver                             Industry = "Silver"
	IndustrySoftwareApplication                Industry = "Software\u2014Application"
	IndustrySoftwareInfrastructure             Industry = "Software\u2014Infrastructure"
	IndustrySolar                              Industry = "Solar"
	IndustrySpecialtyBusinessServices          Industry = "Specialty Business Services"
	IndustrySpecialtyChemicals                 Industry = "Specialty Chemicals"
	IndustrySpecialtyIndustrialMachinery       Industry = "Specialty Industrial Machinery"
	IndustrySpecialtyRetail                    Industry = "Specialty Retail"
	IndustryStaffingEmploymentServices         Industry = "Staffing & Employment Services"
	IndustrySteel                              Industry = "Steel"
	IndustryTelecomServices                    Industry = "Telecom Services"
	IndustryTextileManufacturing               Industry = "Textile Manufacturing"
	IndustryThermalCoal                        Industry = "Thermal Coal"
	IndustryTobacco                            Industry = "Tobacco"
	IndustryToolsAccessories                   Industry = "Tools & Accessories"
	IndustryTravelServices                     Industry = "Travel Services"
	IndustryTrucking                           Industry = "Trucking"
	IndustryUranium                            Industry = "Uranium"
	IndustryUtilitiesDiversified               Industry = "Utilities\u2014Diversified"
	IndustryUtilitiesIndependentPowerProducers Industry = "Utilities\u2014Independent Power Producers"
	IndustryUtilitiesRegulatedElectric         Industry = "Utilities\u2014Regulated Electric"
	IndustryUtilitiesRegulatedGas              Industry = "Utilities\u2014Regulated Gas"
	IndustryUtilitiesRegulatedWater            Industry = "Utilities\u2014Regulated Water"
	IndustryUtilitiesRenewable                 Industry = "Utilities\u2014Renewable"
	IndustryWasteManagement                    Industry = "Waste Management"
)

// Asset represents a single tradeable instrument.
type Asset struct {
	CompositeFigi   string
	Ticker          string
	Name            string
	AssetType       AssetType
	PrimaryExchange Exchange
	Sector          Sector
	Industry        Industry
	SICCode         int
	CIK             string
	Listed          time.Time
	Delisted        time.Time
}

// EconomicIndicator is a sentinel asset for metrics not tied to a specific instrument.
var EconomicIndicator = Asset{Ticker: "$ECONOMIC_INDICATOR"}

// CashAsset is a sentinel asset representing uninvested cash in a portfolio.
// Used by ChildAllocations to represent a child strategy's cash position.
var CashAsset = Asset{Ticker: "$CASH", CompositeFigi: "$CASH"}

// Factor is a sentinel asset for factor return series (e.g. Fama-French
// SMB, HML, MktRF). Factor names are represented as metrics in a DataFrame
// with this asset, following the same pattern as EconomicIndicator.
var Factor = Asset{Ticker: "$FACTOR", CompositeFigi: "$FACTOR"}
