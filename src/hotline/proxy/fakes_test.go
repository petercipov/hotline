package proxy_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"time"
)

var errUnreachable = errors.New("fake network: no route to host")

// issuer is a test certificate authority: the one the proxy trusts.
type issuer struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newIssuer() *issuer {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "hotline test issuer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &issuer{cert: cert, key: key, pool: pool}
}

// leaf makes a server certificate for host. With signer nil it signs
// itself, so no issuer vouches for it.
func (i *issuer) leaf(host string, notAfter time.Time, selfSigned bool) tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	parent, signer := i.cert, i.key
	if selfSigned {
		parent, signer = tmpl, key
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signer)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// vendorCall is what a fake vendor received.
type vendorCall struct {
	scheme string
	port   string
	proto  string
	method string
	uri    string
	host   string
	header http.Header
	body   []byte
}

// vendor is a fake vendor: one host that answers on https port 443 and
// http port 80 of the fake network.
type vendor struct {
	host     string
	notAfter time.Time
	ip       netip.Addr
	servers  []*httptest.Server
	mu       sync.Mutex
	calls    []vendorCall
	answer   http.HandlerFunc
}

func (v *vendor) record(scheme, port string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := vendorCall{
			scheme: scheme, port: port, proto: r.Proto, method: r.Method,
			uri: r.RequestURI, host: r.Host, header: r.Header.Clone(),
		}
		v.mu.Lock()
		answer := v.answer
		v.mu.Unlock()
		if answer == nil {
			c.body, _ = io.ReadAll(r.Body)
		}
		v.mu.Lock()
		v.calls = append(v.calls, c)
		v.mu.Unlock()
		if answer != nil {
			answer(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-5521")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func (v *vendor) received() []vendorCall {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]vendorCall(nil), v.calls...)
}

func (v *vendor) setAnswer(h http.HandlerFunc) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.answer = h
}

func (v *vendor) close() {
	for _, s := range v.servers {
		s.CloseClientConnections()
		s.Close()
	}
}

// network is the fake network between the proxy and the vendors: its own
// DNS, and addresses that lead to local test servers.
type network struct {
	mu     sync.Mutex
	names  map[string][]netip.Addr
	routes map[string]string
	// hang lists addresses or names that never answer.
	hang map[string]bool
	// unreachable lists addresses with no route.
	unreachable map[string]bool
	lookups     int
	dials       int
	// lookupTime is how long each answered lookup takes.
	lookupTime time.Duration
	// stuck runs when a lookup or a connection hangs.
	stuck func()
}

func newNetwork() *network {
	return &network{
		names:       map[string][]netip.Addr{},
		routes:      map[string]string{},
		hang:        map[string]bool{},
		unreachable: map[string]bool{},
	}
}

func (n *network) LookupNetIP(ctx context.Context, _, host string) ([]netip.Addr, error) {
	n.mu.Lock()
	n.lookups++
	ips, found := n.names[host]
	hang := n.hang[host]
	lookupTime := n.lookupTime
	n.mu.Unlock()
	time.Sleep(lookupTime)
	if hang {
		n.stuck()
		<-ctx.Done()
		return nil, &net.DNSError{Err: "timeout", Name: host, IsTimeout: true}
	}
	if !found {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return ips, nil
}

func (n *network) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	n.mu.Lock()
	n.dials++
	local, found := n.routes[address]
	hang, unreachable := n.hang[address], n.unreachable[address]
	n.mu.Unlock()
	switch {
	case hang:
		n.stuck()
		<-ctx.Done()
		return nil, &net.OpError{Op: "dial", Net: network, Err: ctx.Err()}
	case unreachable || !found:
		return nil, &net.OpError{Op: "dial", Net: network, Err: errUnreachable}
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, local)
	if err != nil {
		return nil, err
	}
	remote, _ := net.ResolveTCPAddr("tcp", address)
	return &fakeConn{Conn: conn, remote: remote}, nil
}

func (n *network) route(address, local string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.routes[address] = local
}

func (n *network) name(host string, ip netip.Addr) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.names[host] = []netip.Addr{ip}
}

func (n *network) setHang(target string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.hang[target] = true
}

func (n *network) setUnreachable(address string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.unreachable[address] = true
}

func (n *network) counts() (int, int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lookups, n.dials
}

// fakeConn reports the address the proxy asked for, not the local test
// server it really reached.
type fakeConn struct {
	net.Conn
	remote net.Addr
}

func (c *fakeConn) RemoteAddr() net.Addr { return c.remote }

func listenLocal() net.Listener {
	l, _ := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	return l
}

func ipFor(n int) netip.Addr {
	return netip.MustParseAddr(fmt.Sprintf("192.0.2.%d", n))
}
