@e2e
Feature: AWS SNS through the proxy, end to end
  The shop example (examples/aws-shop) uses the AWS SDK for Go. Here its
  checkout app publishes invoice jobs to the order-paid topic on SNS
  through the real hotline-proxy program, with its own name, checkout, as
  the integration ID. SNS names the action in a form field of the body,
  which the proxy reads to pick the route (x-hotline-action-field). The
  proxy reaches a fake SNS on this computer over TLS; the app points the
  SDK at it, as with a local SNS. Like the topic's subscription, the fake
  puts each message on the invoices queue. The proxy runs with
  testdata/aws/hotline-proxy.yaml, changing only its addresses: the stable
  SNS spec with the order-paid topic overlay (Peter, 2026-11-01).

  Only the order-paid topic is reached this way: one integration can't
  name two apis with the same host, so the shop's other AWS apis can't all
  be on localhost. aws-shop.feature runs the whole story through the proxy
  inside the test, with a fake AWS for every host (Peter, 2026-11-01).

  Background:
    Given a fake OTLP receiver
    And a fake SNS whose order-paid topic feeds the invoices queue
    And the hotline-proxy program runs with the shop's hotline-proxy.yaml

  Scenario: An invoice job goes through Hotline to SNS, signed and checked
    When checkout sends the message {"schemaVersion":1,"eventId":"5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10","orderId":"A-1001","templateId":"invoice-standard-v2","language":"en","requestedAt":"2026-11-01T10:00:00Z"}
    Then the invoices queue has 1 message waiting
    And the fake SNS got every call signed by the SDK, over https, without Proxy-Authorization
    And the receiver got one Publish record from the integration checkout, naming the topic, with no results

  Scenario: A broken job is still delivered, and reported
    When checkout sends the message {"schemaVersion":1,"orderId":"A-1001"}
    Then the invoices queue has 1 message waiting
    And the receiver got a Publish record with the request results body $.Message.eventId required; body $.Message.language required; body $.Message.requestedAt required; body $.Message.templateId required
