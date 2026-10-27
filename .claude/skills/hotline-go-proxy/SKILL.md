---
name: hotline-go-proxy
description: How to write Go code for the Hotline proxy - layout, tests first with Ginkgo, standard library HTTP, OpenTelemetry names, errors, concurrency and the proxy's hard rules. Use when designing, writing, testing or reviewing any proxy code in this repo.
---

# Writing the Hotline proxy in Go

The proxy sits between a customer's apps and their vendors. It forwards each call, measures it, checks the bodies against the vendor's OpenAPI spec after the reply has gone back, and sends what it found as OpenTelemetry data.

**The spec is the source of truth:** `proxy-spec.md` in the `petercipov/hotline-strategic` repo. This skill says *how* to code it; the spec says *what* to build. If they disagree, the spec wins; say so and stop rather than guess.

## 1. Tests first, always

This repo works test-first (see `CLAUDE.md`):

1. Before any code, list the test cases for the piece you are building: the normal cases, the edge cases and the failures, each as one plain sentence. Put them in the test file as pending specs (`PIt`) or in the plan, so they can be reviewed first.
2. Turn one case into a failing Ginkgo `It`.
3. Run `go test ./...` and see it fail for the right reason.
4. Write the least code that makes it pass, run again, then tidy up. Move to the next case.

Rules for tests here:

- Ginkgo and Gomega only. No testify, no mocking libraries.
- Test packages are `<pkg>_test` (black box). Use `<pkg>_internal_test.go` only when an internal detail truly needs it.
- There is no fixed coverage target. Aim for every listed case to have a test; code that is hard to test is a design smell, so split it.
- Replace the outside world with small hand-written fakes behind interfaces, or real local servers from `net/http/httptest` (also with TLS, `httptest.NewTLSServer`). Never call the real internet in unit tests.
- Time and randomness come in through the constructor (a clock function, a reader), so tests stay deterministic.
- Each test name reads as a sentence about the behaviour, e.g. `It("redacts unmarked query values")`.
- End-to-end tests (Java, Node, Go and Rust clients through every door) are the first milestone's finish line. Keep them separate from the fast unit tests.

## 2. Dependencies

- **Standard library first.** `net/http`, `net/http/httputil`, `net/http/httptrace`, `crypto/tls`, `crypto/hmac`, `log/slog`, `context`, `sync`.
- Outside modules only where the standard library has nothing: the OpenTelemetry Go SDK and `semconv`, a YAML parser, an OpenAPI and JSON Schema checker. Name every new module and why in the pull request.
- No web frameworks (gin, echo, chi), no dependency-injection frameworks, no helper libraries for things a short loop does.
- The project is open source under Apache-2.0: every new module must have a compatible license (MIT, BSD, Apache-2.0).

## 3. The parts of the proxy

The proxy is a small core with the outside world kept behind interfaces (often called *ports and adapters*: the core only talks to small interfaces, the "ports", and the code that touches real networks, files or clocks, the "adapters", plugs into them). That keeps every part testable with fakes.

**On the call path** (must stay fast; nothing here waits on checks or exports):

1. **Door finder:** works out which door the call used (forward proxy, Host header, vendor URL in path), reads and removes the integration ID, and picks the integration and api from the current config.
2. **Forwarder:** the standard library reverse proxy (`httputil.ReverseProxy` with its own `http.Transport`). It makes the call to the vendor, with TLS origination when set, and times each phase with `httptrace` (DNS, connect, TLS, first byte). On failure it answers with `502` or `504` and `Proxy-Status`.
3. **Body tap:** wraps the request and response bodies, passes every byte straight on, and keeps a copy of at most 1 MiB for later.

**After the reply has gone back:**

4. **Checker workers:** a fixed number of background workers fed by a bounded queue. Each takes a finished call and checks the bodies against the spec, redacts, makes the fingerprints and builds the record. If the queue is full, the call is recorded with `hotline.schema.skipped=overloaded` instead of waiting.
5. **Exporters:** one bounded queue and sender per destination. When a queue is full the oldest record is dropped and counted.

**Around the call:**

6. **Config loader:** reads and checks the config, specs and key files, watches them for changes, and swaps in a new immutable snapshot only when everything is valid. The same checks back `hotline-proxy validate`.
7. **Admin server:** `/livez` and `/readyz` on the admin port only.
8. **Own health data:** calls in flight, queue lengths, dropped records, config version, sent only to the `telemetry` exporter.

**Ports** (interfaces the core uses; each has a real adapter and a test fake): a record sink (exporters), a config source (files), a clock, and the vendor transport (`http.RoundTripper`). Add a new port only when a test needs to replace something real.

`main` builds the adapters, connects them to the core, starts everything, and on shutdown stops in reverse order: stop accepting calls, let calls in flight finish, drain the checker queue, flush the exporters, then exit.

