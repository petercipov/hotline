package uuid_test

import (
	"hotline/uuid"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UUID", func() {
	It("generates deterministic v7 UUID", func() {
		v7 := uuid.NewV7(&uuid.ConstantRandReader{})

		v7uuid, err := v7(time.Time{})
		Expect(err).ToNot(HaveOccurred())
		Expect(v7uuid).To(Equal("x3zt0ygAcQGBAQEBAQEBAQ"))
	})

	It("returns error when readin random fails", func() {
		v7 := uuid.NewV7(&uuid.ErrorRandReader{})

		_, err := v7(time.Time{})
		Expect(err).To(HaveOccurred())
	})
})
