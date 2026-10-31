# Answers to the open questions are in docs/proxy-spec-checks.md.
Feature: Checking calls against the vendor's spec
  Every part of a call is checked against our copy of the vendor's OpenAPI
  spec: path values, query values, headers, cookies and the body, and in
  the reply the status, headers and body. The checks run after the reply
  has gone back to the app and never change it.

  Each result is a structured entry with the part, the place, the rule and
  what the spec expects, never a value from the call. A result is written
  here as "part place rule expected"; several are joined by "; ".

  Background:
    Given the shop spec

  Rule: Every part of the request is checked

    Scenario: A call that matches the spec has no results
      Given the vendor answers 201
      When the app sends POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":2}
      Then the app got the vendor's answer
      And the record has no request results
      And the record has no reply results

    Scenario Outline: A mistake in one part is reported in that part
      When the app sends <call>
      Then the request results are <results>

      Examples:
        | call                                                                    | results                              |
        | GET /orders?limit=abc                                                   | query limit type integer             |
        | GET /orders?limit=500                                                   | query limit maximum 100              |
        | GET /orders?lmit=5                                                      | query lmit not_in_spec               |
        | GET /orders?tag=new&tag=old                                             | query tag[1] enum new, paid          |
        | GET /orders?filter[status]=lost                                         | query filter.status enum open, closed |
        | GET /orders with "Cookie: session=abc"                                  | cookie session type integer          |
        | GET /orders/1234                                                        | path orderId format uuid             |
        | POST /orders and the json body {"item":"A1","quantity":2}               | header Idempotency-Key required      |
        | POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1"} | body $.quantity required             |
        | POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":0} | body $.quantity minimum 1 |
        | POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":1,"note":"x"} | body $.note additionalProperties |
        | POST /orders with "Idempotency-Key: k1" and the json body {"item":"A1","quantity":1,"lines":[{"sku":"a"},{}]} | body $.lines[1].sku required |

    Scenario: Results never hold values from the call
      When the app sends GET /orders?limit=secret-value&filter[status]=other-secret
      Then no result holds secret-value or other-secret

    Scenario: Several mistakes are all reported
      When the app sends GET /orders?limit=abc&lmit=5
      Then the request results are query limit type integer; query lmit not_in_spec

    Scenario: Headers the spec doesn't name are ignored
      When the app sends GET /orders with "User-Agent: shop/1; traceparent: 00-abc-def-01"
      Then the record has no request results

    Scenario: Calls are matched to the route before they are checked
      When the app sends GET /nowhere
      Then the record says hotline.schema.skipped=no_spec_route

  Rule: The body's type must be one the spec lists

    Scenario: A content type the spec doesn't list is reported
      When the app sends POST /payments and the json body {"amount":1,"currency":"eur"}
      Then the request results are body $ content_type application/x-www-form-urlencoded

    Scenario: A body where the spec expects none is reported
      When the app sends GET /orders and the json body {"item":"A1"}
      Then the request results are body $ not_in_spec

    Scenario: A missing required body is reported
      When the app sends POST /orders with "Idempotency-Key: k1"
      Then the request results are body $ required

    Scenario: A body that is not valid JSON is reported
      When the app sends POST /orders with "Idempotency-Key: k1" and the json body {"item":
      Then the request results are body $ json

    Scenario: A listed format the proxy can't read is skipped
      When the app sends PUT /files/a.pdf and the pdf body %PDF-1.7
      Then the record says hotline.schema.skipped=unsupported_content_type
      And the record has no request results

    Scenario: A body larger than 1 MiB is not checked
      When the app sends POST /orders with "Idempotency-Key: k1" and a json body of 2 MiB
      Then the record says hotline.schema.skipped=body_too_large
      And the record has no request results

  Rule: Form bodies are read the way Stripe writes them

    Scenario Outline: A form body is checked like JSON
      When the app sends POST /payments and the form body <body>
      Then the request results are <results>

      Examples:
        | body                                                    | results                               |
        | amount=abc&currency=eur&metadata[order]=6735            | body $.amount type integer            |
        | amount=5                                                | body $.currency required              |
        | amount=5&currency=eur&items[0][price]=p1&items[1][quantity]=x | body $.items[1].quantity type integer |

    Scenario: A valid form body has no results
      When the app sends POST /payments and the form body amount=5&currency=eur&metadata[order]=6735&items[0][price]=p1&items[0][quantity]=2
      Then the record has no request results

  Rule: Formats are checked

    Scenario Outline: A wrong format is reported
      When the app sends GET /formats?<name>=<wrong>
      Then the request results are query <name> format <name>

      Examples:
        | name      | wrong               |
        | date-time | 2026-13-45T00:00:00Z |
        | date      | yesterday           |
        | email     | not-an-email        |
        | uuid      | 1234                |
        | uri       | no-scheme           |
        | ipv4      | 300.1.1.1           |
        | ipv6      | zz::1               |
        | int32     | 3000000000          |
        | int64     | 9223372036854775808 |

    Scenario Outline: Other formats are ignored
      When the app sends GET /formats?<name>=<value>
      Then the record has no request results

      Examples:
        | name      | value       |
        | hostname  | -not-a-host |
        | stripe-id | anything    |

  Rule: The login key must be present, and its value is never read

    Scenario: A missing login key is reported
      When the app sends GET /orders without the login key
      Then the request results are header Authorization required

    Scenario: A route open to all needs no login key
      When the app sends GET /public without the login key
      Then the record has no request results

  Rule: The reply is checked the same way

    Scenario: A reply body that breaks the spec is reported
      Given the vendor answers 200 with "X-Request-Id: r1" and the json body {"item":"A1"}
      When the app sends GET /orders/0b4e7d2a-5c55-4b36-9f6e-8a0d1f2b3c4d
      Then the reply results are body $.quantity required

    Scenario: A missing required reply header is reported
      Given the vendor answers 200 and the json body {"item":"A1","quantity":1}
      When the app sends GET /orders/0b4e7d2a-5c55-4b36-9f6e-8a0d1f2b3c4d
      Then the reply results are header X-Request-Id required

    Scenario: A status the spec doesn't list is reported, and the reply is not checked further
      Given the vendor answers 418 and the json body {"nonsense":true}
      When the app sends GET /orders
      Then the reply results are status 418 not_in_spec

    Scenario: A default reply matches any status
      Given the vendor answers 503 and the json body {}
      When the app sends GET /orders/0b4e7d2a-5c55-4b36-9f6e-8a0d1f2b3c4d
      Then the reply results are body $.message required

    Scenario: A streaming reply is not checked
      Given the vendor answers 200 with "Content-Type: text/event-stream" and the body data: x
      When the app sends GET /orders
      Then the record says hotline.schema.skipped=streaming
      And the record has no reply results

  Rule: At most 20 results each way

    Scenario: Many mistakes are capped, with the total
      When the app sends POST /orders with "Idempotency-Key: k1" and an order with 25 lines without a sku
      Then the record has 20 request results
      And the record says hotline.schema.request.errors_total=25

    Scenario: The total is left out when nothing was cut
      When the app sends POST /orders with "Idempotency-Key: k1" and an order with 3 lines without a sku
      Then the record has 3 request results
      And the record has no hotline.schema.request.errors_total

  Rule: Calls without a known integration are not checked

    Scenario: A not-passed call has no results
      Given the app sends no integration ID
      When the app sends GET /orders?limit=abc
      Then the record has no request results
