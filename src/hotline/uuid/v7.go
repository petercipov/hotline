package uuid

import (
	"io"
	"time"

	"github.com/gofrs/uuid/v5"
)

type V7StringGenerator func(time.Time) (string, error)

func NewDeterministicV7(randReader io.Reader) V7StringGenerator {
	gen := uuid.NewGenWithOptions(
		uuid.WithRandomReader(randReader),
	)

	return func(time time.Time) (string, error) {
		uuidV7, err := gen.NewV7AtTime(time)
		if err != nil {
			return "", err
		}
		return uuidV7.String(), nil
	}
}
