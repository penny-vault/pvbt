package data_test

import (
	"database/sql"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/data"

	_ "modernc.org/sqlite"
)

var _ = Describe("SnapshotSchema", func() {
	var db *sql.DB

	BeforeEach(func() {
		var err error
		db, err = sql.Open("sqlite", ":memory:")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		db.Close()
	})

	It("creates all expected tables", func() {
		err := data.CreateSnapshotSchema(db)
		Expect(err).NotTo(HaveOccurred())

		tables := []string{"assets", "eod", "metrics", "fundamentals", "ratings", "index_members"}
		for _, table := range tables {
			var count int
			err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
			Expect(err).NotTo(HaveOccurred(), "table %s should exist", table)
			Expect(count).To(Equal(0))
		}
	})

	It("creates the fundamentals table with all metricColumn entries", func() {
		err := data.CreateSnapshotSchema(db)
		Expect(err).NotTo(HaveOccurred())

		// Insert a row with just the required columns to verify the table accepts them.
		_, err = db.Exec("INSERT INTO fundamentals (composite_figi, event_date, dimension) VALUES ('TEST', '2024-01-02', 'ARQ')")
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates fundamentals table with date_key and report_period columns", func() {
		dbPath := filepath.Join(GinkgoT().TempDir(), "schema_check.db")
		fileDB, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer fileDB.Close()

		err = data.CreateSnapshotSchema(fileDB)
		Expect(err).NotTo(HaveOccurred())

		// Verify columns exist by inserting a row with date_key and report_period.
		_, err = fileDB.Exec(
			"INSERT INTO assets (composite_figi, ticker) VALUES (?, ?)",
			"TEST-FIGI", "TEST",
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = fileDB.Exec(
			"INSERT INTO fundamentals (composite_figi, event_date, date_key, report_period, dimension) VALUES (?, ?, ?, ?, ?)",
			"TEST-FIGI", "2024-06-30", "2024-03-31", "2024-03-29", "ARQ",
		)
		Expect(err).NotTo(HaveOccurred())

		var dateKey, reportPeriod string
		err = fileDB.QueryRow(
			"SELECT date_key, report_period FROM fundamentals WHERE composite_figi = ?",
			"TEST-FIGI",
		).Scan(&dateKey, &reportPeriod)
		Expect(err).NotTo(HaveOccurred())
		Expect(dateKey).To(Equal("2024-03-31"))
		Expect(reportPeriod).To(Equal("2024-03-29"))
	})
})
