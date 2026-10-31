# Signed records

*Agreed with Peter and built, 2026-10-30. Scenarios: `src/hotline-proxy/features/signature.feature`. The source of truth is `proxy-spec.md` in the `hotline-strategic` repo.*

Every full record carries a signature of everything it holds. Anyone with the record and the integration's key can check that nobody changed the record after the proxy wrote it.

Some terms, in plain words:

- **HMAC** is a short code made from some bytes and a secret key. The same bytes and key always give the same code. Without the key, nobody can make the code.
- **RFC 9421**, "HTTP Message Signatures", is the internet standard for signing an HTTP call. It writes chosen parts of a call as text lines in a fixed order, the *signature base*, and signs that text.

## What the record holds

| Attribute | What it holds |
|---|---|
| `hotline.signature` | The HMAC-SHA256 of the signature base, in base64. |
| `hotline.signature_input` | What was signed, as RFC 9421 writes it: the list of covered names, then `created` (when it was signed, in seconds since 1970), `keyid` and `alg="hmac-sha256"`. |

Example input:

```
("@method" "@target-uri" "content-length" "content-type" "@status" "date";resp "x-request-id";resp "hotline.integration.id" … "log.record.timestamp");created=1793404800;keyid="9524c0eafdce16d4";alg="hmac-sha256"
```

## The signature base

One line per covered part, `"name": value`, in this order:

1. `"@method"`: `http.request.method`.
2. `"@target-uri"`: `url.full`, as the record holds it, so with the same redaction. Left out when the record has none.
3. The kept request headers, `"content-type": application/json`, by name in alphabetical order. A header with several values joins them with `, `, as RFC 9421 says.
4. `"@status"`: `http.response.status_code`.
5. The kept reply headers, marked `;resp`, such as `"x-request-id";resp: r1`, by name. RFC 9421 has no word for "the reply's header" inside one signature, so `;resp` is Hotline's own mark.
6. Every other attribute, `"hotline.timing.total": 0.088`, and the record's timestamp as `"log.record.timestamp"` in nanoseconds since 1970, all sorted by name. Values are written as JSON: text in quotes, numbers, `true` or `false`, lists, and maps with their keys sorted.
7. Last, `"@signature-params": ` followed by `hotline.signature_input`.

Lines end with a line feed, and the last line has none. Lines 1 to 5 follow RFC 9421 exactly; line 6 is Hotline's own, so a plain RFC 9421 checker can't check a whole record by itself (Peter, 2026-10-30). The signer in `internal/httpsig` is tested against the RFC's own example B.2.5.

## Decisions (Peter, 2026-10-30)

1. **Sign what the record holds, never the original call.** The record is all a checker has.
2. **Every field is signed**, timings and check results included, not only the HTTP parts.
3. **Login headers and bodies are never signed.** They are never in the record. No body digest is recorded for now; adding one later is safe, since each signature lists what it covers.
4. **This replaces the body fingerprints** `hotline.body.request.hmac_sha256` and `hotline.body.response.hmac_sha256` that the spec planned.
5. **The key** is the integration's `fingerprintKey`, read from `{ file: path }` or `{ env: NAME }`; a key written in the config is refused. `keyid` is the first 8 bytes of the key's SHA-256, in hex, never the key itself. An integration without a key is signed with its ID, which is not secret, and the proxy logs a warning at start.
6. **Our own code**, no outside library: building the signature base is short.
7. **Minimal records are not signed**, and the call and reply that apps and vendors see are not changed.

## Later

- A `hotline-proxy verify` command that checks a record with the key. The tests already check signatures the way it would.
- Reading a key file again when it changes, with config reload. Today the key is read once at start.
- Signed batches with the proxy's own key (`technical-notes.md`, the evidence ledger).
