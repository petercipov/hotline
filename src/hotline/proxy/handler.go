package proxy

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"hotline/proxy/internal/config"
	"hotline/proxy/internal/export"
)

// handler serves the proxy port. Slice 1 knows one ingress: the forward
// proxy, where the app puts the full vendor address in the request line.
type handler struct {
	cfg       *config.Config
	clock     Clock
	sink      *export.Sink
	transport *http.Transport
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.clock.Now()
	if !r.URL.IsAbs() {
		// A path or CONNECT: meant for an ingress that later slices build.
		refuse(w, http.StatusNotImplemented)
		return
	}
	if r.URL.Scheme != "http" && r.URL.Scheme != "https" || r.URL.Hostname() == "" {
		refuse(w, http.StatusBadRequest)
		return
	}
	id := integrationID(r.Header)
	host := strings.ToLower(r.URL.Hostname())
	api := h.cfg.API(id, host)
	settings := h.cfg.NotPassed
	if api != nil {
		settings = api.Settings
	}
	scheme := r.URL.Scheme
	if settings.TLSOrigination {
		scheme = "https"
	}
	port := r.URL.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[scheme]
	}
	c := &call{
		h:       h,
		start:   start,
		id:      id,
		api:     api,
		scheme:  scheme,
		host:    host,
		port:    port,
		in:      r,
		timeout: settings.VendorTimeout,
		reqBody: &tap{ReadCloser: r.Body},
	}
	r.Body = c.reqBody
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	c.cancel = cancel
	defer c.finish()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = scheme
			pr.Out.URL.Host = net.JoinHostPort(host, port)
			pr.Out.URL.Fragment = ""
			pr.Out.Host = r.URL.Host
		},
		Transport:     c,
		FlushInterval: -1,
		ErrorHandler:  c.fail,
		ErrorLog:      log.New(io.Discard, "", 0),
	}
	app := &toApp{ResponseWriter: w, appLeft: r.Context().Done()}
	c.app = app
	proxy.ServeHTTP(app, r.WithContext(ctx))
	_ = http.NewResponseController(w).Flush()
}

// toApp writes the reply to the app and notes whether the app had already
// gone when a part of the reply was ready for it. Checking before each
// write, not at the end, keeps an app that leaves after reading the whole
// reply from counting as gone.
type toApp struct {
	http.ResponseWriter
	appLeft <-chan struct{}
	gone    bool
}

func (a *toApp) WriteHeader(status int) {
	a.check()
	a.ResponseWriter.WriteHeader(status)
}

func (a *toApp) Write(b []byte) (int, error) {
	a.check()
	return a.ResponseWriter.Write(b)
}

func (a *toApp) check() {
	select {
	case <-a.appLeft:
		a.gone = true
	default:
	}
}

// Unwrap lets http.ResponseController reach the server's writer to flush.
func (a *toApp) Unwrap() http.ResponseWriter { return a.ResponseWriter }

// refuse answers a call the proxy does not forward.
func refuse(w http.ResponseWriter, status int) {
	w.Header().Set("Proxy-Status", "hotline; error=http_request_error")
	w.WriteHeader(status)
}

