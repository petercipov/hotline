// Package proxy is the Hotline proxy: it forwards each call from an app to
// its vendor, measures it, and sends one record per call over OTLP.
package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"hotline-proxy/internal/config"
	"hotline-proxy/internal/export"
)

// Resolver looks up the addresses of a vendor host. *net.Resolver is one.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Dialer opens connections to vendors. *net.Dialer is one.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Clock tells the time and waits. SystemClock is the real one.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is a wait started by Clock.AfterFunc.
type Timer interface {
	Stop() bool
}

// SystemClock is the computer's own clock.
type SystemClock struct{}

// Now is time.Now.
func (SystemClock) Now() time.Time { return time.Now() }

// AfterFunc is time.AfterFunc.
func (SystemClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// Options hold what the proxy needs from the world. Every field is required.
type Options struct {
	// ConfigPath is the hotline-proxy.yaml file.
	ConfigPath string
	Resolver   Resolver
	Dialer     Dialer
	// RootCAs are the trusted certificate issuers for vendors; nil means
	// the system trust store.
	RootCAs *x509.CertPool
	Clock   Clock
	Logger  *slog.Logger
	// ExportInterval is how often queued records and counts are sent.
	ExportInterval time.Duration
	// ExportTimeout is how long one send to a destination may take,
	// retries included.
	ExportTimeout time.Duration
	Version       string
	InstanceID    string
}

// Proxy is a running proxy.
type Proxy struct {
	server *http.Server
	// tunnelServer serves the calls inside tunnels opened by CONNECT.
	tunnelServer *http.Server
	handler      *handler
	transports   []*http.Transport
	sink         *export.Sink
	served       chan struct{}
	tunnelServed chan struct{}
}

// Start loads the config, listens on its proxy address and serves calls
// until Shutdown. A config with problems is refused with all of them listed.
// With listen.tls set, the proxy port accepts only TLS. With listen.connect
// set, CONNECT is an ingress.
func Start(ctx context.Context, o Options) (*Proxy, error) {
	cfg, err := config.Load(o.ConfigPath, o.Clock.Now())
	if err != nil {
		return nil, err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.ProxyAddr)
	if err != nil {
		return nil, err
	}
	sink, err := export.New(ctx, cfg.Exporters, export.Service{Version: o.Version, InstanceID: o.InstanceID},
		export.Timing{Interval: o.ExportInterval, Timeout: o.ExportTimeout}, o.Logger)
	if err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	h := &handler{
		cfg:    cfg,
		clock:  o.Clock,
		logger: o.Logger,
		sink:   sink,
	}
	if cfg.Connect != nil {
		h.intercept = newInterceptor(cfg.Connect, o.Clock, o.Logger)
	}
	// Calls that may reach internal IPs get their own connections, so a
	// connection opened for them is never reused for a call that may not.
	h.transport = newTransport(o, false)
	h.internal = newTransport(o, true)
	// HTTP/1.1, and HTTP/2 from the first byte (prior knowledge), also
	// after TLS, which the proxy sets up itself. The older Upgrade: h2c
	// handshake is not offered.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	h.tunnels = newTunnels(listener)
	newServer := func() *http.Server {
		return &http.Server{
			Handler:           h,
			Protocols:         protocols,
			ReadHeaderTimeout: 30 * time.Second,
			ErrorLog:          log.New(serverLog{logger: o.Logger}, "", 0),
			ConnContext:       connContext,
		}
	}
	p := &Proxy{
		handler:      h,
		server:       newServer(),
		tunnelServer: newServer(),
		transports:   []*http.Transport{h.transport, h.internal},
		sink:         sink,
		served:       make(chan struct{}),
		tunnelServed: make(chan struct{}),
	}
	port := listener
	if cfg.TLS != nil {
		// The proxy sets up TLS itself, so the server sees plain
		// connections.
		port = tlsOnlyListener{Listener: listener, config: newAppCert(cfg.TLS, o.Clock, o.Logger).tlsConfig(), clock: o.Clock, logger: o.Logger}
	}
	go func() {
		defer close(p.served)
		_ = p.server.Serve(port)
	}()
	go func() {
		defer close(p.tunnelServed)
		_ = p.tunnelServer.Serve(h.tunnels)
	}()
	for _, w := range cfg.Warnings {
		o.Logger.Warn("config setting ignored", "reason", w)
	}
	o.Logger.Info("hotline proxy started", "address", listener.Addr().String(), "config_version", cfg.Version)
	return p, nil
}

func newTransport(o Options, allowInternal bool) *http.Transport {
	d := &dialer{resolver: o.Resolver, dialer: o.Dialer, clock: o.Clock, allowInternal: allowInternal}
	return &http.Transport{
		DialContext:         d.dial,
		TLSClientConfig:     &tls.Config{RootCAs: o.RootCAs, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	}
}

// Validate checks a config and every spec and certificate it points to,
// without starting anything. A config with problems is refused with all of
// them listed.
func Validate(configPath string, clock Clock) error {
	_, err := config.Load(configPath, clock.Now())
	return err
}

// Shutdown stops taking calls, lets calls in flight finish, then sends every
// queued record, all within the time ctx allows.
func (p *Proxy) Shutdown(ctx context.Context) error {
	// Both servers stop at once: a tunnel opened over HTTP/2 is a call on
	// the proxy port that lasts until the tunnels' server closes the
	// tunnel. Tunnels still being set up are cut short.
	stopped := make(chan error, 1)
	go func() { stopped <- p.server.Shutdown(ctx) }()
	err := errors.Join(p.tunnelServer.Shutdown(ctx), <-stopped)
	<-p.served
	<-p.tunnelServed
	finished := make(chan struct{})
	go func() {
		p.handler.calls.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
	}
	for _, t := range p.transports {
		t.CloseIdleConnections()
	}
	return errors.Join(err, p.sink.Shutdown(ctx))
}
