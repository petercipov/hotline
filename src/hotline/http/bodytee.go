package http

import (
	"bytes"
	"compress/gzip"
	"io"
)

type BodyTeeBuffer struct {
	body       io.ReadCloser
	teeReader  io.Reader
	buffer     *bytes.Buffer
	skipWriter *limitSkipWriter
}

func NewBodyTeeBuffer(body io.ReadCloser, contentType string, limit int64) (*BodyTeeBuffer, error) {
	if contentType == "gzip" {
		var gzipErr error
		body, gzipErr = gzip.NewReader(body)
		if gzipErr != nil {
			return nil, gzipErr
		}
	}

	buffer := &bytes.Buffer{}
	skipWriter := newLimitSkipWriter(buffer, limit)

	return &BodyTeeBuffer{
		body:       body,
		buffer:     buffer,
		skipWriter: skipWriter,
		teeReader:  io.TeeReader(body, skipWriter),
	}, nil
}

func (t *BodyTeeBuffer) Read(p []byte) (n int, err error) {
	return t.teeReader.Read(p)
}

func (t *BodyTeeBuffer) GetBody() (bool, *bytes.Buffer) {
	if t.skipWriter.IsLimitReached() {
		return true, t.buffer
	}

	return false, t.buffer
}

func (t *BodyTeeBuffer) Close() error {
	return t.body.Close()
}

type limitSkipWriter struct {
	writer io.Writer
	limit  int64
}

func newLimitSkipWriter(writer io.Writer, limit int64) *limitSkipWriter {
	return &limitSkipWriter{
		writer: writer,
		limit:  limit,
	}
}

func (w *limitSkipWriter) Write(content []byte) (int, error) {
	if int64(len(content)) > w.limit {
		content = content[0:w.limit]
	}

	n, writeErr := w.writer.Write(content)

	w.limit -= int64(n)
	return n, writeErr
}

func (w *limitSkipWriter) IsLimitReached() bool {
	return w.limit <= 0
}
