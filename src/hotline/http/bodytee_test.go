package http_test

import (
	"encoding/hex"
	"hotline/http"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BodyTeeBuffer", Ordered, func() {
	sut := bodyTeeSut{}
	It("will handle simple body", func() {
		sut.forBodyTeeBuffer("hello world", "text")
		content, contentErr := sut.ReadAll()
		Expect(contentErr).ToNot(HaveOccurred())
		Expect(content).To(Equal([]byte("hello world")))

		limitReached, buffer := sut.GetBodyBuffer()
		Expect(limitReached).To(BeFalse())
		Expect(buffer).To(Equal([]byte("hello world")))
	})

	It("tee body read up to limit", func() {
		sut.forBodyTeeBuffer(strings.Repeat("aaaaa", 30), "text")
		bodyContent, contentErr := sut.ReadAll()
		Expect(contentErr).ToNot(HaveOccurred())
		Expect(bodyContent).To(HaveLen(150))

		limitReached, buffer := sut.GetBodyBuffer()
		Expect(limitReached).To(BeTrue())
		Expect(buffer).To(HaveLen(100))
	})

	It("will handle empty gzip body", func() {
		gzipHeader, _ := hex.DecodeString(emptyGzipHex)

		sut.forBodyTeeBuffer(string(gzipHeader), "gzip")
		content, contentErr := sut.ReadAll()
		Expect(contentErr).ToNot(HaveOccurred())
		Expect(content).To(BeEmpty())

		limitReached, buffer := sut.GetBodyBuffer()
		Expect(limitReached).To(BeFalse())
		Expect(buffer).To(BeEmpty())
	})

	It("will fail for invalid gzip body", func() {
		err := sut.forFailingBodyBuffer("invalid gzip", "gzip")
		Expect(err).To(HaveOccurred())
	})
})

type bodyTeeSut struct {
	bodyBuffer *http.BodyTeeBuffer
}

func (s *bodyTeeSut) forBodyTeeBuffer(content string, contentType string) {
	var bufferErr error
	s.bodyBuffer, bufferErr = http.NewBodyTeeBuffer(io.NopCloser(strings.NewReader(content)), contentType, 100)
	Expect(bufferErr).NotTo(HaveOccurred())
}

func (s *bodyTeeSut) forFailingBodyBuffer(content string, contentType string) error {
	var bufferErr error
	s.bodyBuffer, bufferErr = http.NewBodyTeeBuffer(io.NopCloser(strings.NewReader(content)), contentType, 100)
	return bufferErr
}

func (s *bodyTeeSut) ReadAll() ([]byte, error) {
	byteArr, byteErr := io.ReadAll(s.bodyBuffer)
	closeErr := s.bodyBuffer.Close()
	Expect(closeErr).NotTo(HaveOccurred())
	return byteArr, byteErr
}

func (s *bodyTeeSut) GetBodyBuffer() (bool, []byte) {
	reached, bodyBuffer := s.bodyBuffer.GetBody()

	return reached, bodyBuffer.Bytes()
}
