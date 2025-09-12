package egress_test

import (
	"errors"
	"hotline/clock"
	"hotline/ingestions"
	"hotline/ingestions/egress"
	"hotline/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy", Ordered, func() {
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
		ingested := sut.IngestedRequests()
		Expect(len(ingested)).To(Equal(1))
		ingestedRequest := ingested[0]

		Expect(ingestedRequest).To(Equal(&ingestions.HttpRequest{
			ID:              ingestedRequest.ID,
			IntegrationID:   "integration 123",
			ProtocolVersion: "HTTP/1.1",
			Method:          "GET",
			StatusCode:      "200",
			URL:             ingestedRequest.URL,
			StartTime:       ingestedRequest.StartTime,
			EndTime:         ingestedRequest.EndTime,
			ErrorType:       "",
			CorrelationID:   "request-id-123",
		}))
		Expect(len(ingestedRequest.ID)).To(Equal(22))
		Expect(ingestedRequest.URL.Path).To(Equal("/abcd"))
		Expect(ingestedRequest.EndTime.After(ingestedRequest.StartTime)).To(BeTrue())
	})

	It("return gateway timeout for request with long latency", func() {
		sut.ForRunningProxy()
		sut.ForDedicatedServerWithBigLatency()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout))

		ingested := sut.IngestedRequests()
		Expect(len(ingested)).To(Equal(1))
		ingestedRequest := ingested[0]
		Expect(ingestedRequest.ErrorType).To(Equal("timeout"))
	})

	It("return gateway error for request without backend", func() {
		sut.ForRunningProxy()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))

		ingested := sut.IngestedRequests()
		Expect(len(ingested)).To(Equal(1))
		ingestedRequest := ingested[0]
		Expect(ingestedRequest.ErrorType).To(Equal("unknown"))
	})

	It("return gateway error for broken transport", func() {
		sut.ForRunningProxyWithBrokenRoundTripper()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))

		ingested := sut.IngestedRequests()
		Expect(len(ingested)).To(Equal(1))
		ingestedRequest := ingested[0]
		Expect(ingestedRequest.ErrorType).To(Equal("proxy_copy_err"))
	})

	It("return gateway error for internal rand issue", func() {
		sut.ForRunningProxyWithFailingRand()
		sut.ForDedicatedServer()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		Expect(len(sut.IngestedRequests())).To(Equal(0))
	})

	It("return bad gateway for missing integration id", func() {
		sut.ForRunningProxy()
		resp := sut.WhenRequestWithoutIntegrationIDIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})
})

type proxySUT struct {
	managedTime *clock.ManualClock

	proxyServer      *httptest.Server
	dedicatedServer  *httptest.Server
	receivedRequests []*http.Request

	ingestedRequests []*ingestions.HttpRequest

	proxyClient *http.Transport

	semantics egress.RequestSemantics

	integrationID string
}

func (s *proxySUT) ForRunningProxy() {
	s.ForRunningProxyWithRoundTripper(&http.Transport{}, &constantRandReader{})
}

func (s *proxySUT) ForRunningProxyWithRoundTripper(roundtripper http.RoundTripper, reader io.Reader) {
	s.managedTime = clock.NewManualClock(clock.ParseTime("2025-05-18T12:02:10Z"), 1*time.Second)

	s.semantics = egress.DefaultRequestSemantics()
	s.integrationID = "integration 123"
	s.proxyServer = httptest.NewServer(egress.New(
		roundtripper,
		func(req *ingestions.HttpRequest) {
			s.ingestedRequests = append(s.ingestedRequests, req)
		},
		s.managedTime,
		10*time.Millisecond,
		uuid.NewDeterministicV7(
			reader,
		),
		&s.semantics,
	))

	proxyURL, parseErr := url.Parse(s.proxyServer.URL)
	Expect(parseErr).NotTo(HaveOccurred())

	s.proxyClient = &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
}

func (s *proxySUT) ForRunningProxyWithBrokenRoundTripper() {
	s.ForRunningProxyWithRoundTripper(&failingTransport{}, &constantRandReader{})
}

func (s *proxySUT) ForRunningProxyWithFailingRand() {
	s.ForRunningProxyWithRoundTripper(&http.Transport{}, &failingReader{})
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

func (s *proxySUT) IngestedRequests() []*ingestions.HttpRequest {
	return s.ingestedRequests
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
	req.Header.Add(s.semantics.RequestIDName, "request-id-123")
	req.Header.Add(s.semantics.IntegrationIDName, s.integrationID)
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
		_ = req.Body.Close()
	}
	s.receivedRequests = nil
	s.ingestedRequests = nil
}

func (s *proxySUT) RequestsAreProxiedToDedicatedServer() []*http.Request {
	return s.receivedRequests
}

func (s *proxySUT) WhenRequestWithoutIntegrationIDIsSend() *http.Response {
	s.integrationID = ""
	return s.WhenRequestIsSend()
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
