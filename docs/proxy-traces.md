# Each call's timings as a trace

Agreed with Peter on 2026-10-31. Tests: `src/hotline-proxy/features/timing.feature`, and the trace part of `signature.feature`.

A trace is a set of spans; a span is one step with a start time and an end time. The timings of a call used to be a few number attributes on its record (`hotline.timing.*_us`). They are a trace now: every call gets one, sent over OTLP to every exporter next to its record, so Grafana Cloud stores it in Tempo and draws the call as a timeline (Peter, 2026-10-31).

## The spans of one call

| Span | Kind | From | To |
|---|---|---|---|
| `hotline call` | server | the call reached the proxy, or the app TLS handshake started on a connection's first call | its last byte went back to the app |
| `app tls handshake` | internal | TLS setup with the app started | it ended. First call of a connection only |
| `prepare call` | internal | the call reached the proxy | the vendor call started: finding the ingress, the integration and the vendor api |
| `vendor call` | client | the vendor call started | the vendor's reply ended |
| `dns lookup`, under `vendor call` | internal | the lookup started | it ended |
| `tcp connect`, under `vendor call` | internal | the first try to connect | connected; several addresses tried count once |
| `vendor tls handshake`, under `vendor call` | internal | TLS setup with the vendor started | it ended |
| `send call`, under `vendor call` | internal | the call had its connection | the call was fully sent |
| `wait for vendor`, under `vendor call` | internal | the call was fully sent | the vendor's first reply byte |
| `receive reply`, under `vendor call` | internal | the first reply byte | the vendor's reply ended |
| `finish reply` | internal | the vendor's reply ended | the last byte went to the app |

- A step that did not happen has no span: no `dns lookup` for an IP address, no `vendor tls handshake` for plain http, none of the three on a reused connection.
- The spec checks run after the reply has gone back, so they are not in the trace.
- A refused call, and a `CONNECT` tunnel whose TLS setup failed, have only `hotline call` (with `app tls handshake` for the tunnel, when it was timed).
- A failed step ends at the failure with status error, and so do `vendor call` and `hotline call`. Why it failed is only in the record's `error.type`. A call to an internal address the guard refused marks only `vendor call` and `hotline call`.
- Every call gets a trace: there is no sampling, and the app's sampled flag does not change that (Peter, 2026-10-31).
- Times come from the proxy's clock, the same as the record's.

## Nothing on both

Each piece of data is either on the trace or on the record, never on both, unless there is a good reason (Peter, 2026-10-31).

- Spans carry only their name, kind, start, end and status.
- Two attributes moved from the record to the trace: `hotline.connection.reused` on `vendor call`, and `dns.question.name` on `dns lookup`.
- Method, route, status, integration, host, vendor IP and TLS facts stay only on the record. To find a call's timeline, search for its record and jump to its trace.
- The links are on both: the record's trace id is the trace's, and the record's span id is the id of `hotline call`.
- The record keeps `hotline.timing.total_us`, the length of `hotline call` (Peter, 2026-10-31: fine to have). Every other `hotline.timing.*_us` is gone. On the first call of an app's TLS connection, it counts from the start of the handshake.

## Ids and the app's trace

- When the app sent trace context, `hotline call` sits under the app's span, so the proxy's steps show inside the app's own trace. The app's span id is not on the record.
- Without it, `hotline call` has no parent and the trace id is a new one, as on the record.
- With `traceForward: true`, the vendor gets the id of `vendor call`, so the vendor's own spans sit under that step.

## Signed

The trace of a full record is signed with the integration's key, the same as the record (Peter, 2026-10-31). The signature goes on the record as `hotline.trace.signature`, so the record's own signature covers it, as with the body signatures. Minimal records are not signed, so their traces are not either.

The signed text is one line with the trace id, then one block per span in start order, a tie going by span id. Each block has the span's id, parent span id (empty for none), name, kind (`server`, `client` or `internal`), start and end in nanoseconds since 1970, and status (`unset` or `error`), then its attributes sorted by name. Each line is a name and a value, both as JSON, joined by new lines:

```
"trace.trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
"span.span_id": "00f067aa0ba902b7"
"span.parent_span_id": ""
"span.name": "hotline call"
"span.kind": "server"
"span.start_time": 1761940800000000000
"span.end_time": 1761940800012000000
"span.status": "unset"
"span.span_id": ...
```

To check a call, take its record and its spans (found by trace id), and rebuild both signatures with the key.

## How it is sent

- OTLP traces over gRPC, or over HTTP to `/v1/traces`, with each exporter's endpoint, TLS, login headers, gzip, resource labels and timing.
- Each destination has a queue of spans next to its queue of records, of the same `queueSize`, counted in spans. A full queue drops its oldest span and warns, at most once a minute.
- The proxy's own telemetry counts them like records: `otel.sdk.processor.span.queue.size`, `otel.sdk.processor.span.queue.capacity`, `otel.sdk.processor.span.processed` and `otel.sdk.exporter.span.exported`.
- `hotline.proxy.overhead` still measures the proxy's own time: `prepare call` plus `finish reply`.

## Where it lives

- `trace.go`: the spans of a call, and the trace signature.
- `internal/export/trace.go`: spans as the OpenTelemetry SDK sends them, and the two senders. It builds each span with the SDK's `tracetest.SpanStub`, since the proxy sets its own ids and times.
- `internal/export/queue.go`: one queue type for records and spans.
- New modules: `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` and `otlptracehttp`, Apache-2.0, the OpenTelemetry trace senders.
