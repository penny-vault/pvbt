# Asset Metadata Extension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `asset.Asset` with metadata fields (name, asset type, exchange, sector, industry, SIC code, CIK, listed/delisted dates) so strategies can filter by sector, asset type, exchange, etc.

**Architecture:** Add four string-typed enums (`AssetType`, `Sector`, `Industry`, `Exchange`) and new scalar fields to the `Asset` struct. Update all database read paths (PVDataProvider, SnapshotProvider) to SELECT and scan the new columns. Update the snapshot schema and SnapshotRecorder to persist the new fields. Index state and changelog construction sites keep zero-value metadata.

**Tech Stack:** Go, PostgreSQL (pgx), SQLite (modernc.org/sqlite), Ginkgo/Gomega

---

### Task 1: Add enum types and extend Asset struct

**Files:**
- Modify: `asset/asset.go:16-34`

- [ ] **Step 1: Add the AssetType enum and constants**

Add above the `Asset` struct definition, after the `package asset` line and before the struct:

```go
import "time"

// AssetType classifies the instrument type.
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
```

- [ ] **Step 2: Add the Exchange enum, constants, and NormalizeExchange**

```go
// Exchange identifies the primary exchange an asset trades on.
type Exchange string

const (
	ExchangeNYSE   Exchange = "NYSE"
	ExchangeNASDAQ Exchange = "NASDAQ"
	ExchangeBATS   Exchange = "BATS"
	ExchangeFRED   Exchange = "FRED"
)

// NormalizeExchange maps raw exchange codes from the database to the
// coarse Exchange enum. Unknown values return the raw string as-is.
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
```

- [ ] **Step 3: Add the Sector enum and constants**

```go
// Sector classifies the asset's business sector.
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
```

- [ ] **Step 4: Add the Industry enum and constants**

```go
// Industry classifies the asset's specific industry within its sector.
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
```

- [ ] **Step 5: Extend the Asset struct**

Replace the existing `Asset` struct with:

```go
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
```

- [ ] **Step 6: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./asset/...`
Expected: success (sentinel values still compile -- zero values for new fields are fine)

- [ ] **Step 7: Commit**

```bash
git add asset/asset.go
git commit -m "feat: add metadata fields and enum types to Asset struct"
```

---

### Task 2: Test NormalizeExchange

**Files:**
- Create: `asset/asset_suite_test.go`
- Create: `asset/asset_test.go`

- [ ] **Step 1: Create the Ginkgo test suite bootstrap**

```go
package asset_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAsset(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Asset Suite")
}
```

- [ ] **Step 2: Write the NormalizeExchange tests**

```go
package asset_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
)

var _ = Describe("NormalizeExchange", func() {
	DescribeTable("maps raw exchange codes to normalized values",
		func(raw string, expected asset.Exchange) {
			Expect(asset.NormalizeExchange(raw)).To(Equal(expected))
		},
		Entry("NYSE", "NYSE", asset.ExchangeNYSE),
		Entry("XNYS", "XNYS", asset.ExchangeNYSE),
		Entry("NYSE ARCA", "NYSE ARCA", asset.ExchangeNYSE),
		Entry("NYSE MKT", "NYSE MKT", asset.ExchangeNYSE),
		Entry("ARCX", "ARCX", asset.ExchangeNYSE),
		Entry("XASE", "XASE", asset.ExchangeNYSE),
		Entry("AMEX", "AMEX", asset.ExchangeNYSE),
		Entry("NASDAQ", "NASDAQ", asset.ExchangeNASDAQ),
		Entry("XNAS", "XNAS", asset.ExchangeNASDAQ),
		Entry("NMFQS", "NMFQS", asset.ExchangeNASDAQ),
		Entry("BATS", "BATS", asset.ExchangeBATS),
		Entry("FRED", "FRED", asset.ExchangeFRED),
		Entry("unknown passes through", "MYSTERY", asset.Exchange("MYSTERY")),
		Entry("empty string passes through", "", asset.Exchange("")),
	)
})
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./asset/...`
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add asset/asset_suite_test.go asset/asset_test.go
git commit -m "test: add NormalizeExchange tests"
```

