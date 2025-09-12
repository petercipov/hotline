package clock_test

import (
	"hotline/clock"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Entities", Ordered, func() {
	DescribeTable("conversion TimeFromUint64OrZero",
		func(value uint64, expectedString string) {
			t := clock.TimeFromUint64OrZero(value)

			Expect(t.Format(time.RFC3339)).To(Equal(expectedString))
		},
		Entry("now", uint64(1757708516201416000), "2025-09-12T20:21:56Z"),
		Entry("> math.MaxInt64", uint64(math.MaxInt64+1), "0001-01-01T00:00:00Z"),
	)

	DescribeTable("conversion TimeToUint64NanoOrZero",
		func(value string, expectedInt uint64) {
			parsed := clock.ParseTime(value)
			t := clock.TimeToUint64NanoOrZero(parsed)

			Expect(t).To(Equal(expectedInt))
		},
		Entry("now", "2025-09-12T20:21:56Z", uint64(1757708516000000000)),
	)
})
