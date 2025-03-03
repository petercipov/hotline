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
