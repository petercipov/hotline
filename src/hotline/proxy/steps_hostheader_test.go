package proxy_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func hostHeaderSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^the call uses the Host header ingress$`, func() error {
		return w.lastHas("hotline.ingress", "host_header")
	})
	sc.Step(`^the proxy forwards it to "([^"]+)" with "Host: (\S+)"$`, w.forwardWithHost)
	sc.Step(`^an app sends "([A-Z]+) ([^"]+)" over HTTP/1.0 with no Host header$`, func(method, path string) error {
		if err := w.start(); err != nil {
			return err
		}
		conn, err := w.dialProxy()
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		if _, err := io.WriteString(conn, method+" "+path+" HTTP/1.0\r\n"+basic(w.appID+":x")+"\r\n\r\n"); err != nil {
			return err
		}
		return w.readAnswer(conn)
	})
	sc.Step(`^an app sends a Host header call to the proxy's own address$`, func() error {
		if err := w.start(); err != nil {
			return err
		}
		return w.call("GET", "/v2/x", "Host: "+w.addr)
	})
	sc.Step(`^an app sends "([A-Z]+) ([^"]+)" with "Host: (\S+)" and "Upgrade: h2c"$`, func(method, path, host string) error {
		return w.call(method, path, "Host: "+host, "Connection: Upgrade, HTTP2-Settings", "Upgrade: h2c", "HTTP2-Settings: AAMAAABkAAQCAAAAAAIAAAAA")
	})
	sc.Step(`^the connection stays HTTP/1.1$`, func() error {
		if w.resp == nil || w.resp.Proto != "HTTP/1.1" || w.resp.StatusCode != http.StatusOK {
			return failf("the app got %v", w.resp)
		}
		return nil
	})

	sc.Step(`^an app that speaks HTTP/2 straight away without TLS$`, func() error { return nil })
	sc.Step(`^it sends "([A-Z]+) ([^"]+)" with ":authority (\S+)"(?: and ":scheme http")?$`, func(method, path, authority string) error {
		return w.sendHTTP2(method, path, authority)
	})
	sc.Step(`^the app sends "([A-Z]+) ([^"]+)" with ":authority (\S+)" over TLS to the proxy$`, w.sendHTTP2)
	sc.Step(`^the connection uses HTTP/2$`, func() error {
		if w.tlsState.NegotiatedProtocol != "h2" || w.resp == nil || w.resp.ProtoMajor != 2 {
			return failf("the connection agreed on %q and the app got %v", w.tlsState.NegotiatedProtocol, w.resp)
		}
		return nil
	})
}

// sendHTTP2 sends one HTTP/2 call to the proxy, over TLS when the app uses
// it and otherwise plain from the first byte, with :authority set.
func (w *world) sendHTTP2(method, path, authority string) error {
	if err := w.start(); err != nil {
		return err
	}
	host := strings.ToLower(authority)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if _, err := netip.ParseAddr(host); err != nil && !strings.HasPrefix(path, "/http") {
		w.vendor(host)
	}
	if strings.HasPrefix(path, "/http") {
		w.vendor(strings.ToLower(hostOf(path)))
	}
	protocols := new(http.Protocols)
	transport := &http.Transport{Protocols: protocols}
	scheme := "http"
	if w.appTLS != nil {
		protocols.SetHTTP1(true)
		protocols.SetHTTP2(true)
		transport.TLSClientConfig = w.appTLS
		scheme = "https"
	} else {
		protocols.SetUnencryptedHTTP2(true)
	}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, scheme+"://"+w.addr+path, nil)
	if err != nil {
		return err
	}
	req.Host = authority
	if w.appID != "" {
		req.Header.Set("Proxy-Authorization", strings.TrimPrefix(basic(w.appID+":x"), "Proxy-Authorization: "))
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
	w.resp, w.respErr = resp, err
	if err != nil {
		return nil
	}
	if resp.TLS != nil {
		w.tlsState = *resp.TLS
	}
	w.respBody, w.respErr = io.ReadAll(resp.Body)
	return resp.Body.Close()
}
