package servicelevels

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Units", func() {
	Context("Percentile", func() {
		It("should not parse 0", func() {
			_, failed := ParsePercentile(0)
			Expect(failed).To(HaveOccurred())
		})

		It("should not parse negative", func() {
			_, failed := ParsePercentile(-1)
			Expect(failed).To(HaveOccurred())
		})

		It("should not parse over 100", func() {
			_, failed := ParsePercentile(100.1)
			Expect(failed).To(HaveOccurred())
		})

		It("should parse valid value", func() {
			p, failed := ParsePercentile(100)
			Expect(failed).NotTo(HaveOccurred())
			Expect(p.Normalized()).To(Equal(float64(1)))
		})

		DescribeTable("name construction",
			func(value float64, name string) {
				percentile, err := ParsePercentile(value)
				Expect(err).To(BeNil())
				Expect(percentile.Name()).To(Equal(name))
			},
			Entry("When 100", 100.0, "p100"),
			Entry("When 9", float64(9), "p9"),
			Entry("When 99", float64(99), "p99"),
			Entry("When 99.9", 99.9, "p99.9"),
			Entry("When 99.99", 99.99, "p99.99"),
			Entry("When 99.999", 99.999, "p99.999"),
			Entry("When 99.9999", 99.9999, "p99.9999"),
			Entry("When 99.99999", 99.99999, "p99.99999"),
			Entry("When 99.999999", 99.999999, "p100"),
		)
	})

	Context("Percent", func() {
		It("should not parse 0", func() {
			_, failed := ParsePercent(0)
			Expect(failed).To(HaveOccurred())
		})

		It("should not parse negative", func() {
			_, failed := ParsePercent(-1)
			Expect(failed).To(HaveOccurred())
		})

		It("should not parse over 100", func() {
			_, failed := ParsePercent(100.1)
			Expect(failed).To(HaveOccurred())
		})

		It("should parse valid value", func() {
			p, failed := ParsePercent(100)
			Expect(failed).NotTo(HaveOccurred())
			Expect(p.Value()).To(Equal(float64(100)))
		})
	})
})
