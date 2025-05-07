package egress

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"time"
)

type Proxy struct {
	transport http.RoundTripper
	timeout   time.Duration
}

func New(transport http.RoundTripper, timeout time.Duration) *Proxy {
	return &Proxy{
		transport: transport,
		timeout:   timeout,
	}
}

func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	receivedTime := time.Now()
	reqCtx, cancel := context.WithDeadlineCause(
		req.Context(),
		receivedTime.Add(p.timeout),
		context.DeadlineExceeded)
	defer cancel()
	trace := &httptrace.ClientTrace{}
	resp, respErr := p.transport.RoundTrip(req.WithContext(httptrace.WithClientTrace(reqCtx, trace)))
	if respErr != nil {
		if errors.Is(respErr, context.DeadlineExceeded) {
			log.Printf("Error proxying request: timeout")
			rw.WriteHeader(http.StatusGatewayTimeout)
			return
		}

		log.Printf("Error proxying request: %s", respErr.Error())
		rw.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() {
		resp.Body.Close()
	}()
	for key, values := range resp.Header {
		rw.Header()[key] = values
	}
	rw.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(rw, resp.Body)
	if copyErr != nil {
		log.Printf("Error copying response, http status already sent,: %s", copyErr.Error())
		return
	}
}
