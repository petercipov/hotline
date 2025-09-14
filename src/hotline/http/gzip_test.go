package http_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"hotline/http"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const emptyGzipHex = "1f8b08000000000000ff010000ffff0000000000000000"

var _ = Describe("Gzip", func() {
	It("compress and decompress small text", func() {
		compressedBytes, compressErr := http.CompressGzip(strings.NewReader("hello world"))
		Expect(compressErr).NotTo(HaveOccurred())

		decompressedBytes, decompressErr := http.UncompressGzip(bytes.NewReader(compressedBytes), 1024)
		Expect(decompressErr).NotTo(HaveOccurred())

		Expect(string(decompressedBytes)).To(Equal("hello world"))
	})

	It("compress and decompress up to limit", func() {
		compressedBytes, comparesErr := http.CompressGzip(strings.NewReader("hello world"))
		Expect(comparesErr).NotTo(HaveOccurred())

		decompressedBytes, decompressErr := http.UncompressGzip(bytes.NewReader(compressedBytes), 5)
		Expect(decompressErr).NotTo(HaveOccurred())

		Expect(string(decompressedBytes)).To(Equal("hello"))
	})

	It("fails to compress from broken reader", func() {
		_, comparesErr := http.CompressGzip(&brokenReader{})
		Expect(comparesErr).To(HaveOccurred())
	})

	It("fails to decompress from broken reader", func() {
		_, decompressErr := http.UncompressGzip(&brokenReader{}, 1024)
		Expect(decompressErr).To(HaveOccurred())
	})

	It("fails to decompress from broken content", func() {
		gzipHeader, _ := hex.DecodeString(emptyGzipHex)
		_, decompressErr := http.UncompressGzip(io.MultiReader(bytes.NewReader(gzipHeader), &brokenReader{}), 1024)
		Expect(decompressErr).To(HaveOccurred())
	})

})

var ErrReaderIO = errors.New("reader io error")

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) {
	return 0, ErrReaderIO
}
