package setup

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
)

type HttpServer interface {
	Host() string
	Start()
	Close() error
}

type CreateServer func(host string, handler http.Handler) HttpServer

type HttpTestServer struct {
	server *httptest.Server
}

func NewHttpTestServer(handler http.Handler) *HttpTestServer {
	return &HttpTestServer{
		server: httptest.NewUnstartedServer(handler),
	}
}

func (t *HttpTestServer) Host() string {
	u, _ := url.Parse(t.server.URL)
	return u.Host
}

func (t *HttpTestServer) Start() {
	t.server.Start()
	slog.Info("Started test server", slog.Any("server", t.server.URL))
}

func (t *HttpTestServer) Close() error {
	if t == nil {
		return nil
	}
	slog.Info("Closing test server", slog.Any("server", t.server.URL))
	t.server.Close()
	return nil
}
