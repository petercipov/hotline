# The proxy's own telemetry

Agreed with Peter on 2026-10-31. Tests: `src/hotline-proxy/features/telemetry.feature`. Code: `src/hotline-proxy/own.go`, `internal/export/own.go` and `internal/config/telemetry.go`.

## What it does

The proxy sends numbers about itself as OpenTelemetry metrics (numbers sent every few seconds) to the `telemetry` destination, and nowhere else: never mixed with the records and counts of vendor calls. An operator builds a Grafana board and alerts on them, such as "a destination has been down for 5 minutes", "a reload failed" or "the certificate expires within 7 days". This replaces a detail page on `/readyz` (health checks, decision 5).

```yaml
telemetry:
  otlp:                       # the same settings as one entry under exporters
    endpoint: otel-collector.observability:4317
  disable: [http.server.request.duration, go.*]   # optional; metrics to turn off
```

## What is sent

No number is split by integration, vendor, host, URL or route: a proxy may hold a million integrations and call any vendor, and the numbers stay as few as with one. Labels come only from short fixed lists (the 4 ingresses, methods, schemes, status codes, error types, reload results) or from the config's few destinations and certificates. The detail for each integration stays in the records, for the control plane.

| Metric | Kind | Unit | Labels | What it says |
|---|---|---|---|---|
| `http.server.active_requests` | up and down count | `{request}` | `http.request.method`, `url.scheme`, `hotline.ingress` | calls from apps in flight now |
| `http.server.request.duration` | histogram | `s` | `http.request.method`, `url.scheme`, `http.response.status_code`, `error.type`, `hotline.ingress` | how long each call took, from arrival until the last byte went to the app |
| `hotline.proxy.overhead` | histogram | `s` | `hotline.ingress` | the proxy's own time on each call that got as far as the vendor: `proxy_request` plus `proxy_response` |
| `otel.sdk.processor.log.queue.size` | up and down count | `{log_record}` | destination | records waiting in a destination's queue |
| `otel.sdk.processor.log.queue.capacity` | up and down count | `{log_record}` | destination | how many records the queue holds at most |
| `otel.sdk.processor.log.processed` | count | `{log_record}` | destination, `error.type` | records handed on to be sent; with `error.type=queue_full`, dropped from a full queue |
| `otel.sdk.exporter.log.exported` | count | `{log_record}` | destination, `error.type` | records sent; with `error.type`, lost in a send that failed |
| `otel.sdk.exporter.metric_data_point.exported` | count | `{data_point}` | destination, `error.type` | the same for the `hotline.requests` count |
| `hotline.destination.state` | gauge | `1` | `server.address`, `server.port`, `otel.component.name` | 1 while sends to the destination work, 0 from a failed send until one works again |
| `hotline.config.info` | gauge, always 1 | `1` | `hotline.config.version`, `hotline.listen.tls`, `hotline.listen.connect` | which config is in use, and whether the proxy port takes TLS and CONNECT |
| `hotline.config.reloads` | count | `{reload}` | `hotline.reload.result` (`success`, `failure`) | config reloads |
| `hotline.config.reload.last_success` | gauge | `1` | none | 1 unless the last reload failed, so an alert stays on until a good config arrives |
| `hotline.config.restart_needed` | gauge | `1` | none | 1 while a changed listen address, exporter or telemetry setting waits for a restart |
| `hotline.certificate.not_after` | gauge | `s` | `hotline.certificate.use` (`listen.tls`, `listen.connect`) | when the proxy's own certificate or the CONNECT issuer expires, in seconds since 1970 |
| `hotline.app_tls.failures` | count | `{connection}` | `error.type`, and `hotline.ingress=connect` inside a tunnel | app connections whose TLS setup failed |
| `go.memory.used`, `go.memory.limit`, `go.memory.allocated`, `go.memory.allocations`, `go.memory.gc.goal`, `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | gauges and counts | | as the standard says | the Go runtime: memory, garbage collection and goroutines |

"Destination" labels are `otel.component.type` and `otel.component.name` (such as `otlp_grpc_log_exporter/0`, numbered in the order of `exporters`), `server.address` and `server.port`, as the OpenTelemetry SDK metrics name them.

`error.type` on a failed send is the gRPC status in lower case, such as `unavailable` or `deadline_exceeded`, or `_OTHER` over HTTP. On a failed TLS setup it is `tls_untrusted_by_app` (the app refused the certificate), `plain_http` (plain HTTP on the TLS port), `tls_version` (only TLS versions the proxy does not take) or `tls_handshake_failed`. A method the standard does not list counts as `_OTHER`.

Histogram ranges are fixed. `hotline.proxy.overhead`: 0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.005, 0.025 and 0.1 seconds, so each ingress in use sends 11 numbers. `http.server.request.duration`: the standard's suggested 0.005 to 10 seconds (14 limits). An ingress or set of labels nobody used sends nothing.

## Decisions (Peter, 2026-10-31)

1. **A destination's state is a 1 or 0 gauge,** next to the counts of sent and lost records, like HAProxy's server state and Envoy's `server.live`. No time of the last good send.
2. **The queue and send numbers use OpenTelemetry's SDK metric names,** though the standard still marks them "development". If the standard renames them, we follow and say so in the release notes. Numbers with no standard name use `hotline.`.
3. **The config's state is a count plus two 1 or 0 gauges:** `hotline.config.reloads`, `hotline.config.reload.last_success` (as Prometheus does) and `hotline.config.restart_needed`. A change put back as it was clears `restart_needed`, as it is compared with the config the proxy started with.
4. **Without a `telemetry` section nothing of the proxy's own is measured or sent,** and the proxy logs one warning at start, like a missing admin port.
5. **The proxy's own time is a small histogram,** `hotline.proxy.overhead`, by ingress only, in seconds (Peter first asked for microseconds, then for seconds so it reads like the standard histograms on one board).
6. **Each call's total time is the standard `http.server.request.duration`,** in seconds, with the standard's labels. `http.route` is left out on purpose: for the proxy it is the vendor's route, and grows with apis.
7. **The warnings about dropped records stay** (once a minute per destination, and once at shutdown), so `kubectl logs` still shows a problem when the telemetry receiver is down or not set up. So do the reload warnings.
8. **The Go runtime numbers are sent,** from `go.opentelemetry.io/contrib/instrumentation/runtime` (Apache-2.0, the OpenTelemetry project).
9. **Every metric can be turned off on its own** with `telemetry.disable`: exact names, or a name ending in `*` for every name that starts with what is before it. A turned off metric is not measured or sent. A name that matches none of ours is a config warning, so a typo is seen.
10. **`telemetry` changes only on a restart,** like exporters; a change is logged as `telemetry changed and needs a restart` and sets `hotline.config.restart_needed`.

Chosen without a question: the certificate expiry is a time, not "days left", so it does not change every day; telemetry is sent on the same timing as the exporters; when the telemetry receiver itself is down only the warning lines show it; `http.server.active_requests` carries the method and scheme as the standard asks.
