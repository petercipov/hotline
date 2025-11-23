package tdigest_test

import (
	"hotline/metrics/tdigest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Scaling", func() {
	It("weight based scaling gives biggest capacity in the center, smallest size on the edges ", func() {
		scaling := tdigest.NewWeightScaling(100)

		var maxWeights []float64
		totalWeight := uint64(1000)
		for quantile := 0; quantile <= 100; quantile += 1 {
			q := float64(quantile) / 100.0
			maxWeight := round(scaling.MaxWeight(q, q, totalWeight), 2)
			maxWeights = append(maxWeights, maxWeight)
		}

		Expect(maxWeights).To(Equal([]float64{
			1, 7.25, 9.8, 11.72, 13.31, 14.69, 15.92, 17.03, 18.04,
			18.98, 19.85, 20.66, 21.41, 22.13, 22.8, 23.43, 24.03,
			24.6, 25.14, 25.64, 26.13, 26.59, 27.02, 27.44, 27.83,
			28.2, 28.56, 28.89, 29.21, 29.51, 29.79, 30.05, 30.3,
			30.54, 30.76, 30.96, 31.15, 31.33, 31.49, 31.64, 31.78,
			31.9, 32.01, 32.1, 32.18, 32.25, 32.31, 32.35, 32.39,
			32.4, 32.41, 32.4, 32.39, 32.35, 32.31, 32.25, 32.18,
			32.1, 32.01, 31.9, 31.78, 31.64, 31.49, 31.33, 31.15,
			30.96, 30.76, 30.54, 30.3, 30.05, 29.79, 29.51, 29.21,
			28.89, 28.56, 28.2, 27.83, 27.44, 27.02, 26.59, 26.13,
			25.64, 25.14, 24.6, 24.03, 23.43, 22.8, 22.13, 21.41,
			20.66, 19.85, 18.98, 18.04, 17.03, 15.92, 14.69, 13.31,
			11.72, 9.8, 7.25, 1,
		}))

	})
})
