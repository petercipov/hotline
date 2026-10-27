# Proxy slice 1: scope and test cases

The proxy is built in small slices, tests first. This page says what slice 1 covers, what waits for later, and which parts of the spec need an answer before building starts. The source of truth is `proxy-spec.md` and `proxy-example/` in the `hotline-strategic` repo.

Right now slice 1 is only test cases: pending Ginkgo specs (`PIt`, tests written down but not run yet) under `src/hotline/proxy/`. There is no production code yet. Each pending case becomes a real failing test, then code, once Peter has reviewed the list.

## What slice 1 covers

- **Forward proxy door only.** The app puts the full vendor address in the request line (`PUT https://api.talon.one/...`), with `http` or `https`.
- **TLS origination** (the proxy makes the `https` call to the vendor itself): on by default, off per api with `tlsOrigination: false`.
- **Integration ID** from `Proxy-Authorization: Basic`, the user name part. A missing or unknown ID, or a host outside the integration's apis, uses the reserved `not-passed` settings.
- **Phase timings:** DNS, connect, TLS, first byte and total, plus the vendor IP and whether the connection was reused.
- **The request record**, with redaction driven by the `x-hotline-redact` marks in our copy of the vendor's OpenAPI spec.
- **OpenTelemetry output** over OTLP: one log record per call and the `hotline.requests` counter, with the standard OpenTelemetry names and `hotline.*` for the rest.
- **The proxy's own errors:** `502` or `504` with a `Proxy-Status` header (RFC 9209, the standard for proxy error answers).
- **Loading the config file** once at start.

The proxy judges nothing. Verdicts, thresholds and SLAs live in the control plane.

## Where the test cases are

| Package | What it tests |
|---|---|
| `proxy/config` | loading and checking `hotline-proxy.yaml` |
| `proxy/spec` | reading route templates, marks and secrets from the vendor spec |
| `proxy/door` | finding the door and reading the integration ID |
| `proxy/forward` | the call to the vendor, certificates, the proxy's own errors |
| `proxy/timing` | phase timings |
| `proxy/redact` | redacting the address and choosing headers |
| `proxy/record` | building the request record |
| `proxy/export` | sending logs and the counter over OTLP |
| `proxy/e2e` | real clients through the real proxy |

Run them with `go test ./src/hotline/proxy/...`. Pending specs show as pending and do not fail the run.

## End-to-end checks

The end-to-end tests reuse the client matrix from the research folder: `research/matrix-clients/go`, `research/matrix-clients/node` and the prototype proxy in `research/hotline-matrix-test`. The case numbers are those of the matrix in `egress-proxy-options.md`. Only cases 1, 2, 4 and 5 put a full address in the request line, so only they belong to slice 1. The rest arrive with the Host header door.

Unlike the research runs, these tests never call the real Stripe. They use a local TLS test vendor whose certificate issuer is trusted through `SSL_CERT_FILE` (an environment variable Go reads on Linux to find trusted issuers), and a local fake OTLP receiver. The Java and Rust clients from the first milestone come in a later slice.

## Left for later slices

- The Host header door and the vendor URL in path door (and with them HTTP/2 from apps, matrix cases 3 and 6 to 10).
- JSON and form body checks against the spec, and the `hotline.schema.*` fields.
- Body fingerprints (`hotline.body.*.hmac_sha256`) and the `fingerprintKey` files.
- Config reload without a restart, and `hotline-proxy validate`.
- The admin port with `/livez` and `/readyz`.
- The proxy's own health data on the `telemetry` exporter.
- Streaming replies, WebSockets and the body size limit.
- Trace spans.
- The load test, which uses **k6** (Grafana's open source load testing tool, scripted in JavaScript), as Peter decided on 2026-10-27.
- The container image.

## Open questions

The test names point here with Q1 to Q14. Each needs an answer before its test is written.

1. **Other requests on the proxy port.** Until the other doors exist, what should a path like `/https://...`, a plain path with a Host header, or a `CONNECT` get? An error answer with `Proxy-Status`, and which status code?
2. **Broken `Proxy-Authorization`.** A scheme other than Basic, bad base64, no colon, an empty user name, or two such headers. Treat all as "no ID sent" and record `not-passed`?
3. **A path with no route in the spec.** Without a route the proxy can't tell which segments are values. Redact every segment of `url.full`, and leave `url.template` out?
4. **Turning `http` into `https`.** For `http://api.x.com/...` the proxy calls port 443. For `http://api.x.com:8080/...`, keep 8080 or use 443?
5. **`not-passed` and `https`.** "Exactly as sent" with no TLS origination: an `https://` request line still goes out as `https`, right?
6. **Forwarding headers.** Should the proxy add `X-Forwarded-For` or `Via` to the vendor call? The tests assume no, to keep the call unchanged.
7. **Config version.** What is `hotline.config.version`: a hash of the config, specs and keys, or a counter?
8. **Strict config.** Refuse unknown fields (catches typos)? Refuse a `not-passed` entry that has an `apis` list?
9. **Matching routes.** How to treat a trailing slash, and a base path in the spec's `servers` address (e.g. `https://api.x.com/v1`)?
10. **Timing definitions.** Is each timing the length of its own phase, or the time since the call started? Is `first_byte` measured from the start of the call or from when the request was sent? And is `hotline.timing.proxy` the total minus the vendor's part?
11. **Empty query values.** For `?customer=`, write `customer=` or `customer=REDACTED`?
12. **Fields not built yet.** Should slice 1 leave the `hotline.schema.*` and fingerprint fields out, or set `hotline.schema.skipped` to a value such as `not_checked`?
13. **Dropped records.** The spec counts dropped records in the proxy's own health data, which is a later slice. Count them in a log line until then?
14. **TLS from apps to the proxy.** Is the optional `listen.tls` in slice 1? Matrix cases 4 and 5 need it.
