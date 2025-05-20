package uuid

import (
	"github.com/gofrs/uuid/v5"
	"hotline/clock"
	"io"
	"time"
)

func NewDeterministicV7(now clock.NowFunc, reader io.Reader) V7StringGenerator {
	gen := uuid.NewGenWithOptions(
		uuid.WithEpochFunc(uuid.EpochFunc(now)),
		uuid.WithRandomReader(reader),
	)

	return func(time time.Time) (string, error) {
		uuidV7, err := gen.NewV7AtTime(time)
		if err != nil {
			return "", err
		}
		return uuidV7.String(), nil
	}
}
