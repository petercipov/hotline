package http_test

import (
	"bytes"
	"errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	http2 "hotline/http"
	"io"
	"net/http"
	"time"
)

var _ = Describe("Retry Round Tripper", func() {
	sut := retrySUT{}

	It("does not retry for empty list status codes and failed request", func() {
		sut.forEmpty()
		resp, respErr := sut.sendFailedRequest()
		Expect(respErr).To(BeNil())
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		retries := sut.getRetries()
		Expect(retries).To(BeEmpty())
	})

	It("will retry 500 3 times and exponentially", func() {
		sut.forRetry(500)
		resp, respErr := sut.sendFailedRequest()
		Expect(respErr).To(BeNil())
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		retries := sut.getRetries()
		Expect(retries).To(Equal([]fakeRetry{
			{
				sleep: 1500 * time.Millisecond,
			},
			{
				sleep: 2250 * time.Millisecond,
			},
			{
				sleep: 3375 * time.Millisecond,
			},
			{
				sleep: 5062 * time.Millisecond,
			},
			{
				sleep: 7593 * time.Millisecond,
			},
		}))
	})

	It("for success request it will not retry", func() {
		sut.forRetry(500)
		resp, respErr := sut.sendSuceessRequest()
		Expect(respErr).To(BeNil())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		retries := sut.getRetries()
		Expect(retries).To(BeEmpty())
	})

	It("for network failure it will not retry", func() {
		sut.forRetry(500)
		_, respErr := sut.sendNetworkFailureRequest()
		Expect(respErr).NotTo(BeNil())
		retries := sut.getRetries()
		Expect(retries).To(BeEmpty())
	})
})

type retrySUT struct {
	rountripper *http2.RetryRoundTripper

	responder *fakeRoundtripper
	timer     *fakeTime
}

func (s *retrySUT) forEmpty() {
	s.responder = &fakeRoundtripper{}
	s.timer = &fakeTime{}

	s.rountripper = http2.WrapWithRetries(
		s.responder,
		func(resp *http.Response, err error) bool {
			return false
		},
		5,
		1.5,
		s.timer.Sleep,
	)
}

func (s *retrySUT) forRetry(code ...int) {
	s.responder = &fakeRoundtripper{}
	s.timer = &fakeTime{}

	s.rountripper = http2.WrapWithRetries(
		s.responder,
		http2.RetryStatusCodes(code...),
		5,
		1.5,
		s.timer.Sleep,
	)
}

func (s *retrySUT) sendFailedRequest() (*http.Response, error) {
	req, reqErr := http.NewRequest("GET", "http://example.com", bytes.NewReader([]byte("some content")))
	Expect(reqErr).NotTo(HaveOccurred())

	s.responder.SendNext(http.StatusInternalServerError)
	return s.rountripper.RoundTrip(req)
}

func (s *retrySUT) sendSuceessRequest() (*http.Response, error) {
	req, reqErr := http.NewRequest("GET", "http://example.com", bytes.NewReader([]byte("some content")))
	Expect(reqErr).NotTo(HaveOccurred())

	s.responder.SendNext(http.StatusOK)
	return s.rountripper.RoundTrip(req)
}

func (s *retrySUT) sendNetworkFailureRequest() (interface{}, interface{}) {
	req, reqErr := http.NewRequest("GET", "http://example.com", bytes.NewReader([]byte("some content")))
	Expect(reqErr).NotTo(HaveOccurred())

	s.responder.SendError(errors.New("network failure"))
	return s.rountripper.RoundTrip(req)
}

type fakeRetry struct {
	sleep time.Duration
}

func (s *retrySUT) getRetries() []fakeRetry {
	var retries []fakeRetry
	for _, sleep := range s.timer.sleeps {
		retries = append(retries, fakeRetry{
			sleep: sleep,
		})
	}
	return retries
}

type fakeRoundtripper struct {
	statusCode int
	err        error
}

func (f *fakeRoundtripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: f.statusCode,
		Body:       io.NopCloser(bytes.NewReader([]byte("some content"))),
	}, nil
}

func (f *fakeRoundtripper) SendNext(statusCode int) {
	f.statusCode = statusCode
}

func (f *fakeRoundtripper) SendError(err error) {
	f.err = err
}

type fakeTime struct {
	sleeps []time.Duration
}

func (f *fakeTime) Sleep(t time.Duration) {
	f.sleeps = append(f.sleeps, t)
}
