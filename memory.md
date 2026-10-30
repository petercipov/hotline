# Project memory

Decisions already made, so a new session does not have to dig through old threads and pull requests. Keep each entry short and point to where the detail lives. Add to it when Peter decides something new.

## Where the rules live

- How to test and the TDD loop: [CLAUDE.md](CLAUDE.md).
- How to write, test and review proxy code in Go: the [`hotline-go-proxy` skill](.claude/skills/hotline-go-proxy/SKILL.md). It covers the system under test, testing only through the proxy's ways in and out, and no test-only code.
- The proxy's spec and examples: `proxy-spec.md` and `proxy-example/` in the `hotline-strategic` repo.
- What the proxy sends over OpenTelemetry, field by field: [docs/otel-export.md](docs/otel-export.md).

## Code layout

- The proxy is its own Go module, `hotline-proxy`, in `src/hotline-proxy/`: the `proxy` package at the top, then `cmd/` (the program), `features/`, `internal/` and `testdata/`. The metrics code stays in module `hotline`, in `src/hotline/metrics/`. Both are listed in `go.work` (Peter, 2026-10-30).

## Testing, in one line each

- Proxy behaviour: Gherkin feature files in `src/hotline-proxy/features/`, run by godog. Everything else: Ginkgo and Gomega.
- The whole proxy is the system under test (the thing a test drives and checks), with fake vendors and a fake OTLP receiver at its edges.
- Never change production code just so a test can reach it.

## Slice plan

The proxy is built in small slices, tests first.

