package uuid_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/uuid"
	"io"
	"time"
)

var _ = Describe("UUID", func() {
	It("generates deterministic v7 UUID", func() {
		v7 := uuid.NewDeterministicV7(func() time.Time {
			return time.Time{}
		}, &constantRandReader{})

		v7uuid, err := v7(time.Time{})
		Expect(err).To(BeNil())
		Expect(v7uuid).To(Equal("c77cedd3-2800-7101-8101-010101010101"))
	})

	It("returns error when readin random fails", func() {
		v7 := uuid.NewDeterministicV7(func() time.Time {
			return time.Time{}
		}, &errorRandReader{})

		_, err := v7(time.Time{})
		Expect(err).NotTo(BeNil())
	})
})

type constantRandReader struct {
}

func (m *constantRandReader) Read(p []byte) (n int, err error) {
	for i := 0; i < len(p); i++ {
		p[i] = byte(1)
	}
	return len(p), nil
}

type errorRandReader struct {
}

func (m *errorRandReader) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}
