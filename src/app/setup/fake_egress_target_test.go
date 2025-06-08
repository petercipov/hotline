package setup_test

import (
	"hotline/clock"
	"math/rand"
	"net/http"
	"time"
)

type fakeEgressTarget struct {
	clock clock.ManagedTime
	rand  *rand.Rand
}

func newFakeEgressTarget(clock clock.ManagedTime, seed int64) *fakeEgressTarget {
	return &fakeEgressTarget{
		clock: clock,
		rand:  rand.New(rand.NewSource(seed)),
	}
}

func (c *fakeEgressTarget) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	latency := 100 + c.rand.Int31n(5000)
	c.clock.Sleep(time.Duration(latency) * time.Millisecond)
	isFailure := c.rand.Int63n(2) == 0

	switch req.Method {
	case "GET":
		if isFailure {
			writer.WriteHeader(http.StatusInternalServerError)
		} else {
			writer.WriteHeader(http.StatusOK)
		}
		return
	case "POST":
		if isFailure {
			writer.WriteHeader(http.StatusInternalServerError)
		} else {
			writer.WriteHeader(http.StatusCreated)
		}
		return
	case "DELETE":
		writer.WriteHeader(http.StatusNoContent)
		return
	default:
		writer.WriteHeader(http.StatusOK)
	}
}
