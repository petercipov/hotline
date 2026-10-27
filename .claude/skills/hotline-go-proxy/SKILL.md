---
name: hotline-go-proxy
description: How to write Go code for the Hotline proxy - layout, tests first with Ginkgo, standard library HTTP, OpenTelemetry names, errors, concurrency and the proxy's hard rules. Use when designing, writing, testing or reviewing any proxy code in this repo.
---

# Writing the Hotline proxy in Go

The proxy sits between a customer's apps and their vendors. It forwards each call, measures it, checks the bodies against the vendor's OpenAPI spec after the reply has gone back, and sends what it found as OpenTelemetry data.

**The spec is the source of truth:** `proxy-spec.md` in the `petercipov/hotline-strategic` repo. This skill says *how* to code it; the spec says *what* to build. If they disagree, the spec wins; say so and stop rather than guess.

## 1. Tests first, always

This repo works test-first (see `CLAUDE.md`):

1. Write a failing Ginkgo `It` that describes one behaviour in plain words.
2. Run `go test ./...` and see it fail for the right reason.
3. Write the least code that makes it pass, run again, then tidy up.

Rules for tests here:

- Ginkgo and Gomega only. No testify, no mocking libraries.
- Test packages are `<pkg>_test` (black box). Use `<pkg>_internal_test.go` only when an internal detail truly needs it.
- Coverage must stay at 100% (`make test && make cover`). Untestable code is a design smell: split it.
- Replace the outside world with small hand-written fakes behind interfaces, or real local servers from `net/http/httptest` (also with TLS, `httptest.NewTLSServer`). Never call the real internet in unit tests.
- Time and randomness come in through the constructor (a clock function, a reader), so tests stay deterministic.
- Each test name reads as a sentence about the behaviour, e.g. `It("redacts unmarked query values")`.
- End-to-end tests (Java, Node, Go and Rust clients through every door) are the first milestone's finish line. Keep them separate from the fast unit tests.

## 2. Dependencies

- **Standard library first.** `net/http`, `net/http/httputil`, `net/http/httptrace`, `crypto/tls`, `crypto/hmac`, `log/slog`, `context`, `sync`.
- Outside modules only where the standard library has nothing: the OpenTelemetry Go SDK and `semconv`, a YAML parser, an OpenAPI and JSON Schema checker. Name every new module and why in the pull request.
- No web frameworks (gin, echo, chi), no dependency-injection frameworks, no helper libraries for things a short loop does.
- The project is open source under Apache-2.0: every new module must have a compatible license (MIT, BSD, Apache-2.0).

## 3. Layout and packages

- Code lives under `src/hotline/` in module `hotline`. The program entry point goes in `src/hotline/cmd/hotline-proxy/`; it only reads flags, wires parts together and starts. All logic lives in packages it imports.
- One package per idea, named after what it provides, short and lower case: e.g. `config`, `door`, `redact`, `fingerprint`, `schema`, `record`, `export`, `admin`. Never `util`, `common` or `helpers`.
- Packages depend in one direction: the forwarding path knows nothing about exporters; it hands a finished record to a small interface.
- Keep interfaces small (one or two methods) and declare them where they are *used*, not where they are implemented. Return concrete types.
- No package-level variables except sentinel errors, and no `init()` (the linter enforces this). Everything is passed in through constructors.

## 4. Errors

- Define sentinel errors per package: `var ErrUnknownDoor = errors.New("door: unknown door")`. Add detail with `fmt.Errorf("%w: ...", ErrX, ...)`. Check with `errors.Is` or `errors.As`. The linter forbids ad hoc `errors.New` inside functions.
- Errors carry context, never secrets: no header values, keys, tokens or body bytes in error text or logs.
- Failures talking to the vendor become a `502` or `504` answer with an RFC 9209 `Proxy-Status` header (e.g. `hotline; error=dns_timeout`) and the same name in the record's `error.type`.
- Never `panic` on input from apps, vendors, config or specs. A bad config at reload keeps the old config and reports why.

## 5. The call path: hard rules

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

## 6. Concurrency

- Every goroutine has an owner and a way to stop: it ends when its `context` is cancelled, and the owner waits for it (`sync.WaitGroup`) on shutdown.
- Pass `context.Context` as the first argument; never store it in a struct.
- The vendor call uses its own context bound by `vendorTimeout`, not the app's request context: if the app gives up, keep waiting and set `hotline.client.cancelled=true`.
- Share config by swapping an immutable snapshot (`atomic.Pointer`), so calls in flight finish on the config they started with.
- Run tests with `-race` when touching concurrent code.

## 7. OpenTelemetry

- Use the standard names from `go.opentelemetry.io/otel/semconv` constants wherever one exists. Hotline-only fields use the `hotline.` prefix and match the spec's tables exactly.
- One log record per call (`event.name = hotline.request`), the `hotline.requests` counter, optional spans. Data for customers and the proxy's own health data go to separate exporters, never mixed.
- Use `log/slog` for the proxy's own logs.

## 8. Before you hand work back

- `go test ./...` passes, coverage is 100%, `make lint` is clean.
- New behaviour has a test that failed first.
- Any new module is named and justified.
- No secret or body value can reach a log, error or record.
