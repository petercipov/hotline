# Proxy example

A complete, working-shape example for `proxy-spec.md`:

- `hotline-proxy.yaml`: the full config.
- `specs/talon-one.openapi.yaml`: our copy of the Talon.One spec, with `x-hotline-redact` marks.
- `specs/stripe.openapi.yaml`: our copy of the Stripe spec, with `x-hotline-redact` marks.

What to keep or redact is marked only in the specs; the config has no lists.

## Sample call 1: Talon.One, forward proxy ingress

```
curl http://localhost:8080 \
  --request-target 'https://api.talon.one/v2/customer_sessions/sess-8812?dry=true' \
  -X PUT \
  -H 'Host: api.talon.one' \
  -H 'Proxy-Authorization: Basic dGFsb24tc2hvcC1hOng=' \
  -H 'Authorization: ApiKey-v1 c2VjcmV0' \
  -H 'Content-Type: application/json' \
  -d '{"customerSession":{"profileId":"jo@shop.com","state":"open","cartItems":[{"sku":"A1","quantity":0,"price":9.5}]}}'
```

`dGFsb24tc2hvcC1hOng=` is `talon-shop-a:x` in base64.

The proxy reads the ID `talon-shop-a` from `Proxy-Authorization`, removes that header, finds the api for host `api.talon.one`, and makes the call with `https`.

What the record holds (the main fields):

```
hotline.integration.id        talon-shop-a
hotline.ingress               forward_proxy
http.request.method           PUT
url.full                      https://api.talon.one/v2/customer_sessions/REDACTED?dry=true
url.template                  /v2/customer_sessions/{customerSessionId}
http.response.status_code     200
http.response.header.x-request-id   req-5521
hotline.schema.request.errors  ["$.customerSession.cartItems[0].quantity below minimum 1"]
```

Why each value looks the way it does:
- `customerSessionId` is redacted because it has no mark, and unmarked path values are redacted.
- `dry` is kept because the spec marks it `x-hotline-redact: false`.
- `X-Request-Id` is kept because it is on the default header list.
- `Authorization` is never recorded: the spec's security section names it.
- The schema error names the field and the rule, never the value.

## Sample call 2: Stripe, vendor URL in path ingress

```
curl 'http://localhost:8080/https://api.stripe.com/v1/payment_intents?limit=10&customer=cus_77' \
  -H 'Proxy-Authorization: Basic dGFsb24tc2hvcC1hOng=' \
  -H 'Authorization: Bearer sk_test_123'
```

What the record holds:

```
hotline.integration.id        talon-shop-a
hotline.ingress               url_in_path
http.request.method           GET
url.full                      https://api.stripe.com/v1/payment_intents?limit=10&customer=REDACTED
url.template                  /v1/payment_intents
http.response.status_code     200
```

- `limit` is kept because the Stripe spec marks it `x-hotline-redact: false`.
- `customer` is redacted because it has no mark.
