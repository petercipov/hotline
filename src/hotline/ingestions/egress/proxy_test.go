package egress_test

import (
	"context"
	"errors"
	"hotline/clock"
	hotlineHttp "hotline/http"
	"hotline/ingestions"
	"hotline/ingestions/egress"
	"hotline/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Proxy", Ordered, func() {
	sut := proxySUT{}

	AfterEach(func() {
		sut.Close()
	})

	Context("Gzipped content", func() {
		It("can proxy request to dedicated server", func() {
			sut.ForRunningProxy()
			sut.ForDedicatedServer()
			resp := sut.WhenGzipRequestIsSend()
			reqs := sut.RequestsAreProxiedToDedicatedServer()
			Expect(reqs).To(HaveLen(1))
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("server")).To(Equal("dedicated"))
			Expect(resp.Header.Get("content-type")).To(Equal("text/plain"))
		})
	})

	It("can will not handle invalid gzip request", func() {
		sut.ForRunningProxy()
		resp := sut.WhenWrongGzipRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
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
		Expect(readErr).ToNot(HaveOccurred())
		Expect(string(bodyBytes)).To(Equal("OK"))
		ingested := sut.IngestedRequests()
		Expect(ingested).To(HaveLen(1))
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
		Expect(ingestedRequest.ID).To(HaveLen(22))
		Expect(ingestedRequest.URL.Path).To(Equal("/abcd"))
		Expect(ingestedRequest.EndTime.After(ingestedRequest.StartTime)).To(BeTrue())
	})

	It("return gateway timeout for request with long latency", func() {
		sut.ForRunningProxy()
		sut.ForDedicatedServerWithBigLatency()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout))

		ingested := sut.IngestedRequests()
		Expect(ingested).To(HaveLen(1))
		ingestedRequest := ingested[0]
		Expect(ingestedRequest.ErrorType).To(Equal("timeout"))
	})

	It("return gateway error for request without backend", func() {
		sut.ForRunningProxy()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusBadGateway))

		ingested := sut.IngestedRequests()
		Expect(ingested).To(HaveLen(1))
		ingestedRequest := ingested[0]
		Expect(ingestedRequest.ErrorType).To(Equal("unknown"))
	})

	It("return gateway error for broken transport", func() {
		sut.ForRunningProxyWithBrokenRoundTripper()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))

		ingested := sut.IngestedRequests()
		Expect(ingested).To(HaveLen(1))
		ingestedRequest := ingested[0]
		Expect(ingestedRequest.ErrorType).To(Equal("proxy_copy_err"))
	})

	It("return gateway error for internal rand issue", func() {
		sut.ForRunningProxyWithFailingRand()
		sut.ForDedicatedServer()
		resp := sut.WhenRequestIsSend()
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		Expect(sut.IngestedRequests()).To(BeEmpty())
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
	s.managedTime = clock.NewDefaultManualClock()

	s.semantics = egress.DefaultRequestSemantics()
	s.integrationID = "integration 123"
	s.proxyServer = httptest.NewServer(egress.New(
		roundtripper,
		func(req *ingestions.HttpRequest) {
			s.ingestedRequests = append(s.ingestedRequests, req)
		},
		s.managedTime,
		10*time.Millisecond,
		uuid.NewV7(
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

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL, nil)
	req.Header.Add(s.semantics.RequestIDName, "request-id-123")
	req.Header.Add(s.semantics.IntegrationIDName, s.integrationID)
	resp, respErr := s.proxyClient.RoundTrip(req)
	Expect(respErr).ToNot(HaveOccurred())
	return resp
}

func (s *proxySUT) WhenWrongGzipRequestIsSend() *http.Response {
	serverURL := "http://unknown/bookings"
	if s.dedicatedServer != nil {
		serverURL = s.dedicatedServer.URL + "/bookings"
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, serverURL, nil)
	req.Header.Add(s.semantics.RequestIDName, "request-id-123")
	req.Header.Add(s.semantics.IntegrationIDName, s.integrationID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Body = io.NopCloser(strings.NewReader("wrong gzip content"))
	resp, respErr := s.proxyClient.RoundTrip(req)
	Expect(respErr).ToNot(HaveOccurred())
	return resp
}

func (s *proxySUT) WhenGzipRequestIsSend() *http.Response {
	serverURL := "http://unknown/bookings"
	if s.dedicatedServer != nil {
		serverURL = s.dedicatedServer.URL + "/bookings"
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, serverURL, nil)
	req.Header.Add(s.semantics.RequestIDName, "request-id-123")
	req.Header.Add(s.semantics.IntegrationIDName, s.integrationID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	contentBytes, contentErr := hotlineHttp.CompressGzip(strings.NewReader(`
		{
		  "trip_id": "4f4e4e1-c824-4d63-b37a-d8d698862f1d",
		  "passenger_name": "John Doe",
		  "has_bicycle": true,
		  "has_dog": true
		}
	`))
	Expect(contentErr).ToNot(HaveOccurred())
	req.Body = io.NopCloser(strings.NewReader(string(contentBytes)))
	resp, respErr := s.proxyClient.RoundTrip(req)
	Expect(respErr).ToNot(HaveOccurred())
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
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(&failingReader{}),
	}, nil
}

type failingReader struct{}

var errSome = errors.New("some error")

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, errSome
}

type constantRandReader struct {
}

func (m *constantRandReader) Read(p []byte) (n int, err error) {
	for i := 0; i < len(p); i++ {
		p[i] = byte(1)
	}
	return len(p), nil
}
