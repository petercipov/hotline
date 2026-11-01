@e2e
Feature: AWS SQS through the proxy, end to end
  The shop example (examples/aws-shop) uses the AWS SDK for Go. Here its
  checkout app sends invoice jobs to SQS through the real hotline-proxy
  program, with its own name, checkout, as the integration ID. The proxy
  reaches a fake SQS on this computer over TLS; the app points the SDK at
  it, as with a local SQS. The proxy runs with testdata/aws/hotline-proxy.yaml,
  changing only its addresses: the stable SQS spec with the invoices
  overlay (Peter, 2026-10-30).

  Only the invoices queue is reached this way: one integration can't name
  two apis with the same host, so the shop's other AWS apis can't all be
  on localhost. aws-shop.feature runs the whole story through the proxy
  inside the test, with a fake AWS for every host (Peter, 2026-11-01).

  Background:
    Given a fake OTLP receiver
    And a fake SQS
    And the hotline-proxy program runs with the shop's hotline-proxy.yaml

  Scenario: An invoice job goes through Hotline to SQS, signed and checked
    When checkout sends the message {"schemaVersion":1,"eventId":"5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10","orderId":"A-1001","templateId":"invoice-standard-v2","language":"en","requestedAt":"2026-11-01T10:00:00Z"}
    Then the fake SQS has 1 message waiting
    And the fake SQS got every call signed by the SDK, over https, without Proxy-Authorization
    And the receiver got one SendMessage record from the integration checkout, with no results

  Scenario: A broken job is still delivered, and reported
    When checkout sends the message {"schemaVersion":1,"orderId":"A-1001"}
    Then the fake SQS has 1 message waiting
    And the receiver got a SendMessage record with the request results body $.MessageBody.eventId required; body $.MessageBody.language required; body $.MessageBody.requestedAt required; body $.MessageBody.templateId required
