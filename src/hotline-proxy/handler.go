package proxy

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"hotline-proxy/internal/config"
	"hotline-proxy/internal/export"
	"hotline-proxy/internal/spec"
)

// handler serves the proxy port. It knows four ingresses: the forward
// proxy, where the app puts the full vendor address in the request line;
// the vendor URL in path, where the app puts it after the proxy's own
// address; the Host header, where the app names only the vendor's host;
// and CONNECT, where the app opens a tunnel to the vendor's host and the
// proxy poses as the vendor inside it.
type handler struct {
	cfg       *config.Config
	clock     Clock
	logger    *slog.Logger
	sink      *export.Sink
	transport *http.Transport
	// internal is the transport for calls that may reach internal IPs.
	internal *http.Transport
	// intercept makes the certificates shown inside tunnels; nil when
	// CONNECT is not an ingress.
	intercept *interceptor
	// tunnels takes each tunnel once it is set up.
	tunnels *tunnels
	// calls counts calls in flight, including upgraded ones, which the
	// server stops tracking once they take over the connection.
	calls sync.WaitGroup
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.calls.Add(1)
	defer h.calls.Done()
	start := h.clock.Now()
	a := appConnOf(r)
	var ingress string
	var target *url.URL
	var status int
	id := integrationID(r.Header)
	switch {
	case a != nil && a.tunnel != nil:
		// The CONNECT carried the ID for every call in its tunnel.
		ingress, id = "connect", a.tunnel.id
		target, status = a.tunnel.find(r, a)
	case r.Method == http.MethodConnect && h.intercept != nil:
		h.connect(w, r, start)
		return
	default:
		ingress, target, status = findIngress(r)
	}
	if status != 0 {
		refuse(w, status)
		h.refused(r, start, ingress, id, target, status)
		return
	}
	host := strings.ToLower(target.Hostname())
	api := h.cfg.API(id, host)
	settings := h.cfg.NotPassed
	if api != nil {
		settings = api.Settings
	}
	scheme := vendorScheme(r, target, api, settings)
	port := target.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[scheme]
	}
	transport := h.transport
	if settings.AllowInternalIPs {
		transport = h.internal
	}
	c := &call{
		h:         h,
		start:     start,
		ingress:   ingress,
		target:    target,
		id:        id,
		api:       api,
		scheme:    scheme,
		host:      host,
		port:      port,
		in:        r,
		timeout:   settings.VendorTimeout,
		transport: transport,
		reqBody:   &tap{ReadCloser: r.Body, limit: keepFor(api)},
	}
	c.appTLS, c.appTimed = a.handshakeTime()
	r.Body = c.reqBody
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	c.cancel = cancel
	defer c.finish()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			out := *target
			out.Scheme = scheme
			out.Host = net.JoinHostPort(host, port)
			out.Fragment = ""
			pr.Out.URL = &out
			pr.Out.Host = target.Host
			if strings.EqualFold(pr.Out.Header.Get("Upgrade"), "h2c") {
				// A switch to HTTP/2 on the app's own link, which the
				// proxy does not offer: the call stays HTTP/1.1.
				pr.Out.Header.Del("Upgrade")
				pr.Out.Header.Del("Connection")
			}
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