---

### Task 3: Update snapshot schema

**Files:**
- Modify: `data/snapshot_schema.go:14-18`

- [ ] **Step 1: Expand the assets CREATE TABLE**

Replace the `assets` table DDL (lines 15-18 of `data/snapshot_schema.go`) with:

```go
		`CREATE TABLE IF NOT EXISTS assets (
			composite_figi TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			asset_type TEXT NOT NULL DEFAULT '',
			primary_exchange TEXT NOT NULL DEFAULT '',
			sector TEXT NOT NULL DEFAULT '',
			industry TEXT NOT NULL DEFAULT '',
			sic_code INTEGER NOT NULL DEFAULT 0,
			cik TEXT NOT NULL DEFAULT '',
			listed TEXT NOT NULL DEFAULT '',
			delisted TEXT NOT NULL DEFAULT ''
		)`,
```

- [ ] **Step 2: Verify compilation and existing tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./data/... && ginkgo run ./data/...`
Expected: all pass. Existing tests insert only `composite_figi, ticker` into assets -- the DEFAULT values handle the missing columns.

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_schema.go
git commit -m "feat: expand snapshot assets table with metadata columns"
```

---

### Task 4: Update SnapshotRecorder write path

**Files:**
- Modify: `data/snapshot_recorder.go:172-197`

- [ ] **Step 1: Expand recordAssets INSERT statement**

Replace the `recordAssets` method body. The prepared statement changes from 2 columns to 11. Time fields are stored as `2006-01-02` text (matching the existing date format in other tables), with zero times stored as empty string. The `PrimaryExchange` is stored as-is (already normalized by the time it reaches the Asset struct):

```go
func (r *SnapshotRecorder) recordAssets(assets []asset.Asset) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO assets
		(composite_figi, ticker, name, asset_type, primary_exchange, sector, industry, sic_code, cik, listed, delisted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, aa := range assets {
		listedStr := ""
		if !aa.Listed.IsZero() {
			listedStr = aa.Listed.Format("2006-01-02")
		}

		delistedStr := ""
		if !aa.Delisted.IsZero() {
			delistedStr = aa.Delisted.Format("2006-01-02")
		}

		if _, err := stmt.Exec(
			aa.CompositeFigi, aa.Ticker, aa.Name,
			string(aa.AssetType), string(aa.PrimaryExchange),
			string(aa.Sector), string(aa.Industry),
			aa.SICCode, aa.CIK,
			listedStr, delistedStr,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./data/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_recorder.go
git commit -m "feat: write all asset metadata fields in SnapshotRecorder"
```

---

### Task 5: Update SnapshotRecorder tests to verify metadata round-trip

**Files:**
- Modify: `data/snapshot_recorder_test.go:35-91`

- [ ] **Step 1: Update the Assets() recording test to include metadata**

Replace the "records assets from Assets() call" test (lines 36-64) with a version that constructs Assets with full metadata and verifies the metadata was written to SQLite:

