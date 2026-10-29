# The Host header ingress, with HTTP/2 from apps: scope and test cases

*Requirements agreed with Peter, 2026-10-29. Not built yet; building starts only when Peter says so. The source of truth is `proxy-spec.md` in the `hotline-strategic` repo.*

## What the feature is

An ingress is a way an app reaches the proxy. Two are built: the forward proxy ingress (the full vendor address in the request line) and the vendor URL in path ingress (`/https://api.stripe.com/…` after the proxy's address). This slice adds the **Host header ingress**: the app calls the proxy with a plain path and names the vendor in the `Host` header.

```http
POST /v1/payment_intents HTTP/1.1
Host: api.stripe.com
Proxy-Authorization: Basic dGVzdC1zaG9wOng=
```

The app's own address for the proxy (`http://localhost:8080`) only says how the app talks to the proxy. A normal HTTP request never sends a scheme, so the proxy has to pick `http` or `https` for the vendor.

**HTTP/2 from apps comes with it.** An HTTP/2 call has no request line: it carries `:authority` (the vendor's name, playing the part of `Host`), `:path` and `:scheme`. So an HTTP/2 call is a Host header call, or a vendor URL in path call when `:path` starts with `/http://` or `/https://`. There is no forward proxy ingress over HTTP/2.

## Requirements

1. **Which calls belong to it.** Anything on the proxy port that is neither a full address in the request line, nor a path starting `/http:` or `/https:`, nor CONNECT.
2. **The vendor is the `Host`.** `Host: api.x.com:8443` goes to port 8443; with no port, 443 for `https` and 80 for `http`. The vendor gets the `Host` the app sent, since it is already the vendor's name. A missing or empty `Host` gets `400` with `Proxy-Status: hotline; error=http_request_error`.
3. **`https` by default.** A call with a known integration goes out over `https`, unless its api sets `tlsOrigination: false`, which sends it as plain `http` (egress-proxy-options, 2026-10-27). A `not-passed` call, one without a known integration, also goes out over `https` (Peter, 2026-10-29).
4. **HTTP/2 is `https` only** (2026-10-05). `:scheme http` never makes a plain call to the vendor.
5. **HTTP/2 over TLS to the proxy** is switched on with this slice (TLS from apps, decision 7). The proxy offers both HTTP/2 and HTTP/1.1 during the TLS setup.
6. **Plain HTTP/2 (without TLS, called h2c) by prior knowledge only** (Peter, 2026-10-29). Prior knowledge means the app starts talking HTTP/2 straight away. The older `Upgrade: h2c` handshake, dropped from the current standard (RFC 9113), is not supported: such a request is answered over HTTP/1.1 like any other.
7. **The same steps as the other ingresses.** The integration ID comes from `Proxy-Authorization` (added as a default header by the app), which is always removed; the proxy adds no forwarding headers; the record is the same, with `hotline.ingress=host_header`; refused calls get a minimal record; the vendor IP is in every record.
8. **Guarded from the start** (Peter, 2026-10-29). The internal address guard (`proxy-internal-guard.md`) applies. A call whose `Host` is the proxy itself goes to a loopback or cluster-internal IP, so it gets `403` and cannot loop.
9. **HTTP version in the record.** `network.protocol.version` is the version used to the vendor, as today.
10. **No HTTP/3 for now** (Peter, 2026-10-29). HTTP/3 runs over QUIC (on UDP) and gains little between an app and a proxy in the same cluster, and server side clients rarely use it to call a proxy. It is added later only if a real app needs it.

## Tests

- Feature scenarios: `src/hotline/proxy/features/host-header.feature`, tagged `@later` until building starts.
- When built, the `501` rows for Host header calls in `ingress.feature` and `url-in-path.feature` are replaced by these scenarios.
- End to end: the Go client calls with `Host` set, over plain HTTP/1.1, over plain HTTP/2 and over HTTP/2 with TLS.

## Not in this slice

The `Upgrade: h2c` handshake; HTTP/3; the transparent redirect ingress; CONNECT; gRPC; caller authentication.