// findIngress works out which ingress a request used and the vendor address
// it names. A status other than 0 means the proxy refuses the request; the
// address is then whatever part of it the proxy could read, or nil.
func findIngress(r *http.Request) (string, *url.URL, int) {
	switch {
	case r.Method == http.MethodConnect:
		// Not an ingress without listen.connect: the app would encrypt
		// with the vendor itself.
		return "", &url.URL{Host: r.Host}, http.StatusNotImplemented
	case r.URL.IsAbs():
		if r.URL.Scheme != "http" && r.URL.Scheme != "https" || r.URL.Hostname() == "" {
			return "forward_proxy", r.URL, http.StatusBadRequest
		}
		return "forward_proxy", r.URL, 0
	case inPath(r.RequestURI):
		// Only the clean form is used; a squashed slash or an encoded
		// address is refused rather than guessed at.
		lower := strings.ToLower(r.RequestURI)
		if !strings.HasPrefix(lower, "/http://") && !strings.HasPrefix(lower, "/https://") {
			return "url_in_path", nil, http.StatusBadRequest
		}
		target, err := url.Parse(r.RequestURI[1:])
		if err != nil || target.Hostname() == "" {
			return "url_in_path", nil, http.StatusBadRequest
		}
		return "url_in_path", target, 0
	}
	// The Host header ingress: the request names no scheme, so the caller
	// picks one.
	target := &url.URL{Host: r.Host, Path: r.URL.Path, RawPath: r.URL.RawPath, RawQuery: r.URL.RawQuery}
	if target.Hostname() == "" {
		return "host_header", nil, http.StatusBadRequest
	}
	return "host_header", target, 0
}

// vendorScheme is http or https for the vendor call. TLS origination and
// HTTP/2 from the app always mean https. A Host header call names no
// scheme: it goes out over https, unless its api turned TLS origination off.
func vendorScheme(r *http.Request, target *url.URL, api *config.API, settings config.Settings) string {
	switch {
	case settings.TLSOrigination, r.ProtoMajor == 2:
		return "https"
	case target.Scheme != "":
		return target.Scheme
	case api != nil:
		return "http"
	}
	return "https"
}