```go
		It("records assets from Assets() call", func() {
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			stubAssets := []asset.Asset{
				{
					CompositeFigi:   "BBG000BLNNH6",
					Ticker:          "SPY",
					Name:            "SPDR S&P 500 ETF Trust",
					AssetType:       asset.AssetTypeETF,
					PrimaryExchange: asset.ExchangeNYSE,
					Sector:          "",
					Industry:        "",
					SICCode:         6726,
					CIK:             "0000884394",
					Listed:          time.Date(1993, 1, 22, 0, 0, 0, 0, nyc),
				},
				{
					CompositeFigi:   "BBG000BHTK15",
					Ticker:          "TLT",
					Name:            "iShares 20+ Year Treasury Bond ETF",
					AssetType:       asset.AssetTypeETF,
					PrimaryExchange: asset.ExchangeNASDAQ,
					Sector:          "",
					Industry:        "",
					SICCode:         0,
					CIK:             "0000088525",
					Listed:          time.Date(2002, 7, 22, 0, 0, 0, 0, nyc),
				},
			}
			stub := &stubAssetProvider{assets: stubAssets}

			var recErr error
			recorder, recErr = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(recErr).NotTo(HaveOccurred())

			result, err := recorder.Assets(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(stubAssets))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			// Verify metadata was written to SQLite.
			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(2))

			var name, assetType, exchange, cik string
			var sicCode int
			Expect(db.QueryRow(
				"SELECT name, asset_type, primary_exchange, sic_code, cik FROM assets WHERE ticker = 'SPY'",
			).Scan(&name, &assetType, &exchange, &sicCode, &cik)).To(Succeed())
			Expect(name).To(Equal("SPDR S&P 500 ETF Trust"))
			Expect(assetType).To(Equal("ETF"))
			Expect(exchange).To(Equal("NYSE"))
			Expect(sicCode).To(Equal(6726))
			Expect(cik).To(Equal("0000884394"))
		})
```

- [ ] **Step 2: Update the LookupAsset() recording test to include metadata**

Replace the "records asset from LookupAsset() call" test (lines 66-90) with:

