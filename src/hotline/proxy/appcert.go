package proxy

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hotline/proxy/internal/config"
)

// appCert is the certificate the proxy shows to apps. It follows its files:
// when they change, new connections get the new certificate, and
// connections already open keep the old one. A pair that is broken or
// expired is not taken; the last good one stays.
type appCert struct {
	certFile, keyFile string
	clock             Clock
	logger            *slog.Logger

	current atomic.Pointer[tls.Certificate]

	mu sync.Mutex
	// seen are the files as last read, good or not, so a broken pair is
	// read and reported only once.
	seen [2]os.FileInfo
}

func newAppCert(t *config.TLS, clock Clock, logger *slog.Logger) *appCert {
	c := &appCert{certFile: t.CertFile, keyFile: t.KeyFile, clock: clock, logger: logger}
	c.current.Store(t.Certificate)
	c.seen = c.stat()
	return c
}

// tlsConfig is the TLS setup of the proxy port: TLS 1.2 and 1.3 with Go's
// default ciphers, offering HTTP/2 and HTTP/1.1.
func (c *appCert) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: c.get,
	}
}

// get gives the certificate for a new connection, whatever name the app
// asks for. It first checks whether the files changed, which costs two
// file lookups per new connection.
func (c *appCert) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	files := c.stat()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !changed(c.seen, files) {
		return c.current.Load(), nil
	}
	c.seen = files
	cert, err := config.LoadCertificate(c.certFile, c.keyFile, c.clock.Now())
	if err != nil {
		c.logger.Warn("kept the TLS certificate, the new files are not usable", "reason", err.Error())
		return c.current.Load(), nil
	}
	c.current.Store(cert)
	c.logger.Info("new TLS certificate in use", "not_after", cert.Leaf.NotAfter)
	return cert, nil
}

func (c *appCert) stat() [2]os.FileInfo {
	var files [2]os.FileInfo
	files[0], _ = os.Stat(c.certFile) // a missing file shows up when it is read
	files[1], _ = os.Stat(c.keyFile)
	return files
}

// changed tells whether either file is another file than before, or was
// written since. Kubernetes swaps a Secret's files for new ones, and an
// editor writes in place.
func changed(before, now [2]os.FileInfo) bool {
	return !same(before[0], now[0]) || !same(before[1], now[1])
}

func same(a, b os.FileInfo) bool {
	return a == nil && b == nil ||
		a != nil && b != nil && os.SameFile(a, b) && a.ModTime().Equal(b.ModTime()) && a.Size() == b.Size()
}

// serverLog routes the HTTP server's own log lines: an app that fails to
// set up TLS is routine noise, logged at debug level; anything else is a
// warning.
type serverLog struct{ logger *slog.Logger }

func (s serverLog) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if detail, found := strings.CutPrefix(line, "http: TLS handshake error from "); found {
		s.logger.Debug("app TLS setup failed", "detail", detail)
	} else {
		s.logger.Warn("http server", "detail", line)
	}
	return len(p), nil
}

// errPlainHTTP is why TLS failed to set up when an app sent plain HTTP.
var errPlainHTTP = errors.New("proxy: the app sent plain HTTP to the TLS port")

// handshakeTimeout bounds how long an app may take to set up TLS.
const handshakeTimeout = 30 * time.Second

// tlsOnlyListener hands out connections that answer plain HTTP with 400,
// so an app that forgot TLS learns why, and that must start TLS within
// handshakeTimeout.
type tlsOnlyListener struct{ net.Listener }

func (l tlsOnlyListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	// The server sets its own read deadlines once TLS is up.
	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	return &tlsOnlyConn{Conn: conn}, nil
}

// tlsOnlyConn checks the first byte an app sends: TLS always starts with a
// handshake record, 0x16.
type tlsOnlyConn struct {
	net.Conn
	checked bool
}

func (c *tlsOnlyConn) Read(p []byte) (int, error) {
	if c.checked {
		return c.Conn.Read(p)
	}
	c.checked = true
	n, err := c.Conn.Read(p)
	if n > 0 && p[0] != 0x16 {
		c.refusePlain(p[:n])
		return 0, errPlainHTTP
	}
	return n, err
}

// refusePlain reads the rest of the plain request, so closing the
// connection does not reset it before the app reads the answer, and
// answers 400.
func (c *tlsOnlyConn) refusePlain(first []byte) {
	req, err := http.ReadRequest(bufio.NewReader(io.MultiReader(bytes.NewReader(first), c.Conn)))
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(req.Body, 1<<20))
	}
	const body = "the proxy port accepts only TLS\n"
	_, _ = io.WriteString(c.Conn, "HTTP/1.1 400 Bad Request\r\n"+
		"Proxy-Status: hotline; error=http_request_error\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"Connection: close\r\n"+
		"Content-Length: "+strconv.Itoa(len(body))+"\r\n\r\n"+body)
}
