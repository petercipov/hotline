package uuid

import "time"

type V7StringGenerator func(time.Time) (string, error)