// inPath says whether a raw request target starts with a vendor address,
// clean or damaged: /http: or /https:, the colon plain or encoded.
func inPath(requestURI string) bool {
	lower := strings.ToLower(requestURI)
	for _, prefix := range []string{"/http:", "/https:", "/http%3a", "/https%3a"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

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
	ingress string
	// target is the vendor address the app named.
	target  *url.URL
	id      string
	api     *config.API
	scheme  string
	host    string
	port    string
	in      *http.Request
	timeout time.Duration
	// appTLS is how long setting up TLS with the app took, when this is
	// the first call on its connection and appTimed is set.
	appTLS   time.Duration
	appTimed bool
	// transport makes the vendor call.
	transport *http.Transport
	cancel    context.CancelFunc
	app       *toApp

	reqBody  *tap
	respBody *tap
	timedOut atomic.Bool

	mu        sync.Mutex
	phase     phase
	times     map[phase][2]time.Time
	firstByte time.Time
	vendorIn  time.Time
	vendorOut time.Time
	connected bool
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
		DNSStart:             func(i httptrace.DNSStartInfo) { c.lookup(i.Host) },
		DNSDone:              func(httptrace.DNSDoneInfo) { c.end(phaseDNS) },
		ConnectStart:         func(string, string) { c.begin(phaseConnect) },
		ConnectDone:          func(string, string, error) { c.end(phaseConnect) },
		TLSHandshakeStart:    func() { c.begin(phaseTLS) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { c.end(phaseTLS) },
		GotConn:              c.gotConn,
		GotFirstResponseByte: func() { c.mark(func() { c.firstByte = c.h.clock.Now() }) },
	}
	ctx := context.WithValue(req.Context(), deadlineKey{}, c.h.clock.Now().Add(c.timeout))
	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	timer := c.h.clock.AfterFunc(c.timeout, func() {
		c.timedOut.Store(true)
		c.cancel()
	})
	resp, err := c.transport.RoundTrip(req)
	timer.Stop()
	if err != nil {
		return nil, err
	}
	c.respBody = &tap{ReadCloser: resp.Body, limit: keepFor(c.api), done: func() { c.mark(func() { c.vendorOut = c.h.clock.Now() }) }}
	if both, ok := resp.Body.(io.Writer); ok && resp.StatusCode == http.StatusSwitchingProtocols {
		// An upgraded connection, such as a WebSocket, carries data both
		// ways, so the proxy must be able to write to it too. It may stay
		// quiet for as long as both sides like.
		resp.Body = upgraded{tap: c.respBody, Writer: both}
	} else {
		c.respBody.wait = c.waitForVendor
		resp.Body = c.respBody
	}
	c.mark(func() { c.resp = resp; c.status = resp.StatusCode })
	return resp, nil
}

// waitForVendor limits one wait for the next part of the reply to
// vendorTimeout, so a vendor that goes quiet can't hold the call forever.
// It returns the function that ends the wait.
func (c *call) waitForVendor() func() bool {
	return c.h.clock.AfterFunc(c.timeout, c.cancel).Stop
}

func (c *call) mark(f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f()
}

// begin starts a phase. A phase tried again, such as connecting to the next
// address of a host, keeps its first start, so no time is counted twice.
// Once the call has its connection, events from a connection still being
// opened in the background belong to no call and are ignored.
func (c *call) begin(p phase) {
	c.mark(func() {
		if c.connected {
			return
		}
		c.phase = p
		if c.times[p][0].IsZero() {
			c.times[p] = [2]time.Time{c.h.clock.Now()}
		}
	})
}

func (c *call) lookup(host string) {
	c.begin(phaseDNS)
	c.mark(func() {
		if !c.connected {
			c.looked = host
		}
	})
}

func (c *call) end(p phase) {
	c.mark(func() {
		if c.connected {
			return
		}
		t := c.times[p]
		t[1] = c.h.clock.Now()
		c.times[p] = t
	})
}

func (c *call) gotConn(info httptrace.GotConnInfo) {
	c.mark(func() {
		c.phase = phaseReply
		c.connected = true
		c.reused = info.Reused
		if info.Reused {
			// A lookup or connect already started was for a connection
			// this call doesn't use.
			c.times = map[phase][2]time.Time{}
			c.looked = ""
		}
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
	case errors.Is(err, errProhibited):
		return http.StatusForbidden, "destination_ip_prohibited"
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

// upgraded is the vendor's side of an upgraded connection: read through
// the tap, written straight to the vendor.
type upgraded struct {
	*tap
	io.Writer
}

// keptBody is how much of each body the proxy keeps for the checks.
const keptBody = 1 << 20

// keepFor is how much of a body to keep: none for a call that is not
// checked.
func keepFor(api *config.API) int {
	if api == nil {
		return 0
	}
	return keptBody
}

// tap passes a body straight on and counts the bytes that passed. It keeps
// a copy of up to limit bytes for the checks. done, if
// set, runs once when the body ends: read to the end, broken off, or closed
// early. The request body is read by the transport, which may still be
// reading when the record is made, so the counts are behind a lock.
type tap struct {
	io.ReadCloser
	done func()
	// wait, if set, runs before each read and limits how long it may take.
	wait func() func() bool

	limit int

	mu   sync.Mutex
	size int64
	err  error
	kept []byte
	over bool
}

func (t *tap) Read(p []byte) (int, error) {
	if t.wait != nil {
		defer t.wait()()
	}
	n, err := t.ReadCloser.Read(p)
	t.mu.Lock()
	t.size += int64(n)
	if t.limit > 0 {
		room := min(n, t.limit-len(t.kept))
		t.kept = append(t.kept, p[:room]...)
		t.over = t.over || room < n
	}
	if err != nil && !errors.Is(err, io.EOF) {
		t.err = err
	}
	t.mu.Unlock()
	if err != nil {
		t.finish()
	}
	return n, err
}

func (t *tap) Close() error {
	t.finish()
	return t.ReadCloser.Close()
}

// counts says how many bytes passed so far, and why the body broke off,
// if it did.
func (t *tap) counts() (int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.size, t.err
}

// copied is the copy of the body kept so far.
func (t *tap) copied() spec.Body {
	t.mu.Lock()
	defer t.mu.Unlock()
	return spec.Body{Data: t.kept, TooLarge: t.over}
}

func (t *tap) finish() {
	if t.done != nil {
		t.done()
		t.done = nil
	}
}
