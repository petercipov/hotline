package schemas

import (
	"hotline/uuid"
	"time"
)

type ID string

type IDGenerator func(now time.Time) (ID, error)

func NewIDGenerator(generator uuid.V7StringGenerator) IDGenerator {
	return func(now time.Time) (ID, error) {
		v7Str, err := generator(now)
		if err != nil {
			return "", err
		}
		return ID("SC" + v7Str), nil
	}
}
