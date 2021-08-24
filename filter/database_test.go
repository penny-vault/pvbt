package filter_test

import (
	"main/filter"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Database", func() {
	Describe("when building a select", func() {
		Context("with passed parameters", func() {
			It("should error for no 'from'", func() {
				_, _, err := filter.BuildQuery("", make([]string, 0), make([]string, 0), make(map[string]string), "")
				Expect(err).NotTo(BeNil())
			})
			It("should escape select identifiers", func() {
				fields := []string{"a\"a", "b"}
				where := map[string]string{}
				sql, _, err := filter.BuildQuery("my_table", fields, make([]string, 0), where, "event_date DESC")
				Expect(err).To(BeNil())
				Expect(sql).To(Equal(`select "a""a", "b" from "my_table" order by event_date DESC`))
			})
			It("should escape from identifier", func() {
				fields := []string{"a"}
				where := map[string]string{}
				sql, _, err := filter.BuildQuery("my_\"table", fields, make([]string, 0), where, "event_date DESC")
				Expect(err).To(BeNil())
				Expect(sql).To(Equal(`select "a" from "my_""table" order by event_date DESC`))
			})
		})
	})
})
