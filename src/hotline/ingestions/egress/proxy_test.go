package egress_test

import (
	"errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/clock"
	"hotline/ingestions"
	"hotline/ingestions/egress"
	"hotline/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"
)

var _ = Describe("Proxy", func() {
	sut := proxySUT{}

	AfterEach(func() {
		sut.Close()
	})

	It("proxy request to dedicated server", func() {
		sut.ForRunningProxy()
		sut.ForDedicatedServer()
		resp := sut.WhenRequestIsSend()
		reqs := sut.RequestsAreProxiedToDedicatedServer()
		Expect(reqs).To(HaveLen(1))
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("server")).To(Equal("dedicated"))
		Expect(resp.Header.Get("content-type")).To(Equal("text/plain"))

		bodyBytes, readErr := io.ReadAll(resp.Body)
		Expect(readErr).To(BeNil())
		Expect(string(bodyBytes)).To(Equal("OK"))
	})

	It("return gateway timeout for request with long latency", func() {
		sut.ForRunningProxy()
		sut.ForDedicatedServerWithBigLatency()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout))
	})

	It("return gateway error for request without backend", func() {
		sut.ForRunningProxy()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))
	})

	It("return gateway error for broken transport", func() {
		sut.ForRunningProxyWithBrokenRoundTripper()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
	})
})

type proxySUT struct {
	managedTime *clock.ManualClock

	proxyServer      *httptest.Server
	dedicatedServer  *httptest.Server
	receivedRequests []*http.Request

	proxyClient *http.Transport
}

func (s *proxySUT) ForRunningProxy() {
	s.ForRunningProxyWithRoundTripper(&http.Transport{})
}

func (s *proxySUT) ForRunningProxyWithRoundTripper(roundtripper http.RoundTripper) {
	s.managedTime = clock.NewManualClock(clock.ParseTime("2025-05-18T12:02:10Z"))

	s.proxyServer = httptest.NewServer(egress.New(
		roundtripper,
		func(req []*ingestions.HttpRequest) {},
		s.managedTime,
		10*time.Millisecond,
		uuid.NewDeterministicV7(
			s.managedTime.Now,
			&constantRandReader{},
		),
	))

	proxyURL, parseErr := url.Parse(s.proxyServer.URL)
	Expect(parseErr).NotTo(HaveOccurred())

	s.proxyClient = &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
}

func (s *proxySUT) ForRunningProxyWithBrokenRoundTripper() {
	s.ForRunningProxyWithRoundTripper(&failingTransport{})
}

func (s *proxySUT) ForDedicatedServer() {
	s.dedicatedServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.receivedRequests = append(s.receivedRequests, r.Clone(r.Context()))
		w.Header().Add("server", "dedicated")
		w.Header().Add("content-type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
}

func (s *proxySUT) ForDedicatedServerWithBigLatency() {
	s.dedicatedServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.receivedRequests = append(s.receivedRequests, r.Clone(r.Context()))
		s.managedTime.Advance(1 * time.Second)
		w.Header().Add("server", "dedicated")
		w.Header().Add("content-type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
}

func (s *proxySUT) WhenRequestIsSend() *http.Response {
	serverURL := "http://unknown/abcd"
	if s.dedicatedServer != nil {
		serverURL = s.dedicatedServer.URL + "/abcd"
	}

	req, _ := http.NewRequest(http.MethodGet, serverURL, nil)
	resp, respErr := s.proxyClient.RoundTrip(req)
	Expect(respErr).To(BeNil())
	return resp
}

func (s *proxySUT) Close() {
	if s.proxyServer != nil {
		s.proxyServer.Close()
	}

	if s.dedicatedServer != nil {
		s.dedicatedServer.Close()
	}
	for _, req := range s.receivedRequests {
		req.Body.Close()
	}
	s.receivedRequests = nil
}

func (s *proxySUT) RequestsAreProxiedToDedicatedServer() []*http.Request {
	return s.receivedRequests
}

type failingTransport struct {
}

func (t *failingTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(&failingReader{}),
	}, nil
}

type failingReader struct{}

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, errors.New("some error")
}

type constantRandReader struct {
}

func (m *constantRandReader) Read(p []byte) (n int, err error) {
	for i := 0; i < len(p); i++ {
		p[i] = byte(1)
	}
	return len(p), nil
}
