# Agreed with Peter, 2026-10-30. Detail: docs/proxy-signatures.md.
Feature: Each record is signed
  A full record carries an RFC 9421 signature of everything it holds,
  made with HMAC-SHA256 and the integration's fingerprintKey, so anyone
  with the record and the key can check that nobody changed it. Login
  headers and bodies are never in the record, so they are never signed.
  The call itself is not changed.

  The signature base is the RFC 9421 lines for the call ("@method",
  "@target-uri", the kept request headers by name, "@status", the kept
  reply headers marked ;resp), then one line per other attribute and the
  record's timestamp, sorted by name, with JSON values, and last the
  "@signature-params" line.

  Background:
    Given the shop spec
    And the integration's fingerprintKey is read from a file

  Scenario: A record is signed
    Given the vendor answers 201 with "X-Request-Id: r1"
    When the app sends POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":2}
    Then the record has hotline.signature and hotline.signature_input
    And the signature input starts with "@method" "@target-uri" "content-length" "content-type" "@status"
    And the signature input names "x-request-id";resp
    And the signature input names every other attribute and log.record.timestamp
    And the signature input says keyid 9524c0eafdce16d4 and alg hmac-sha256
    And the signature checks out when rebuilt from the record alone, with the key

  Scenario Outline: A change to the record shows
    Given the vendor answers 201
    When the app sends POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":2}
    And <change> in the received record
    Then the signature no longer checks out

    Examples:
      | change                                 |
      | http.request.method is changed         |
      | url.full is changed                    |
      | a kept header's value is changed       |
      | http.response.status_code is changed   |
      | hotline.timing.total is changed        |
      | hotline.integration.id is changed      |
      | the timestamp is changed               |

  Scenario: A check result is covered
    When the app sends GET /orders?limit=abc
    Then the signature input names hotline.schema.request.errors
    And the signature checks out when rebuilt from the record alone, with the key

  Scenario: Login headers and bodies are never signed
    When the app sends POST /orders without the login key with "Idempotency-Key: k1; Authorization: Bearer s3cret; Cookie: session=1" and the json body {"item":"A1","quantity":2}
    Then the signature input names no authorization, proxy-authorization, cookie or body
    And the record holds no s3cret

  Scenario: The keyid names the key, never the key itself
    When the app sends GET /orders
    Then the record holds no hotline-test-signing-key-1

  Scenario: The call and the reply are not changed
    When the app sends GET /orders
    Then the vendor got no Signature or Signature-Input header
    And the app got no Signature or Signature-Input header

  Scenario: Minimal records are not signed
    When the app sends GET /orders with no integration ID
    Then the record has no hotline.signature

  Rule: An integration without a key

    Scenario: The record is signed with the integration ID, and the proxy warns
      Given the shop spec, with no fingerprintKey
      When the app sends GET /orders
      Then the signature input says keyid 603e7870264b9ef8 and alg hmac-sha256
      And the signature checks out when rebuilt from the record alone, with the key talon-shop-a
      And the proxy starts and logs a warning naming talon-shop-a