```go
		It("records asset from LookupAsset() call", func() {
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			expected := asset.Asset{
				CompositeFigi:   "BBG000BLNNH6",
				Ticker:          "SPY",
				Name:            "SPDR S&P 500 ETF Trust",
				AssetType:       asset.AssetTypeETF,
				PrimaryExchange: asset.ExchangeNYSE,
				SICCode:         6726,
				CIK:             "0000884394",
				Listed:          time.Date(1993, 1, 22, 0, 0, 0, 0, nyc),
			}
			stub := &stubAssetProvider{lookupResult: expected}

			var recErr error
			recorder, recErr = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(recErr).NotTo(HaveOccurred())

			result, err := recorder.LookupAsset(ctx, "SPY")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expected))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		})
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./data/...`
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add data/snapshot_recorder_test.go
git commit -m "test: verify asset metadata round-trips through SnapshotRecorder"
```

---

### Task 6: Update SnapshotProvider read paths

**Files:**
- Modify: `data/snapshot_provider.go:127-159` (Assets, LookupAsset)
- Modify: `data/snapshot_provider.go:586-652` (IndexMembers, RatedAssets)

- [ ] **Step 1: Add a scanAsset helper to SnapshotProvider**

Add a file-local helper that scans all 11 asset columns from a `*sql.Rows` or `*sql.Row`. This avoids repeating the scan logic in every method. Place it right after the `SnapshotProvider` struct definition (around line 40). The helper scans from a row that selects: `composite_figi, ticker, name, asset_type, primary_exchange, sector, industry, sic_code, cik, listed, delisted`.

```go
// scanAssetRow scans an asset from a row that selects the full assets column set.
func scanAssetRow(scanner interface{ Scan(dest ...any) error }) (asset.Asset, error) {
	var (
		aa                                                          asset.Asset
		name, assetType, exchange, sector, industry, cik            string
		sicCode                                                     int
		listedStr, delistedStr                                      string
	)

	if err := scanner.Scan(
		&aa.CompositeFigi, &aa.Ticker,
		&name, &assetType, &exchange, &sector, &industry,
		&sicCode, &cik, &listedStr, &delistedStr,
	); err != nil {
		return asset.Asset{}, err
	}

	aa.Name = name
	aa.AssetType = asset.AssetType(assetType)
	aa.PrimaryExchange = asset.Exchange(exchange)
	aa.Sector = asset.Sector(sector)
	aa.Industry = asset.Industry(industry)
	aa.SICCode = sicCode
	aa.CIK = cik

	if listedStr != "" {
		if tt, err := time.Parse("2006-01-02", listedStr); err == nil {
			aa.Listed = tt
		}
	}

	if delistedStr != "" {
		if tt, err := time.Parse("2006-01-02", delistedStr); err == nil {
			aa.Delisted = tt
		}
	}

	return aa, nil
}
```

- [ ] **Step 2: Update Assets()**

Replace the Assets method (lines 127-146) to use the expanded SELECT and scanAssetRow:

```go
func (p *SnapshotProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT composite_figi, ticker, name, asset_type, primary_exchange,
		        sector, industry, sic_code, cik, listed, delisted
		 FROM assets ORDER BY ticker`)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		aa, scanErr := scanAssetRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("snapshot provider: scan asset: %w", scanErr)
		}

		assets = append(assets, aa)
	}

	return assets, rows.Err()
}
```

- [ ] **Step 3: Update LookupAsset()**

Replace the LookupAsset method (lines 148-159):

```go
func (p *SnapshotProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT composite_figi, ticker, name, asset_type, primary_exchange,
		        sector, industry, sic_code, cik, listed, delisted
		 FROM assets WHERE ticker = ? LIMIT 1`, ticker)

	aa, err := scanAssetRow(row)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("snapshot provider: lookup asset %q: %w", ticker, err)
	}

	return aa, nil
}
```

- [ ] **Step 4: Update IndexMembers() to JOIN assets**

Replace the IndexMembers method (lines 586-619). The query JOINs `index_members` to `assets` so each returned Asset has full metadata:

```go
func (p *SnapshotProvider) IndexMembers(ctx context.Context, index string, forDate time.Time) ([]asset.Asset, []IndexConstituent, error) {
	dateStr := forDate.Format("2006-01-02")

	rows, err := p.db.QueryContext(ctx,
		`SELECT a.composite_figi, a.ticker, a.name, a.asset_type, a.primary_exchange,
		        a.sector, a.industry, a.sic_code, a.cik, a.listed, a.delisted,
		        COALESCE(im.weight, 0)
		 FROM index_members im
		 JOIN assets a ON a.composite_figi = im.composite_figi
		 WHERE im.index_name = ? AND im.event_date = ?`,
		index, dateStr,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot provider: query index members: %w", err)
	}
	defer rows.Close()

	var (
		assets       []asset.Asset
		constituents []IndexConstituent
	)

	for rows.Next() {
		var (
			name, assetType, exchange, sector, industry, cik string
			sicCode                                          int
			listedStr, delistedStr                           string
			figi, ticker                                     string
			weight                                           float64
		)

		if err := rows.Scan(
			&figi, &ticker, &name, &assetType, &exchange,
			&sector, &industry, &sicCode, &cik, &listedStr, &delistedStr,
			&weight,
		); err != nil {
			return nil, nil, fmt.Errorf("snapshot provider: scan index member: %w", err)
		}

		assetVal := asset.Asset{
			CompositeFigi:   figi,
			Ticker:          ticker,
			Name:            name,
			AssetType:       asset.AssetType(assetType),
			PrimaryExchange: asset.Exchange(exchange),
			Sector:          asset.Sector(sector),
			Industry:        asset.Industry(industry),
			SICCode:         sicCode,
			CIK:             cik,
		}

		if listedStr != "" {
			if tt, parseErr := time.Parse("2006-01-02", listedStr); parseErr == nil {
				assetVal.Listed = tt
			}
		}

		if delistedStr != "" {
			if tt, parseErr := time.Parse("2006-01-02", delistedStr); parseErr == nil {
				assetVal.Delisted = tt
			}
		}

		assets = append(assets, assetVal)
		constituents = append(constituents, IndexConstituent{Asset: assetVal, Weight: weight})
	}

	return assets, constituents, rows.Err()
}
```

- [ ] **Step 5: Update RatedAssets() to JOIN assets**

Replace the RatedAssets method (lines 623-652):

```go
func (p *SnapshotProvider) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, forDate time.Time) ([]asset.Asset, error) {
	filterJSON, err := sonic.Marshal(filter.Values)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: marshal filter: %w", err)
	}

	dateStr := forDate.Format("2006-01-02")

	rows, err := p.db.QueryContext(ctx,
		`SELECT a.composite_figi, a.ticker, a.name, a.asset_type, a.primary_exchange,
		        a.sector, a.industry, a.sic_code, a.cik, a.listed, a.delisted
		 FROM ratings r
		 JOIN assets a ON a.composite_figi = r.composite_figi
		 WHERE r.analyst = ? AND r.filter_values = ? AND r.event_date = ?`,
		analyst, string(filterJSON), dateStr,
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query ratings: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		aa, scanErr := scanAssetRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("snapshot provider: scan rated asset: %w", scanErr)
		}

		assets = append(assets, aa)
	}

	return assets, rows.Err()
}
```

- [ ] **Step 6: Verify compilation and tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./data/... && ginkgo run ./data/...`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add data/snapshot_provider.go
git commit -m "feat: read all asset metadata fields in SnapshotProvider"
```

---

### Task 7: Update SnapshotProvider tests to verify metadata read-back

**Files:**
- Modify: `data/snapshot_provider_test.go:29-77`

- [ ] **Step 1: Update seedDB to insert metadata**

Replace the seedDB helper (lines 29-37) to insert all asset columns:

```go
	seedDB := func() {
		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.CreateSnapshotSchema(db)).To(Succeed())

		_, err = db.Exec(`INSERT INTO assets
			(composite_figi, ticker, name, asset_type, primary_exchange, sector, industry, sic_code, cik, listed, delisted)
			VALUES
			('BBG000BLNNH6', 'SPY', 'SPDR S&P 500 ETF Trust', 'ETF', 'NYSE', '', '', 6726, '0000884394', '1993-01-22', ''),
			('BBG000BHTK15', 'TLT', 'iShares 20+ Year Treasury Bond ETF', 'ETF', 'NASDAQ', '', '', 0, '0000088525', '2002-07-22', '')`)
		Expect(err).NotTo(HaveOccurred())
		db.Close()
	}