## 4. Layout and packages

- Code lives under `src/hotline/` in module `hotline`. The program entry point goes in `src/hotline/cmd/hotline-proxy/`; it only reads flags, wires parts together and starts. All logic lives in packages it imports.
- One package per idea, named after what it provides, short and lower case: e.g. `door`, `forward`, `tap`, `check`, `redact`, `fingerprint`, `record`, `export`, `config`, `admin`. Never `util`, `common` or `helpers`.
- Packages depend in one direction: the forwarding path knows nothing about exporters; it hands a finished record to a small interface.
- Keep interfaces small (one or two methods) and declare them where they are *used*, not where they are implemented. Return concrete types.
- No package-level variables except sentinel errors, and no `init()` (the linter enforces this). Everything is passed in through constructors.

## 5. Errors

- Define sentinel errors per package: `var ErrUnknownDoor = errors.New("door: unknown door")`. Add detail with `fmt.Errorf("%w: ...", ErrX, ...)`. Check with `errors.Is` or `errors.As`. The linter forbids ad hoc `errors.New` inside functions.
- Errors carry context, never secrets: no header values, keys, tokens or body bytes in error text or logs.
- Failures talking to the vendor become a `502` or `504` answer with an RFC 9209 `Proxy-Status` header (e.g. `hotline; error=dns_timeout`) and the same name in the record's `error.type`.
- Never `panic` on input from apps, vendors, config or specs. A bad config at reload keeps the old config and reports why.

## 6. The call path: hard rules

These protect the customer's traffic. Breaking one is a bug even if tests pass.

- **Never slow or change the call.** Bodies stream through untouched. Body checks, fingerprints and exports run *after* the reply has gone back, off the call's goroutine.
- **Body copy is capped at 1 MiB** per call. Above it, the call passes and the record says `hotline.schema.skipped=body_too_large`.
- **Under load, skip, don't wait.** If checks fall behind, skip them (`hotline.schema.skipped=overloaded`). Export queues are bounded (default 10 000); when full, drop the oldest and count it. Never block on a full queue.
- **No retries.** The app's own retry logic is in charge.
- **Streaming and WebSockets:** pass each piece on at once (flush), never buffer the whole reply.
- **Always check the vendor's TLS certificate.** No setting turns it off.
- **Remove `Proxy-Authorization`** before calling the vendor. `Authorization`, cookies and secrets are never recorded.
- **Admin endpoints only on the admin port**, never on the proxy port.
- **Measure, don't judge.** The proxy records facts; good or bad is decided by the control plane.

## 7. Concurrency

- Every goroutine has an owner and a way to stop: it ends when its `context` is cancelled, and the owner waits for it (`sync.WaitGroup`) on shutdown.
- Pass `context.Context` as the first argument; never store it in a struct.
- The vendor call uses its own context bound by `vendorTimeout`, not the app's request context: if the app gives up, keep waiting and set `hotline.client.cancelled=true`.
- Share config by swapping an immutable snapshot (`atomic.Pointer`), so calls in flight finish on the config they started with.
- Pick the simplest tool: a channel to hand work to another goroutine, a mutex to guard shared fields, `sync/atomic` for counters and flags.
- Only the sender closes a channel. Give channel parameters a direction (`chan<-` or `<-chan`).
- Every `select` that waits also watches `ctx.Done()`, so nothing hangs after shutdown.
- Never start an unbounded number of goroutines per call; the checker workers are a fixed pool.
- In loops that wait repeatedly, reuse one `time.Timer` instead of calling `time.After` each time.
- Run tests with `-race` when touching concurrent code, and check that no goroutines are left running after a test with `go.uber.org/goleak` (`goleak.VerifyNone` in an `AfterEach`, or `goleak.VerifyTestMain`).
- Test timing through the injected clock, not with real sleeps.

## 8. OpenTelemetry

- Use the standard names from `go.opentelemetry.io/otel/semconv` constants wherever one exists. Hotline-only fields use the `hotline.` prefix and match the spec's tables exactly.
- One log record per call (`event.name = hotline.request`), the `hotline.requests` counter, optional spans. Data for customers and the proxy's own health data go to separate exporters, never mixed.
- Use `log/slog` for the proxy's own logs. When tracing is on, route it through the OpenTelemetry slog bridge (`go.opentelemetry.io/contrib/bridges/otelslog`) so log lines carry the trace and span IDs.

## 9. Before you hand work back

- `go test ./...` passes and `make lint` is clean.
- The test cases were listed first, and each new behaviour has a test that failed first.
- Any new module is named and justified.
- No secret or body value can reach a log, error or record.

## Credits

Parts of sections 3 and 7 (ports and adapters, goroutine and channel rules, goroutine leak checks, the slog bridge) are adapted, in our own words, from [samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) by Samuel Berthe (MIT License).
