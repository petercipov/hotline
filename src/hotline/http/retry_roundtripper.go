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

func RetryStatusCodes(retryStatus ...int) func(resp *http.Response, err error) bool {
	retryCodes := make(map[int]int, len(retryStatus))
	for _, status := range retryStatus {
		retryCodes[status] = 0
	}

	return func(resp *http.Response, err error) bool {
		if err != nil {
			return false
		}
		_, found := retryCodes[resp.StatusCode]
		return found
	}
}

func WrapWithRetries(origin http.RoundTripper, shouldRetry func(resp *http.Response, err error) bool, retryMax int, inSeconds float64, sleep func(t time.Duration)) *RetryRoundTripper {
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

type IsRetryableResponse func(resp *http.Response, err error) bool
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
	for t.shouldRetry(response, responseErr) && retries < t.retryMax {
		if response.Body != nil {
			_, _ = io.ReadAll(response.Body)
			_ = response.Body.Close()
		}
		t.sleep(t.backoff(retries + 1))

		if req.Body != nil {
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		response, responseErr = t.origin.RoundTrip(req)
		retries++
	}

	return response, responseErr
}