```

- [ ] **Step 2: Update Assets test to verify metadata**

Replace the Assets test (lines 39-51):

```go
	Describe("Assets", func() {
		It("returns all assets with metadata from the snapshot", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			assets, err := snap.Assets(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(assets).To(HaveLen(2))
			Expect(assets[0].Ticker).To(Equal("SPY"))
			Expect(assets[0].Name).To(Equal("SPDR S&P 500 ETF Trust"))
			Expect(assets[0].AssetType).To(Equal(asset.AssetTypeETF))
			Expect(assets[0].PrimaryExchange).To(Equal(asset.ExchangeNYSE))
			Expect(assets[0].SICCode).To(Equal(6726))
			Expect(assets[0].CIK).To(Equal("0000884394"))
			Expect(assets[0].Listed.Year()).To(Equal(1993))
		})
	})
```

- [ ] **Step 3: Update LookupAsset test to verify metadata**

Replace the first LookupAsset test case (lines 55-65):

```go
		It("finds an asset by ticker with full metadata", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.LookupAsset(ctx, "SPY")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.CompositeFigi).To(Equal("BBG000BLNNH6"))
			Expect(result.Name).To(Equal("SPDR S&P 500 ETF Trust"))
			Expect(result.AssetType).To(Equal(asset.AssetTypeETF))
			Expect(result.PrimaryExchange).To(Equal(asset.ExchangeNYSE))
		})
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./data/...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add data/snapshot_provider_test.go
git commit -m "test: verify asset metadata read-back in SnapshotProvider"
```

---

### Task 8: Update PVDataProvider read paths

**Files:**
- Modify: `data/pvdata_provider.go:139-181` (LookupAsset, Assets)
- Modify: `data/pvdata_provider.go:653-687` (RatedAssets)

The PVDataProvider uses pgx which scans with `pgx.Rows` -- nullable columns need `*string`, `*int`, `*time.Time` pointers (pgx handles NULL as nil pointer).

- [ ] **Step 1: Add a scanAssetFromPgx helper**

Add a helper in `data/pvdata_provider.go` near the top (after imports). This avoids repeating the nullable scan + conversion logic:

```go
// scanPgxAsset scans a full asset row from a pgx result set. All metadata
// columns are nullable in the view, so we scan into pointers and fall back
// to zero values.
func scanPgxAsset(scanner interface{ Scan(dest ...any) error }) (asset.Asset, error) {
	var (
		aa                                     asset.Asset
		name, assetType, exchange              *string
		sector, industry, cik                  *string
		sicCode                                *int
		listed, delisted                       *time.Time
	)

	if err := scanner.Scan(
		&aa.CompositeFigi, &aa.Ticker,
		&name, &assetType, &exchange,
		&sector, &industry, &sicCode, &cik,
		&listed, &delisted,
	); err != nil {
		return asset.Asset{}, err
	}

	if name != nil {
		aa.Name = *name
	}

	if assetType != nil {
		aa.AssetType = asset.AssetType(*assetType)
	}

	if exchange != nil {
		aa.PrimaryExchange = asset.NormalizeExchange(*exchange)
	}

	if sector != nil {
		aa.Sector = asset.Sector(*sector)
	}

	if industry != nil {
		aa.Industry = asset.Industry(*industry)
	}

	if sicCode != nil {
		aa.SICCode = *sicCode
	}

	if cik != nil {
		aa.CIK = *cik
	}

	if listed != nil {
		aa.Listed = *listed
	}

	if delisted != nil {
		aa.Delisted = *delisted
	}

	return aa, nil
}
```

- [ ] **Step 2: Update LookupAsset()**

Replace the LookupAsset method (lines 140-158). Note: the original query had `SELECT ticker, composite_figi` but the `scanPgxAsset` helper scans `composite_figi` first, so the new query uses `composite_figi, ticker` order:

```go
func (p *PVDataProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	row := conn.QueryRow(ctx,
		`SELECT composite_figi, ticker, name, asset_type, primary_exchange,
		        sector, industry, sic_code, cik, listed, delisted
		 FROM assets
		 WHERE ticker = $1 AND active = true LIMIT 1`,
		ticker,
	)

	foundAsset, scanErr := scanPgxAsset(row)
	if scanErr != nil {
		return asset.Asset{}, fmt.Errorf("pvdata: lookup asset %q: %w", ticker, scanErr)
	}

	return foundAsset, nil
}
```

- [ ] **Step 3: Update Assets()**

Replace the Assets method (lines 160-181):

```go
func (p *PVDataProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT composite_figi, ticker, name, asset_type, primary_exchange,
		        sector, industry, sic_code, cik, listed, delisted
		 FROM assets ORDER BY ticker`)
	if err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		aa, scanErr := scanPgxAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan asset: %w", scanErr)
		}

		assets = append(assets, aa)
	}

	return assets, rows.Err()
}
```

- [ ] **Step 4: Update RatedAssets()**

The `RatedAssets` method (lines 653-687) queries the `ratings` table which doesn't have asset metadata. Add a JOIN to the `assets` view:

```go
func (p *PVDataProvider) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, asOfDate time.Time) ([]asset.Asset, error) {
	if len(filter.Values) == 0 {
		return nil, nil
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx,
		`SELECT a.composite_figi, a.ticker, a.name, a.asset_type, a.primary_exchange,
		        a.sector, a.industry, a.sic_code, a.cik, a.listed, a.delisted
		 FROM ratings r
		 JOIN assets a ON a.composite_figi = r.composite_figi
		 WHERE r.analyst = $1 AND r.event_date = $2 AND r.rating = ANY($3)`,
		analyst, asOfDate, filter.Values,
	)
	if err != nil {
		return nil, fmt.Errorf("pvdata: query rated assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		aa, scanErr := scanPgxAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("pvdata: scan rated asset: %w", scanErr)
		}

		assets = append(assets, aa)
	}

	return assets, rows.Err()
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./data/...`
Expected: success

- [ ] **Step 6: Commit**

```bash
git add data/pvdata_provider.go
git commit -m "feat: read all asset metadata fields in PVDataProvider"
```

---

### Task 9: Update documentation and changelog

**Files:**
- Modify: `asset/doc.go:16-39`
- Modify: `docs/data.md` (Asset providers section)
- Modify: `CHANGELOG.md:8-9`

- [ ] **Step 1: Update the asset package doc comment**

Replace the doc comment in `asset/doc.go` (lines 16-39) to describe the new fields and enum types:

```go
// Package asset defines the [Asset] type representing a tradeable instrument
// identified by a CompositeFigi and a human-readable Ticker.
//
// # Asset Type
//
// The Asset struct carries identity fields (CompositeFigi, Ticker) and
// metadata loaded from the data provider: Name, [AssetType], [Exchange],
// [Sector], [Industry], SICCode, CIK, and listing dates (Listed, Delisted).
// Strategies use the metadata to filter assets -- for example, excluding
// financial-sector stocks or limiting to common stock only.
//
// The AssetType, Exchange, Sector, and Industry fields are string-typed
// enums with named constants (e.g. [AssetTypeETF], [ExchangeNYSE],
// [SectorTechnology], [IndustryBiotechnology]). Raw exchange codes from
// the database are normalized via [NormalizeExchange].
//
// # Economic Indicators
//
// The [EconomicIndicator] sentinel value represents data that is not tied to a
// specific asset, such as unemployment rates or CPI. It is used in data
// requests and DataFrames to keep the layout uniform -- from the DataFrame's
// perspective, an economic indicator looks like any other asset.
//
// # Ticker Resolution
//
// Tickers can include a namespace prefix to specify the data source (e.g.,
// "FRED:DGS3MO"). The engine's Asset method resolves tickers to full Asset
// values using the registered AssetProvider.
package asset
```

- [ ] **Step 2: Update the data.md Asset providers section**

In `docs/data.md`, find the `AssetProvider` interface section (around line 113-130) and update the prose to mention that `Assets()` and `LookupAsset()` now return fully populated metadata:

After the existing interface code block, add a paragraph:

```markdown
`Assets` and `LookupAsset` return `asset.Asset` values with full metadata: name, asset type, primary exchange, sector, industry, SIC code, CIK, and listing dates. Strategies can use these fields directly for filtering -- for example, `a.Sector == asset.SectorFinancialServices` or `a.AssetType == asset.AssetTypeCommonStock`.
```

- [ ] **Step 3: Add a changelog entry**

Add to the `### Added` section under `## [Unreleased]` in `CHANGELOG.md`:

```markdown
- The `asset.Asset` type carries metadata from the data provider: name, asset type, exchange, sector, industry, SIC code, CIK, and listing dates. Strategies can filter by these fields directly (e.g. exclude financial-sector stocks or limit to common stock).
```

- [ ] **Step 4: Commit**

```bash
git add asset/doc.go docs/data.md CHANGELOG.md
git commit -m "docs: document asset metadata fields and update changelog"
```

---

### Task 10: Run full test suite and lint

**Files:** none (verification only)

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`
Expected: all pass

- [ ] **Step 2: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make lint`
Expected: no errors

- [ ] **Step 3: Fix any lint or test issues discovered**

Address any issues and commit fixes.
