package http

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
)

func UncompressGzip(reader io.Reader, maxBytesLimit int64) ([]byte, error) {
	decompressor, gzipErr := gzip.NewReader(reader)
	if gzipErr != nil {
		return nil, gzipErr
	}
	var uncompressed bytes.Buffer
	_, copyErr := io.Copy(&uncompressed, io.LimitReader(decompressor, maxBytesLimit))
	if copyErr != nil {
		if !errors.Is(copyErr, io.EOF) {
			return nil, copyErr
		}
	}
	_ = decompressor.Close()
	return uncompressed.Bytes(), nil
}

func CompressGzip(reader io.Reader) ([]byte, error) {
	var compressed bytes.Buffer
	compressor := gzip.NewWriter(&compressed)

	_, copyErr := io.Copy(compressor, reader)
	if copyErr != nil {
		return nil, copyErr
	}

	_ = compressor.Close()
	return compressed.Bytes(), nil
}
