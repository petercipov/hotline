package schemas_test

import (
	"hotline/schemas"
	"hotline/uuid"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Schema ID", func() {
	It("can generate schemaID", func() {
		idGenerator := schemas.NewIDGenerator(uuid.NewV7(&uuid.ConstantRandReader{}))

		id, err := idGenerator(time.Time{})
		Expect(err).ToNot(HaveOccurred())
		Expect(id).To(Equal(schemas.ID("SCx3zt0ygAcQGBAQEBAQEBAQ")))
		Expect(id.String()).To(Equal("SCx3zt0ygAcQGBAQEBAQEBAQ"))
	})

	It("can fails if ran cannot be read", func() {
		idGenerator := schemas.NewIDGenerator(uuid.NewV7(&uuid.ErrorRandReader{}))

		_, err := idGenerator(time.Time{})
		Expect(err).To(HaveOccurred())
	})
})
