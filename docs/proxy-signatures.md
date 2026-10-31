# Signed records

*Agreed with Peter and built, 2026-10-30; changed 2026-10-31. Scenarios: `src/hotline-proxy/features/signature.feature`. The source of truth is `proxy-spec.md` in the `hotline-strategic` repo.*

Every full record carries a signature of everything it holds. Anyone with the record and the integration's key can check that nobody changed the record after the proxy wrote it.

Some terms, in plain words:

- **HMAC** is a short code made from some bytes and a secret key. The same bytes and key always give the same code. Without the key, nobody can make the code.

## What the record holds

| Attribute | What it holds |
|---|---|
| `hotline.signature` | The HMAC-SHA256 of the signed text, in base64. |
| `hotline.signature_key_id` | The first 8 bytes of the key's SHA-256, in hex: it names the key, never shows it. |
| `hotline.request.body.signature` | The HMAC-SHA256 of the call's body as it passed, in base64. Only when there was a body. |
| `hotline.response.body.signature` | The HMAC-SHA256 of the reply's body as it passed, in base64. Only when there was a body. |

## The signed text

One line per attribute, sorted by name, every attribute but `hotline.signature` itself, the key ID and the body signatures included. Then a line for the record's timestamp, in nanoseconds since 1970, and lines for its trace id, span id and flags, in hex ([proxy-trace-context.md](proxy-trace-context.md)):

```
"hotline.integration.id": "talon-shop-a"
"hotline.request.body.signature": "…base64…"
"http.request.header.content-type": ["application/json"]
"http.response.status_code": 201
…
"log.record.timestamp": 1793404800000000000
"log.record.trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
"log.record.span_id": "00f067aa0ba902b7"
"log.record.trace_flags": "01"
```

The name and the value are both JSON: text in quotes, numbers, `true` or `false`, lists, and maps with their keys sorted. Lines end with a line feed, and the last line has none. To check a record, build the same text from it and compare the HMAC with the key.

No list of names is kept: the whole record is signed, so sorting by name fixes the order (Peter, 2026-10-31). This is Hotline's own rule, not RFC 9421, which lists the signed parts because it signs only some of an HTTP message. A tool on the way that adds or removes an attribute makes the check fail.

## Body signatures

The proxy signs each body as it passes, with no copy and no size limit, so a body larger than the copy kept for the checks is signed whole. In a dispute, whoever holds a body and the key can recompute its signature and show it is the body that was sent or received (Peter, 2026-10-31). HMAC with the key, not a plain hash, because a plain hash of a short body such as `{"ok":true}` can be guessed. If a call broke off, the signature covers the bytes that passed, which `http.request.body.size` and `http.response.body.size` count. The bodies themselves are never recorded.

## Decisions (Peter, 2026-10-30 and 2026-10-31)

1. **Sign what the record holds, never the original call.** The record is all a checker has.
2. **Every field is signed**, timings and check results included. Since 2026-10-31 the timings are a trace, signed on its own in `hotline.trace.signature`, which the record's signature covers ([proxy-traces.md](proxy-traces.md)).
3. **Each body is signed on its own**, in the record, and those signatures are covered by the record's signature (2026-10-31). Login headers are never in the record.
4. **No list of names** (2026-10-31): the attributes are sorted by name. This replaces the RFC 9421 form with `hotline.signature_input` of 2026-10-30.
5. **The key** is the integration's `fingerprintKey`, read from `{ file: path }` or `{ env: NAME }`; a key written in the config is refused. An integration without a key is signed with its ID, which is not secret, and the proxy logs a warning at start.
6. **Our own code**, no outside library.
7. **Minimal records are not signed**, and the call and reply that apps and vendors see are not changed.

## Later

- A `hotline-proxy verify` command that checks a record with the key. The tests already check signatures the way it would.
- Reading a key file again when it changes, with config reload. Today the key is read once at start.
- Signed batches with the proxy's own key (`technical-notes.md`, the evidence ledger).