// integrationID reads the user name from "Proxy-Authorization: Basic".
// Anything else, including a broken or repeated header, counts as no ID.
func integrationID(h http.Header) string {
	values := h.Values("Proxy-Authorization")
	if len(values) != 1 {
		return ""
	}
	scheme, credentials, _ := strings.Cut(values[0], " ")
	if !strings.EqualFold(scheme, "basic") {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(credentials)
	if err != nil {
		return ""
	}
	id, _, found := strings.Cut(string(decoded), ":")
	if !found {
		return ""
	}
	return id
}

// call is one call through the proxy, from the app's request to the record.
type call struct {
	h       *handler
	start   time.Time
	id      string
	api     *config.API
	scheme  string
	host    string
	port    string
	in      *http.Request
	timeout time.Duration
	cancel  context.CancelFunc
	app     *toApp

	reqBody  *tap
	respBody *tap
	timedOut atomic.Bool

	mu        sync.Mutex
	phase     phase
	times     map[phase][2]time.Time
	firstByte time.Time
	vendorIn  time.Time
	vendorOut time.Time
	reused    bool
	peer      string
	looked    string
	resp      *http.Response
	status    int
	errorType string
}

type phase string

const (
	phaseDNS     phase = "dns"
	phaseConnect phase = "connect"
	phaseTLS     phase = "tls"
	phaseReply   phase = "reply"
)

// RoundTrip makes the vendor call. It waits for the vendor's answer up to
// vendorTimeout; the app giving up does not stop it.
func (c *call) RoundTrip(req *http.Request) (*http.Response, error) {
	c.times = map[phase][2]time.Time{}
	trace := &httptrace.ClientTrace{
		GetConn:              func(string) { c.mark(func() { c.vendorIn = c.h.clock.Now(); c.phase = phaseReply }) },
		DNSStart:             func(i httptrace.DNSStartInfo) { c.begin(phaseDNS); c.mark(func() { c.looked = i.Host }) },
		DNSDone:              func(httptrace.DNSDoneInfo) { c.end(phaseDNS) },
		ConnectStart:         func(string, string) { c.begin(phaseConnect) },
		ConnectDone:          func(string, string, error) { c.end(phaseConnect) },
		TLSHandshakeStart:    func() { c.begin(phaseTLS) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { c.end(phaseTLS) },
		GotConn:              c.gotConn,
		GotFirstResponseByte: func() { c.mark(func() { c.firstByte = c.h.clock.Now() }) },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	timer := c.h.clock.AfterFunc(c.timeout, func() {
		c.timedOut.Store(true)
		c.cancel()
	})
	resp, err := c.h.transport.RoundTrip(req)
	timer.Stop()
	if err != nil {
		return nil, err
	}
	c.respBody = &tap{ReadCloser: resp.Body, done: func() { c.mark(func() { c.vendorOut = c.h.clock.Now() }) }}
	resp.Body = c.respBody
	c.mark(func() { c.resp = resp; c.status = resp.StatusCode })
	return resp, nil
}

func (c *call) mark(f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f()
}

// begin starts a phase. A phase tried again, such as connecting to the next
// address of a host, keeps its first start, so no time is counted twice.
func (c *call) begin(p phase) {
	c.mark(func() {
		c.phase = p
		if c.times[p][0].IsZero() {
			c.times[p] = [2]time.Time{c.h.clock.Now()}
		}
	})
}

func (c *call) end(p phase) {
	c.mark(func() {
		t := c.times[p]
		t[1] = c.h.clock.Now()
		c.times[p] = t
	})
}

func (c *call) gotConn(info httptrace.GotConnInfo) {
	c.mark(func() {
		c.phase = phaseReply
		c.reused = info.Reused
		c.peer, _, _ = net.SplitHostPort(info.Conn.RemoteAddr().String())
	})
}

// fail answers the app when the proxy could not get a reply from the
// vendor, with the RFC 9209 error name in Proxy-Status.
func (c *call) fail(w http.ResponseWriter, _ *http.Request, err error) {
	c.mu.Lock()
	status, name := classify(c.phase, c.timedOut.Load(), err)
	c.status, c.errorType = status, name
	c.vendorOut = c.h.clock.Now()
	c.mu.Unlock()
	w.Header().Set("Proxy-Status", "hotline; error="+name)
	w.WriteHeader(status)
}

func classify(p phase, timedOut bool, err error) (int, string) {
	switch {
	case p == phaseDNS && timedOut:
		return http.StatusGatewayTimeout, "dns_timeout"
	case p == phaseDNS:
		return http.StatusBadGateway, "dns_error"
	case (p == phaseConnect || p == phaseTLS) && timedOut:
		return http.StatusGatewayTimeout, "connection_timeout"
	case p == phaseConnect && errors.Is(err, syscall.ECONNREFUSED):
		return http.StatusBadGateway, "connection_refused"
	case p == phaseConnect:
		return http.StatusBadGateway, "destination_unavailable"
	case p == phaseTLS && isCertificateError(err):
		return http.StatusBadGateway, "tls_certificate_error"
	case p == phaseTLS:
		return http.StatusBadGateway, "tls_protocol_error"
	case timedOut:
		return http.StatusGatewayTimeout, "http_response_timeout"
	}
	return http.StatusBadGateway, "http_response_incomplete"
}

// tap passes a body straight on and counts the bytes that passed. done, if
// set, runs once when the body ends: read to the end, broken off, or closed
// early.
type tap struct {
	io.ReadCloser
	done func()
	size int64
	err  error
}

func (t *tap) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	t.size += int64(n)
	if err != nil && !errors.Is(err, io.EOF) {
		t.err = err
	}
	if err != nil {
		t.finish()
	}
	return n, err
}

func (t *tap) Close() error {
	t.finish()
	return t.ReadCloser.Close()
}

func (t *tap) finish() {
	if t.done != nil {
		t.done()
		t.done = nil
	}
}