1. **Slice 1: done** (pull request #11). Forward proxy ingress, TLS to the vendor, integration ID, phase timings, the request record with redaction, OTLP output, the proxy's own errors, config loaded once. Scope and detail: [docs/proxy-slice-1.md](docs/proxy-slice-1.md).
2. **The `hotline-proxy` program: built** (commands, flags, stopping, exit codes, `@e2e` tests). Detail: [docs/proxy-program.md](docs/proxy-program.md).
3. **TLS from apps to the proxy: built** (`listen.tls`, renewal without a restart, Go cases 4 and 5 end to end). Detail: [docs/proxy-tls-from-apps.md](docs/proxy-tls-from-apps.md).
4. Still to come, roughly in this order:
   - the health checks `/livez` and `/readyz` on the admin port;
   - the Node, Java and Rust clients in the end-to-end tests;
   - the vendor URL in path ingress: built, with refused calls recorded on every ingress. Detail: [docs/proxy-url-in-path.md](docs/proxy-url-in-path.md);
   - the guard against calls to internal addresses: built. Detail: [docs/proxy-internal-guard.md](docs/proxy-internal-guard.md);
   - the Host header ingress, with HTTP/2 from apps: built. Detail: [docs/proxy-host-header.md](docs/proxy-host-header.md);
   - each vendor api named once in the config, shared by integrations: built. Detail: [docs/proxy-spec-checks.md](docs/proxy-spec-checks.md), decisions 10 and 11;
   - checks of every part of a call against the spec (path, query, headers, cookies, body; and the reply's status, headers and body). Built. Detail: [docs/proxy-spec-checks.md](docs/proxy-spec-checks.md);
   - the transparent redirect ingress: built, as a Host header call with no proxy code change. Detail: [docs/proxy-transparent-redirect.md](docs/proxy-transparent-redirect.md);
   - AWS, starting with SQS: signed calls stay valid, actions as routes named by a header. Agreed, not built. Detail: [docs/proxy-aws.md](docs/proxy-aws.md);
   - the CONNECT ingress, with interception: the proxy poses as the vendor inside the tunnel with certificates signed by the customer's issuer. Built. Detail: [docs/proxy-connect.md](docs/proxy-connect.md);
   - OTLP over HTTP, and TLS, login headers and gzip for both gRPC and HTTP: built. Detail: [docs/proxy-otlp-protocols.md](docs/proxy-otlp-protocols.md);
   - config reload without a restart;
   - the load test, with k6.

## Slice 1 decisions (Peter, 2026-10-28)

Full wording is at the end of [docs/proxy-slice-1.md](docs/proxy-slice-1.md).

1. Other requests on the proxy port (a plain path, a Host header request, `CONNECT`) get `501`. `CONNECT` is an ingress since 2026-10-29, when `listen.connect` is set. A full address the proxy can't use gets `400`.
2. A broken `Proxy-Authorization` header counts as no integration ID: `not-passed`.
3. A path with no route in the spec: `url.full` and `url.template` are left out.
4. Turning `http` into `https` keeps the app's port.
5. `not-passed` calls that use `https` stay `https`.
6. The proxy adds no forwarding headers.
7. The config file needs a `version` field. A config without it is refused.
8. Config checks are lenient: unknown fields are accepted, with a warning log line each.
9. Route matching ignores a trailing slash, and puts the path of the first `servers` address in front of every route.
10. Each timing is its own phase, except `first_byte`, counted from arrival. The proxy's own time is split into `hotline.timing.proxy_request` and `hotline.timing.proxy_response`.
11. Empty query values like `customer=` stay as they are.
12. Fields not built yet (schema, fingerprints) are left out of the record.
13. Dropped records: a warning at most once a minute per destination, plus one at shutdown. Queues hold 10 000 records unless `queueSize` says otherwise.
14. TLS from apps to the proxy waits for a later slice.

Also from the review of pull request #11: `vendorTimeout` also limits how long a reply may go silent between its parts. A vendor silent for longer is cut off, recorded as `http_response_incomplete`. WebSockets are exempt.

## Program decisions (Peter, 2026-10-28)

Full list in [docs/proxy-program.md](docs/proxy-program.md).

1. Commands `run`, `validate` and `version`. The config comes only from `--config`, with no default.
2. Own logs: JSON, one object per line, on standard error, with `--log-level`.
3. Stopping: up to 25 seconds (`--shutdown-timeout`); a second signal cuts it short.
4. Exit codes: 0 clean, 1 any failure (also a stop that ran out of time), 2 wrong command line.
5. `--trust-file` adds trusted issuers on top of the system's own, so the end-to-end tests can trust their test vendor on any computer. This changed the spec's vendor certificate rule.
6. End-to-end tests use only the Go client for now. They run with `make e2e`, and `make test` adds their coverage to the 100% check.

## TLS from apps decisions (Peter, 2026-10-28)

Full wording in [docs/proxy-tls-from-apps.md](docs/proxy-tls-from-apps.md).

1. With `listen.tls` set, the proxy port accepts only TLS.
2. Certificate and key are PEM files; on Kubernetes a Secret, usually from cert-manager.
3. Hotline builds nothing for apps to trust the proxy and never makes its own certificate; customers use their own issuer.
4. The files are re-read when they change; a broken new pair keeps the old one.
5. Client certificates (mutual TLS) come later, with caller authentication.
6. TLS 1.2 and 1.3 only, Go's default ciphers, nothing to configure.
7. Only HTTP/1.1 over TLS for now; HTTP/2 comes with the Host header ingress.
8. No record field for it; the proxy's own health data says it once. Changed 2026-10-29: the TLS setup time is recorded as `hotline.timing.app_tls`.
9. A missing, broken, mismatched or expired certificate stops the proxy at start. A failed app TLS setup is a debug log line and, later, a count in health data.
10. The warning before the proxy's own certificate expires comes with the health data.

## Vendor URL in path decisions (Peter, 2026-10-29)

Full wording in [docs/proxy-url-in-path.md](docs/proxy-url-in-path.md).

1. Two slices: vendor URL in path first, over HTTP/1.1; then the Host header ingress with HTTP/2 from apps.
2. Only the clean `/http://` or `/https://` form. A squashed slash or an encoded address gets `400`.
3. The vendor gets its own host (and port) from the path in `Host`, never the proxy's.
4. Every request the proxy reads and refuses (`400`, `501`) still gets a minimal record and is counted, on every ingress. Plain HTTP on a TLS port stays without a record.
5. Every record of a call that reached a vendor holds the vendor IP the proxy connected to (`network.peer.address`), minimal records too.
6. No loop guard in this slice; the loop question moves to the Host header slice.

## Internal address guard decisions (Peter, 2026-10-29)

Full wording in [docs/proxy-internal-guard.md](docs/proxy-internal-guard.md).

1. Guard against server side request forgery: after DNS, the proxy refuses to connect to internal IPs (loopback, link-local with the cloud metadata address, private, shared, unspecified, multicast), on every ingress.
2. An api can set `allowInternalIPs: true`; so can the `not-passed` entry. Calls without an ID still reach public hosts.
3. Its own slice, before the Host header ingress. It also stops calls to the proxy itself, so there is no separate loop check.

## Earlier decisions

- Say "ingress", never "door", for the ways apps reach the proxy (Peter, 2026-10-29).
- The load test uses k6 (Peter, 2026-10-27).

## Host header ingress decisions (Peter, 2026-10-29)

Full wording in [docs/proxy-host-header.md](docs/proxy-host-header.md).

1. Calls without a known integration (`not-passed`) go out over `https`, since a Host header call names no scheme.
2. Plain HTTP/2 (h2c) by prior knowledge only; no `Upgrade: h2c` handshake.
3. Guarded from the start by the internal address guard; no separate loop check.
4. No HTTP/3 for now; added later only if a real app needs it.

## Spec checks and shared apis decisions (Peter, 2026-10-29)

Full wording in [docs/proxy-spec-checks.md](docs/proxy-spec-checks.md).

1. Every part of a call is checked against the spec, not only the body: path, query, headers, cookies and body; in the reply, status, headers and body.
2. Unknown query values are reported; unknown headers are ignored; extra body fields follow `additionalProperties`.
3. Each result is structured: `part`, `place`, `rule`, `expected`. Never a value from the call.
4. A content type the spec doesn't list, or a body where none is expected, is reported.
5. A reply status the spec doesn't list (no `default`) is reported, and that reply is not checked further.
6. At most 20 results per side, plus `errors_total` when more.
7. Common formats are checked (`date-time`, `date`, `email`, `uuid`, `uri`, `ipv4`, `ipv6`, `int32`, `int64`).
8. The login key from the security section must be present; its value is never read.
9. Only OpenAPI 3.1; `$ref` within the same file; OpenAPI query styles; Stripe's bracket form bodies.
10. A broken spec is a config problem: refused at start, by `validate` and on reload.
11. One spec is read once and shared by every integration that uses it.
12. The config names each vendor api once in a top-level `apis` section; integrations name the apis they use, and may change only `vendorTimeout`. `not-passed` keeps its settings on itself.

## Transparent redirect decisions (Peter, 2026-10-29)

Full wording in [docs/proxy-transparent-redirect.md](docs/proxy-transparent-redirect.md).

1. The proxy doesn't care how the cluster redirects; the setup guide covers name lookup (`hostAliases`) and network rules in the pod; the tests fake name lookup.
2. The app adds `Proxy-Authorization` itself as a default header; no new way to pass the integration ID.
3. No separate port or record value: a redirected call is a Host header call, `hotline.ingress=host_header`.
4. The port is always kept, port 80 too (`Host: api.x.com:80` goes to `https` on port 80); no port means 443.

## CONNECT ingress decisions (Peter, 2026-10-29)

Full wording in [docs/proxy-connect.md](docs/proxy-connect.md). Why: a proxy deployed apart from the apps needs the hop to it encrypted, with no change in the app.

1. The issuer comes from the customer: `listen.connect: { cert, key }`, re-read on change. Without it, `CONNECT` gets `501`.
2. Hotline builds nothing for apps to trust the issuer; the setup guide shows how per language.
3. Every host is intercepted; pinned hosts go in `NO_PROXY`. No blind tunnel.
4. The integration ID comes from `Proxy-Authorization` on the `CONNECT`, for the whole tunnel; one inside is ignored. `CONNECT` works on plain and TLS ports.
5. Host certificates: made on first use, memory only, 24 hours.
6. HTTP/2 and HTTP/1.1 inside the tunnel. `CONNECT` over HTTP/2 works too: the tunnel is that `CONNECT`'s own stream, so many tunnels share one connection (Peter, 2026-10-30).
7. Calls go to the `CONNECT` host and port; another host gets `421`.
8. One record per call, `hotline.ingress=connect`; `hotline.timing.app_tls` on the first call of each connection; an app that doesn't trust the issuer gets a minimal record with `tls_untrusted_by_app`.
9. `hotline.timing.app_tls` also on TLS from apps (changes TLS from apps decision 8).
10. The issuer is made only for Hotline; files that are not an issuer stop the start; over a year gets a warning.
11. Plain HTTP inside a tunnel is allowed, handled like a forward proxy `http` call.

Found while building: Go's client offers HTTP/2 to an `https` proxy address and then speaks HTTP/1.1, so the proxy sets up TLS with apps itself and picks the HTTP version from the app's first bytes.

## AWS and SQS decisions (Peter, 2026-10-29)

Full wording in [docs/proxy-aws.md](docs/proxy-aws.md).

1. Signed calls stay valid: the proxy forwards `Host`, the path, the body, `Content-Length` and the app's headers unchanged; a rule with a test.
2. The integration ID stays in `Proxy-Authorization`. The proxy never reads `Authorization` (security). No port per integration.
3. Each AWS action is its own route in a spec we generate: `/#X-Amz-Target=AmazonSQS.SendMessage`.
4. The action header is named in our spec (`x-hotline-action-header`), per api, never per integration or in the proxy's code.
5. The test uses a signed request fixture made once; Hotline never depends on the AWS SDK.
6. AWS headers to record are marked in our SQS spec; the default header list is unchanged. No patterns in `hosts`.
7. The setup guide recommends the forward proxy for AWS, with the app's HTTP client turning `https` into `http` after signing, says vendor URL in path can't work for signed hosts, and warns about `NO_PROXY` and VPC endpoints.
