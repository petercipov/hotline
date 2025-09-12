package http

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"time"
)

type RetryRoundTripper struct {
	origin      http.RoundTripper
	shouldRetry IsRetryableResponse

	retryMax int
	backoff  BackoffFunc
	sleep    func(t time.Duration)
}

func RetryStatusCodes(retryStatus ...int) func(statusCode int, err error) bool {
	retryCodes := make(map[int]int, len(retryStatus))
	for _, status := range retryStatus {
		retryCodes[status] = 0
	}

	return func(statusCode int, err error) bool {
		if err != nil {
			return false
		}
		_, found := retryCodes[statusCode]
		return found
	}
}

func WrapWithRetries(origin http.RoundTripper, shouldRetry func(statusCode int, err error) bool, retryMax int, inSeconds float64, sleep func(t time.Duration)) *RetryRoundTripper {
	backoff := ExponentialBackoff{
		Exponent: inSeconds,
	}

	return &RetryRoundTripper{
		origin:      origin,
		shouldRetry: shouldRetry,
		retryMax:    retryMax,
		backoff:     backoff.Delay,
		sleep:       sleep,
	}
}

type IsRetryableResponse func(statusCode int, err error) bool
type BackoffFunc func(retryCount int) time.Duration

type ExponentialBackoff struct {
	Exponent float64
}

func (e *ExponentialBackoff) Delay(retryCount int) time.Duration {
	millis := int64(math.Pow(e.Exponent, float64(retryCount)) * 1000)
	return time.Duration(millis) * time.Millisecond
}

func (t *RetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	response, responseErr := t.origin.RoundTrip(req)
	retries := 0
	statusCode := 0
	if response != nil {
		statusCode = response.StatusCode
	}
	for t.shouldRetry(statusCode, responseErr) && retries < t.retryMax {
		if response != nil && response.Body != nil {
			_, _ = io.ReadAll(response.Body)
			_ = response.Body.Close()
		}
		t.sleep(t.backoff(retries + 1))

		if req.Body != nil {
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		response, responseErr = t.origin.RoundTrip(req)
		if response != nil {
			statusCode = response.StatusCode
		}
		retries++
	}

	return response, responseErr
}
