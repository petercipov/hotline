package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http/httptrace"
	"net/netip"
)

// dialer opens connections to vendors. It looks up the host itself, so the
// call's trace sees the DNS and connect phases whichever resolver is used.
type dialer struct {
	resolver Resolver
	dialer   Dialer
}

func (d *dialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	trace := httptrace.ContextClientTrace(ctx)
	host, port, _ := net.SplitHostPort(address)
	ip, err := netip.ParseAddr(host)
	ips := []netip.Addr{ip}
	if err != nil {
		trace.DNSStart(httptrace.DNSStartInfo{Host: host})
		ips, err = d.resolver.LookupNetIP(ctx, "ip", host)
		trace.DNSDone(httptrace.DNSDoneInfo{Err: err})
	}
	// Addresses are tried one after another, so their connect phases never
	// overlap.
	var conn net.Conn
	for i := 0; conn == nil && i < len(ips) && ctx.Err() == nil; i++ {
		address = net.JoinHostPort(ips[i].String(), port)
		trace.ConnectStart(network, address)
		conn, err = d.dialer.DialContext(ctx, network, address)
		trace.ConnectDone(network, address, err)
	}
	return conn, err
}

// isCertificateError says whether a TLS handshake failed on the vendor's
// certificate rather than on the protocol.
func isCertificateError(err error) bool {
	var cert *tls.CertificateVerificationError
	return errors.As(err, &cert)
}
