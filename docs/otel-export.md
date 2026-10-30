# What the proxy sends over OpenTelemetry

This page lists every piece of data the Hotline proxy sends out over OTLP, as the code does it today. OTLP is the OpenTelemetry protocol: the standard way to ship telemetry (records, counts, traces) to a collector, which is a server that receives and forwards it.

The rules come from the spec, `proxy-spec.md` section 4 in the `hotline-strategic` repo. The code lives in `src/hotline-proxy/record.go`, `connect.go` and `internal/export/`. Where the two differ, the last section says so.

In short, the proxy sends two things:

1. **One log record per call**, named `hotline.request`. It holds everything known about the call.
2. **One counter**, `hotline.requests`, that goes up by one per call.

It sends no traces and none of its own health data yet.

## How it is sent

| What | Today |
|---|---|
| Protocol | OTLP over gRPC (the default) or over HTTP as protobuf (`protocol: http/protobuf`), to `/v1/logs` and `/v1/metrics`. Detail: [proxy-otlp-protocols.md](proxy-otlp-protocols.md). |
| Connection to the collector | Plain, or TLS with an `https://` endpoint, with `tls.ca` and a client certificate if needed. Login headers from `headers`. |
| Compression | gzip, unless `compression: none`. |
| Destinations | Every entry under `exporters` gets the same data. Each has its own queue, so one that is down never holds up the others or the calls. |
| How often | Every second, or sooner once 512 records are waiting. Fixed in the program, not in the config. |
| Time limit per send | 10 seconds, retries included. Fixed in the program. A busy collector is tried again after about a second at first, then at most every 5 seconds. A send the collector refuses loses its records, counted in a warning line like a full queue. |
| Records per send | At most 512. |
| Queue size | 10 000 records per destination, or `queueSize`. When full, the oldest record is dropped, and a warning log line gives the count, at most once a minute per destination and always once at shutdown. |
| At shutdown | Whatever is queued is sent first, within the shutdown time limit. |
| Sampling | None. Every call is recorded and counted. |
| Counter temporality | Cumulative by default (the running total since start); `temporality: delta` sends only the change since the last send. |

Config example:

```yaml
exporters:
  - otlp:
      endpoint: hotline-control-plane.hotline:4317
  - otlp:
      endpoint: otel-collector.observability:4317
      temporality: delta
      queueSize: 10000
  - otlp:
      protocol: http/protobuf
      endpoint: https://otel.example.com:4318
      headers:
        Authorization: { file: /etc/hotline/otlp/token }
```

## Resource

A resource is the set of labels that describe the sender. Both signals carry the same one.

| Attribute | Value |
|---|---|
| `service.name` | `hotline-proxy` |
| `service.version` | the program's version |
| `service.instance.id` | a random ID (a version 4 UUID) made at each start |

The instrumentation scope (the name of the code that made the data) is `hotline-proxy` for both signals.

## Signal 1: the request record

One OpenTelemetry log record per call, sent once the reply has gone back to the app. A call inside a `CONNECT` tunnel gets its own record; a tunnel that opens fine gets none.

- Event name: `hotline.request`.
- Timestamp: when the call reached the proxy.
- No body text and no severity: everything is in the attributes.
- Standard names come from the OpenTelemetry semantic conventions (the official list of field names), Go package `semconv` v1.43.0. Hotline-only fields start with `hotline.`.

There are two sizes of record:

- A **full record** is for a call whose integration ID is known and whose host belongs to one of that integration's apis.
- A **minimal record** is for everything else: no ID (`not-passed`), an unknown ID, a host the integration does not use, and calls the proxy refused.

### Fields in every record

