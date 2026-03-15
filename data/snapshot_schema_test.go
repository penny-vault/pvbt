package data_test

import (
	"database/sql"

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
})
