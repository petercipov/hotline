package uuid

import (
	"encoding/base64"
	"io"
	"time"

	"github.com/gofrs/uuid/v5"
)

type V7StringGenerator func(time.Time) (string, error)

func NewV7(randReader io.Reader) V7StringGenerator {
	gen := uuid.NewGenWithOptions(
		uuid.WithRandomReader(randReader),
	)

	return func(time time.Time) (string, error) {
		uuidV7, err := gen.NewV7AtTime(time)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(uuidV7.Bytes()), nil
	}
}

type ConstantRandReader struct {
}

func (m *ConstantRandReader) Read(p []byte) (n int, err error) {
	for i := 0; i < len(p); i++ {
		p[i] = byte(1)
	}
	return len(p), nil
}

type ErrorRandReader struct {
}

func (m *ErrorRandReader) Read(_ []byte) (n int, err error) {
	return 0, io.EOF
}
