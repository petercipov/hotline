package setup

import (
	"net/http"
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

func (g *GoHttpServer) Host() string {
	return g.server.Addr
}

func (g *GoHttpServer) Start() {
	go func() {
		_ = g.server.ListenAndServe()
	}()
}

func (g *GoHttpServer) Close() error {
	return g.server.Close()
}

func NewGoHttpServer(host string, handler http.Handler) HttpServer {
	return &GoHttpServer{
		server: &http.Server{
			Addr:        host,
			Handler:     handler,
			ReadTimeout: 5 * time.Second,
		},
	}
}
