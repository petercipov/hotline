package setup

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"
)

type HttpServer interface {
	Host() string
	Start()
	Close() error
}

type CreateServer func(host string, handler http.Handler) HttpServer

type GoHttpServer struct {
	server *http.Server
}

func NewGoHttpServer(host string, handler http.Handler) *GoHttpServer {
	return &GoHttpServer{
		server: &http.Server{
			Addr:        host,
			Handler:     handler,
			ReadTimeout: 5 * time.Second,
		},
	}
}

func (g *GoHttpServer) Host() string {
	return g.server.Addr
}

func (g *GoHttpServer) Start() {
	go func() {
		_ = g.server.ListenAndServe()
	}()
}

func (g *GoHttpServer) Close() error {
	if g == nil {
		return nil
	}
	return g.server.Close()
}

type HttpTestServer struct {
	server *httptest.Server
}

func NewHttpTestServer(_ string, handler http.Handler) *HttpTestServer {
	return &HttpTestServer{
		server: httptest.NewServer(handler),
	}
}

func (t *HttpTestServer) Host() string {
	u, _ := url.Parse(t.server.URL)
	return u.Host
}

func (t *HttpTestServer) Start() {
	if len(t.server.URL) == 0 {
		slog.Info("Starting test server", slog.Any("server", t.server.URL))
		t.server.Start()
	}
}

func (t *HttpTestServer) Close() error {
	if t == nil {
		return nil
	}
	slog.Info("Closing test server", slog.Any("server", t.server.URL))
	t.server.Close()
	return nil
}
