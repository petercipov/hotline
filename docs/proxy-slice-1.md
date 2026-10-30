# Proxy slice 1: scope and test cases

The proxy is built in small slices, tests first. This page says what slice 1 covers, what waits for later, and which parts of the spec need an answer before building starts. The source of truth is `proxy-spec.md` and `proxy-example/` in the `hotline-strategic` repo.

Slice 1 is built. The test cases are Gherkin feature files (plain-language cases in Given, When, Then form) in `src/hotline-proxy/features/`, run by godog, and the code in `src/hotline-proxy/` makes them pass. Peter answered the 14 open questions on 2026-10-28; the answers are listed at the end of this page.

## What slice 1 covers

- **Forward proxy ingress only.** The app puts the full vendor address in the request line (`PUT https://api.talon.one/...`), with `http` or `https`.
- **TLS origination** (the proxy makes the `https` call to the vendor itself): on by default, off per api with `tlsOrigination: false`.
- **Integration ID** from `Proxy-Authorization: Basic`, the user name part. A missing or unknown ID, or a host outside the integration's apis, uses the reserved `not-passed` settings.
- **Phase timings:** DNS, connect, TLS, first byte and total, the proxy's own time before and after the vendor call, plus the vendor IP and whether the connection was reused.
- **The request record**, with redaction driven by the `x-hotline-redact` marks in our copy of the vendor's OpenAPI spec.
- **OpenTelemetry output** over OTLP: one log record per call and the `hotline.requests` counter, with the standard OpenTelemetry names and `hotline.*` for the rest.
- **The proxy's own errors:** `502` or `504` with a `Proxy-Status` header (RFC 9209, the standard for proxy error answers).
- **Loading the config file** once at start.

The proxy judges nothing. Verdicts, thresholds and SLAs live in the control plane.

## Where the test cases are

| File | What it describes |
|---|---|
| `config.feature` | loading and checking `hotline-proxy.yaml` |
| `spec.feature` | reading route templates, marks and secrets from the vendor spec |
| `ingress.feature` | finding the ingress |
| `integration-id.feature` | reading the integration ID |
| `forward.feature` | the call to the vendor, certificates, the proxy's own errors |
| `timing.feature` | phase timings |
| `redaction.feature` | redacting the vendor address |
| `headers.feature` | choosing headers for the record |
| `record.feature` | building the request record |
| `export.feature` | sending logs and the counter over OTLP |
| `end-to-end.feature` | real clients through the real proxy, tagged `@e2e` |

Run them with `go test ./src/hotline-proxy/`. Scenarios tagged `@later` are skipped for now, and those tagged `@e2e` run against the built program with `make e2e` (see [proxy-program.md](proxy-program.md)). The proxy as a whole is the system under test: scenarios drive it the way an app does, with fake vendors and a fake OTLP receiver at its edges (see the `hotline-go-proxy` skill).

## End-to-end checks

The end-to-end tests reuse the client matrix from the research folder: `research/matrix-clients/go`, `research/matrix-clients/node` and the prototype proxy in `research/hotline-matrix-test`. The case numbers are those of the matrix in `egress-proxy-options.md`. Only cases 1, 2, 4 and 5 put a full address in the request line, so only they belong to slice 1. The rest arrive with the Host header ingress.

Unlike the research runs, these tests never call the real Stripe. They use a local TLS test vendor whose certificate issuer is trusted through `SSL_CERT_FILE` (an environment variable Go reads on Linux to find trusted issuers), and a local fake OTLP receiver. The Java and Rust clients from the first milestone come in a later slice.

## Left for later slices

- The Host header ingress and the vendor URL in path ingress (and with them HTTP/2 from apps, matrix cases 3 and 6 to 10).
- JSON and form body checks against the spec, and the `hotline.schema.*` fields.
- Body fingerprints (`hotline.body.*.hmac_sha256`) and the `fingerprintKey` files.
- Config reload without a restart, and `hotline-proxy validate`.
- The admin port with `/livez` and `/readyz`.
- The proxy's own health data on the `telemetry` exporter.
- Streaming replies, WebSockets and the body size limit.
- Trace spans.
- TLS from apps to the proxy (`listen.tls`), which matrix cases 4 and 5 need.
- The `hotline-proxy` program itself (a `main` package with flags), and the `@e2e` scenarios that run it.
- The load test, which uses **k6** (Grafana's open source load testing tool, scripted in JavaScript), as Peter decided on 2026-10-27.
- The container image.

## Answers to the open questions

Peter answered these on 2026-10-28. The feature files and the code follow them.

1. **Other requests on the proxy port.** A path, a Host header request or `CONNECT` gets `501` with `Proxy-Status: hotline; error=http_request_error`. A full address the proxy can't use, such as `ftp://` or one with no host, gets `400`.
2. **Broken `Proxy-Authorization`.** Every broken form counts as no ID, so the record says `not-passed`.
3. **A path with no route in the spec.** `url.full` and `url.template` are left out.
4. **Turning `http` into `https`.** The app's own port stays: `http://api.x.com:8080` goes out as `https` on 8080.
5. **`not-passed` and `https`.** An `https` call stays `https`.
6. **Forwarding headers.** The proxy adds none.
7. **Config version.** A `version` field in the config file, written by whoever changes it. A config without it is refused.
8. **Strict config.** Lenient: unknown fields and an `apis` list on `not-passed` are accepted, and each gets a warning log line.
9. **Matching routes.** A trailing slash is ignored. The path of the first `servers` address goes in front of every route.
10. **Timing definitions.** Each timing is the length of its own phase, except `first_byte`, which counts from when the call reached the proxy. The proxy's own time is split in two: `hotline.timing.proxy_request` (before the vendor call starts) and `hotline.timing.proxy_response` (after the vendor's reply ends). Together they are the total minus the vendor's part.
11. **Empty query values.** `customer=` stays as it is.
12. **Fields not built yet.** The schema and fingerprint fields are left out.
13. **Dropped records.** A warning log line, at most once a minute per destination, and always one on the final send at shutdown. Each exporter's queue holds 10 000 records unless `queueSize` says otherwise.
14. **TLS from apps to the proxy.** A later slice. Its scenario is tagged `@later`.

One more answer came from the review of the pull request (Peter, 2026-10-28): `vendorTimeout` also limits how long a reply may go quiet between its parts. A vendor that stays silent longer is cut off, and the record says `http_response_incomplete`. A WebSocket may stay quiet as long as it likes.
