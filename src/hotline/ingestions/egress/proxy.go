package egress

import (
	"context"
	"errors"
	"hotline/clock"
	"hotline/ingestions"
	"hotline/integrations"
	"hotline/uuid"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"time"
)

type Proxy struct {
	transport http.RoundTripper
	timeout   time.Duration
	ingestion func(req *ingestions.HttpRequest)
	time      clock.ManagedTime
	v7        uuid.V7StringGenerator
}

func New(transport http.RoundTripper, ingestion func(req *ingestions.HttpRequest), time clock.ManagedTime, timeout time.Duration, v7 uuid.V7StringGenerator) *Proxy {
	return &Proxy{
		ingestion: ingestion,
		transport: transport,
		timeout:   timeout,
		time:      time,
		v7:        v7,
	}
}

func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	receivedTime := p.time.Now()
	v7String, v7Err := p.v7(receivedTime)
	if v7Err != nil {
		log.Printf("Error generating v7 string: %s", v7Err.Error())
		rw.WriteHeader(http.StatusBadGateway)
		return
	}

	ingestedRequest := &ingestions.HttpRequest{
		ID:              v7String,
		IntegrationID:   integrations.ID(req.Header.Get("User-Agent")),
		ProtocolVersion: req.Proto,
		Method:          req.Method,
		URL:             req.URL,
		StartTime:       receivedTime,
		CorrelationID:   req.Header.Get("x-request-id"),
	}

	reqCtx, cancel := context.WithCancel(req.Context())
	defer cancel()
	p.time.AfterFunc(p.timeout, func(_ time.Time) {
		cancel()
	})
	trace := &httptrace.ClientTrace{}
	resp, respErr := p.transport.RoundTrip(req.WithContext(httptrace.WithClientTrace(reqCtx, trace)))
	if respErr != nil {
		if errors.Is(respErr, context.Canceled) {
			log.Printf("Error proxying request: timeout")
			rw.WriteHeader(http.StatusGatewayTimeout)
			ingestedRequest.ErrorType = "timeout"
			p.ingestion(ingestedRequest)
			return
		}

		log.Printf("Error proxying request: %s", respErr.Error())
		rw.WriteHeader(http.StatusBadGateway)
		ingestedRequest.ErrorType = "unknown"
		p.ingestion(ingestedRequest)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	for key, values := range resp.Header {
		rw.Header()[key] = values
	}
	rw.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(rw, resp.Body)
	if copyErr != nil {
		log.Printf("Error copying response, http status already sent,: %s", copyErr.Error())
		ingestedRequest.ErrorType = "proxy_copy_err"
		p.ingestion(ingestedRequest)
		return
	}

	ingestedRequest.StatusCode = strconv.Itoa(resp.StatusCode)
	ingestedRequest.EndTime = p.time.Now()
	p.ingestion(ingestedRequest)
}
