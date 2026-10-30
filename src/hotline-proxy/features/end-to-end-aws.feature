@e2e
Feature: AWS SQS through the proxy, end to end
  The example app (examples/aws-sqs-invoices) uses the AWS SDK for Go to
  send invoice jobs to SQS and to receive them, through the real
  hotline-proxy program. The proxy reaches a fake SQS on this computer over
  TLS; the app points the SDK at it, as with a local SQS. The proxy runs
  with testdata/aws/hotline-proxy.yaml, changing only its addresses: the stable SQS spec with the invoices
  overlay (Peter, 2026-10-30).

  Background:
    Given a fake OTLP receiver
    And a fake SQS
    And the hotline-proxy program runs with the invoices hotline-proxy.yaml

  Scenario: An invoice job goes through Hotline to SQS and back
    When the example app requests the invoice for order A-1001
    And the example app works through the queue once
    Then the worker wrote invoice-A-1001.pdf
    And the fake SQS has no messages left
    And the fake SQS got every call signed by the SDK, over https, without Proxy-Authorization
    And the receiver got records for SendMessage, ReceiveMessage and DeleteMessage, each with its url.template and no results

  Scenario: A broken job is still delivered, and reported
    When the example app sends the message {"schemaVersion":1,"orderId":"A-1001"}
    Then the fake SQS has 1 message waiting
    And the receiver got a SendMessage record with the request results body $.MessageBody.eventId required; body $.MessageBody.language required; body $.MessageBody.requestedAt required; body $.MessageBody.templateId required
