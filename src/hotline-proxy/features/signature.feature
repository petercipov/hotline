# Agreed with Peter, 2026-10-30, changed 2026-10-31. Detail: docs/proxy-signatures.md.
Feature: Each record is signed
  A full record carries a signature of everything it holds, made with
  HMAC-SHA256 and the integration's fingerprintKey, so anyone with the
  record and the key can check that nobody changed it. The call itself is
  not changed.

  The signed text is one line per attribute, sorted by name, then a line
  for the record's timestamp: the name and the value, both as JSON. No list
  of names is needed, since the whole record is signed (Peter, 2026-10-31).

  Each body is signed too, on its own, as it passes: the call's body in
  hotline.request.body.signature and the reply's in
  hotline.response.body.signature. In a dispute, whoever holds a body and
  the key can show it is the one that was sent or received. The bodies
  themselves are never recorded.

  Background:
    Given the shop spec
    And the integration's fingerprintKey is read from a file

  Rule: The whole record is signed

    Scenario: A record is signed
      Given the vendor answers 201 with "X-Request-Id: r1"
      When the app sends POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":2}
      Then the record has hotline.signature and hotline.signature_key_id 9524c0eafdce16d4
      And the signature checks out when rebuilt from the record alone, with the key

    Scenario Outline: A change to the record shows
      Given the vendor answers 201
      When the app sends POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":2}
      And <change> in the received record
      Then the signature no longer checks out

      Examples:
        | change                                     |
        | http.request.method is changed             |
        | url.full is changed                        |
        | a kept header's value is changed           |
        | http.response.status_code is changed       |
        | hotline.timing.total_us is changed            |
        | hotline.integration.id is changed          |
        | hotline.request.body.signature is changed  |
        | an attribute is added                      |
        | an attribute is removed                    |
        | the timestamp is changed                   |

    Scenario: A check result is covered
      When the app sends GET /orders?limit=abc
      Then the record has hotline.schema.request.errors
      And the signature checks out when rebuilt from the record alone, with the key

    Scenario: Login headers are never in the record
      When the app sends POST /orders without the login key with "Idempotency-Key: k1; Authorization: Bearer s3cret; Cookie: session=1" and the json body {"item":"A1","quantity":2}
      Then the record holds no s3cret

    Scenario: The key ID names the key, never the key itself
      When the app sends GET /orders
      Then the record holds no hotline-test-signing-key-1

    Scenario: The call and the reply are not changed
      When the app sends GET /orders
      Then the vendor got no Signature or Signature-Input header
      And the app got no Signature or Signature-Input header

    Scenario: Minimal records are not signed
      When the app sends GET /orders with no integration ID
      Then the record has no hotline.signature

  Rule: Each body is signed on its own

    Scenario: The call's body
      When the app sends POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":2}
      Then hotline.request.body.signature is the HMAC of the body the vendor got, with the key
      And the record holds no A1

    Scenario: The reply's body
      Given the vendor answers 200 and the json body {"orders":[]}
      When the app sends GET /orders
      Then hotline.response.body.signature is the HMAC of the body the app got, with the key

    Scenario: A body larger than the copy kept for the checks is signed whole
      When the app sends POST /orders with "Idempotency-Key: k1" and a json body of 2 MiB
      Then hotline.request.body.signature is the HMAC of the body the vendor got, with the key

    Scenario: No body, no body signature
      Given the vendor answers 201
      When the app sends GET /orders
      Then the record has no hotline.request.body.signature or hotline.response.body.signature

  Rule: An integration without a key

    Scenario: The record is signed with the integration ID, and the proxy warns
      Given the shop spec, with no fingerprintKey
      When the app sends GET /orders
      Then the record has hotline.signature and hotline.signature_key_id 603e7870264b9ef8
      And the signature checks out when rebuilt from the record alone, with the key talon-shop-a
      And the proxy starts and logs a warning naming talon-shop-a
