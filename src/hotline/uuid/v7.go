package uuid

import (
	"github.com/gofrs/uuid/v5"
	"io"
	"time"
)

func NewDeterministicV7(randReader io.Reader) V7StringGenerator {
	gen := uuid.NewGenWithOptions(
		uuid.WithEpochFunc(func() time.Time {
			return time.Time{}
		}),
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
