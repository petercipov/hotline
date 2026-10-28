# Project memory

Decisions already made, so a new session does not have to dig through old threads and pull requests. Keep each entry short and point to where the detail lives. Add to it when Peter decides something new.

## Where the rules live

- How to test and the TDD loop: [CLAUDE.md](CLAUDE.md).
- How to write, test and review proxy code in Go: the [`hotline-go-proxy` skill](.claude/skills/hotline-go-proxy/SKILL.md). It covers the system under test, testing only through the proxy's ways in and out, and no test-only code.
- The proxy's spec and examples: `proxy-spec.md` and `proxy-example/` in the `hotline-strategic` repo.

## Testing, in one line each

- Proxy behaviour: Gherkin feature files in `src/hotline/proxy/features/`, run by godog. Everything else: Ginkgo and Gomega.
- The whole proxy is the system under test (the thing a test drives and checks), with fake vendors and a fake OTLP receiver at its edges.
- Never change production code just so a test can reach it.

## Slice plan

The proxy is built in small slices, tests first.

1. **Slice 1: done** (pull request #11). Forward proxy ingress, TLS to the vendor, integration ID, phase timings, the request record with redaction, OTLP output, the proxy's own errors, config loaded once. Scope and detail: [docs/proxy-slice-1.md](docs/proxy-slice-1.md).
2. **The `hotline-proxy` program: built** (commands, flags, stopping, exit codes, `@e2e` tests). Detail: [docs/proxy-program.md](docs/proxy-program.md).
3. Still to come, roughly in this order:
   - the health checks `/livez` and `/readyz` on the admin port;
   - the Node, Java and Rust clients in the end-to-end tests;
   - TLS from apps to the proxy (`listen.tls`);
   - the other ingresses (the Host header, and the vendor URL in the path);
   - JSON and form body checks against the spec;
   - config reload without a restart;
   - the load test, with k6.

## Slice 1 decisions (Peter, 2026-10-28)

Full wording is at the end of [docs/proxy-slice-1.md](docs/proxy-slice-1.md).

1. Other requests on the proxy port (a plain path, a Host header request, `CONNECT`) get `501`. A full address the proxy can't use gets `400`.
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

## Earlier decisions

- The load test uses k6 (Peter, 2026-10-27).