| Attribute | Type | What it holds |
|---|---|---|
| `hotline.integration.id` | text | The ID from `Proxy-Authorization`, as sent, or `not-passed`. |
| `hotline.ingress` | text | How the app reached the proxy: `forward_proxy`, `url_in_path`, `host_header` or `connect`. |
| `server.address` | text | The vendor host, in lower case. |
| `http.request.method` | text | The method, such as `GET`. |
| `http.response.status_code` | whole number | The status the app got, also when the proxy made it up (`502`, `504`, `403`). |
| `error.type` | text | Only when the call failed. Values are listed below. |
| `network.peer.address` | text | The vendor IP the proxy connected to, after DNS. Only when a connection was made. |
| `hotline.timing.*` | number, seconds | The timings below. |

### Timings

Measured with Go's `httptrace` hooks (hooks that fire as each step of a connection happens). Each one is the length of its own step, except `first_byte`. A step that did not happen has no field: no DNS for an IP address, no TLS for plain `http`, and none of the three on a reused connection.

| Attribute | What it measures |
|---|---|
| `hotline.timing.dns` | Looking up the vendor's IP. |
| `hotline.timing.connect` | Opening the connection to the vendor. |
| `hotline.timing.tls` | Setting up TLS with the vendor. |
| `hotline.timing.first_byte` | From the call reaching the proxy until the vendor's first reply byte. |
| `hotline.timing.total` | From the call reaching the proxy until its last byte went back to the app. |
| `hotline.timing.proxy_request` | The proxy's own time before calling the vendor. |
| `hotline.timing.proxy_response` | The proxy's own time after the vendor's reply ended. |
| `hotline.timing.app_tls` | Setting up TLS between the app and the proxy, on a TLS port or inside a `CONNECT` tunnel. Only on the first call of each connection. |

### Fields only in a full record

| Attribute | Type | What it holds |
|---|---|---|
| `url.scheme` | text | `http` or `https`, as sent to the vendor. |
| `server.port` | whole number | The vendor port. |
| `url.full` | text | The vendor address with values replaced by `REDACTED` unless the spec marks them as safe. Left out when the path has no route in the spec. |
| `url.template` | text | The spec's route, such as `/v1/customer_sessions/{sessionId}`. Left out with `url.full`. |
| `dns.question.name` | text | The name looked up, when a lookup happened. |
| `network.protocol.version` | text | The HTTP version to the vendor: `1.1` or `2`. Only when the vendor replied. |
| `http.request.body.size` | whole number | Bytes sent to the vendor. |
| `http.response.body.size` | whole number | Bytes received from the vendor. Only when the vendor replied. |
| `tls.protocol.version` | text | Such as `1.3`. Only over TLS. |
| `tls.server.not_after` | text | When the vendor's certificate expires, as an RFC 3339 time in UTC. Only over TLS. |
| `http.request.header.<name>` | list of text | Allowed request headers, name in lower case. |
| `http.response.header.<name>` | list of text | Allowed reply headers, name in lower case. |
| `hotline.client.cancelled` | true or false | The app gave up before the vendor answered. |
| `hotline.connection.reused` | true or false | The call used a connection that was already open. |
| `hotline.config.version` | text | The `version` field of the config. |
| `hotline.schema.*` | see below | What the check against the spec found. |

**Headers.** Kept by default: `content-type`, `content-length`, `date`, `retry-after`, `request-id`, `x-request-id` and any `ratelimit-*`. A header marked `x-hotline-redact: false` in the spec is kept; one marked `true` shows as `REDACTED`. Never recorded: `authorization`, `proxy-authorization`, `cookie`, `set-cookie` and the login headers named in the spec. Every other header is left out, name and all.

### Check results

The proxy checks each full-record call against the vendor's spec, after the reply has gone back to the app.

| Attribute | Type | What it holds |
|---|---|---|
| `hotline.schema.request.errors` | list of maps | Problems with the call, at most 20. |
| `hotline.schema.response.errors` | list of maps | Problems with the reply, at most 20. |
| `hotline.schema.request.errors_total` | whole number | How many request problems there were, only when more than 20. |
| `hotline.schema.response.errors_total` | whole number | The same for the reply. |
| `hotline.schema.skipped` | text | Why part of the check did not run: `no_spec_route`, `body_too_large`, `unsupported_content_type` or `streaming`. |

