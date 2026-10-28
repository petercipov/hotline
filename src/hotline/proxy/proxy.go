// Package proxy is the Hotline proxy: it forwards each call from an app to
// its vendor, measures it, and sends one record per call over OTLP.
package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"hotline/proxy/internal/config"
	"hotline/proxy/internal/export"
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
	server    *http.Server
	handler   *handler
	transport *http.Transport
	sink      *export.Sink
	served    chan struct{}
}

// Start loads the config, listens on its proxy address and serves calls
// until Shutdown. A config with problems is refused with all of them listed.
func Start(ctx context.Context, o Options) (*Proxy, error) {
	cfg, err := config.Load(o.ConfigPath)
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
		cfg:   cfg,
		clock: o.Clock,
		sink:  sink,
	}
	h.transport = &http.Transport{
		DialContext:         (&dialer{resolver: o.Resolver, dialer: o.Dialer, clock: o.Clock}).dial,
		TLSClientConfig:     &tls.Config{RootCAs: o.RootCAs, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	}
	p := &Proxy{
		handler: h,
		server: &http.Server{
			Handler:           h,
			ReadHeaderTimeout: 30 * time.Second,
			ErrorLog:          slog.NewLogLogger(o.Logger.Handler(), slog.LevelWarn),
		},
		transport: h.transport,
		sink:      sink,
		served:    make(chan struct{}),
	}
	go func() {
		defer close(p.served)
		_ = p.server.Serve(listener)
	}()
	for _, w := range cfg.Warnings {
		o.Logger.Warn("config setting ignored", "reason", w)
	}
	o.Logger.Info("hotline proxy started", "address", listener.Addr().String(), "config_version", cfg.Version)
	return p, nil
}

// Shutdown stops taking calls, lets calls in flight finish, then sends every
// queued record, all within the time ctx allows.
func (p *Proxy) Shutdown(ctx context.Context) error {
	err := p.server.Shutdown(ctx)
	<-p.served
	finished := make(chan struct{})
	go func() {
		p.handler.calls.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
	}
	p.transport.CloseIdleConnections()
	return errors.Join(err, p.sink.Shutdown(ctx))
}