Each problem is a map with these keys. It never holds a value from the call.

| Key | Always there? | What it holds |
|---|---|---|
| `part` | yes | `path`, `query`, `header`, `cookie`, `body` or `status`. |
| `rule` | yes | One word, such as `required`, `type`, `format`, `enum`, `not_in_spec` or `content_type`. |
| `place` | when known | The name (`page`) or the spot in the body (`$.cartItems[0].quantity`). |
| `expected` | when known | What the spec asks for. |

### Error types

`error.type` is set only when the call failed. The same name goes back to the app in the `Proxy-Status` header.

| Value | When |
|---|---|
| `http_request_error` | The proxy refused the request (`400`, `421`, `501`). |
| `destination_ip_prohibited` | The vendor IP is internal (`403`). |
| `dns_error`, `dns_timeout` | The lookup failed or took too long. |
| `connection_refused`, `destination_unavailable`, `connection_timeout` | Connecting to the vendor failed or took too long. |
| `tls_certificate_error`, `tls_protocol_error` | TLS with the vendor failed. |
| `http_response_timeout` | The vendor did not answer in time. |
| `http_response_incomplete` | The reply was cut off, or went silent for longer than `vendorTimeout`. |
| `tls_untrusted_by_app` | Inside a `CONNECT` tunnel, the app refused the certificate it was shown. |
| `tls_handshake_failed` | Inside a `CONNECT` tunnel, TLS with the app failed for another reason. |

### Refused calls and failed tunnels

A request the proxy reads and refuses still gets a minimal record, with `error.type=http_request_error`. `hotline.ingress` and `server.address` are there only when the proxy could work them out. The only timing is `hotline.timing.total`.

A `CONNECT` tunnel whose TLS setup with the app fails gets a minimal record with the ID and host from the `CONNECT`, `http.request.method=CONNECT`, `hotline.ingress=connect`, `hotline.timing.total`, and `hotline.timing.app_tls` when it was measured. It has no status code, since no HTTP reply was sent.

Plain HTTP sent to a TLS port is a failed TLS setup, not a request, so it has no record.

## Signal 2: the hotline.requests counter

| Name | Type | Unit | Attributes |
|---|---|---|---|
| `hotline.requests` | Sum that only goes up | `{request}` | `hotline.integration.id`, `url.template`, `http.request.method`, `http.response.status_code`, `error.type` |

Every record above also counts once here, refused calls and failed tunnels included. Each attribute is there only when the record has it, so minimal records carry no `url.template`.

There is no latency histogram and no pass or fail count: the control plane builds those from the records.

## What is not sent

- **Traces.** No spans at all. The spec plans an optional span per call, off by default.
- **The proxy's own health data.** The `telemetry` section of the config is accepted but nothing is sent there. Dropped records show only in the warning log line.
- **Body fingerprints**, `hotline.body.request.hmac_sha256` and `hotline.body.response.hmac_sha256`. Not built yet.
- **Bodies, cookies, `Authorization` and `Proxy-Authorization`.** Never sent, by design.
- **The proxy's own log lines.** They go to standard error as JSON, not over OTLP.

## Where the spec and the code differ

1. **Error type examples.** The spec's table gives `timeout` and `dns` as examples; the real values are longer, such as `dns_timeout` (table above).
2. **Minimal record and status code.** The spec says every minimal record has `http.response.status_code`. A failed tunnel's record has none.
3. **`hotline.schema.skipped=overloaded`.** In the spec's list, never set by the code.
4. **Send timing.** One second and ten seconds are fixed in the program; the spec does not mention them and they are not in the config.
5. **Not built yet, as the spec and decision 12 of slice 1 allow:** traces, health data to `telemetry`, and body fingerprints.
