# Requirements and decisions: docs/proxy-shop-example.md.
Feature: The shop's AWS apis through the proxy
  The shop example (examples/aws-shop) is four apps, each its own
  integration, using AWS's JSON apis: DynamoDB, Kinesis, EventBridge, Step
  Functions, SSM, Secrets Manager, KMS and CloudWatch Logs, beside SQS and
  S3. Each resource is an api in testdata/aws/hotline-proxy.yaml, with the
  stable spec from specs/aws and an overlay naming that resource and the
  shape of what is sent to it (Peter, 2026-11-01).

  Rule: A DynamoDB row is checked in DynamoDB's own form

    Background:
      Given the shop api aws-dynamodb-orders

    Scenario: A valid order row has no results, and the record names the table
      Given AWS answers 200 with the json body {}
      When the app calls DynamoDB_20120810.PutItem with the json body {"TableName":"orders","Item":{"orderId":{"S":"A-1001"},"total":{"N":"12.50"},"currency":{"S":"EUR"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"}}}
      Then the record has url.template /#X-Amz-Target=DynamoDB_20120810.PutItem
      And the record has no request results
      And the record holds db.collection.name = orders
      And the record holds db.operation.name = DynamoDB_20120810.PutItem

    Scenario Outline: A mistake in the row is reported at its place
      Given AWS answers 200 with the json body {}
      When the app calls DynamoDB_20120810.PutItem with the json body {"TableName":"orders","Item":<item>}
      Then the request results are <results>

      Examples:
        | item | results |
        | {"orderId":{"S":"A-1001"},"currency":{"S":"EUR"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"}} | body $.Item.total required |
        | {"orderId":{"S":"A-1001"},"total":{"S":"12.50"},"currency":{"S":"EUR"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"}} | body $.Item.total.N required; body $.Item.total.S additionalProperties |
        | {"orderId":{"S":"A-1001"},"total":{"N":"12.5"},"currency":{"S":"EUR"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"}} | body $.Item.total.N pattern ^[0-9]+\.[0-9]{2}$ |
        | {"orderId":{"S":"A-1001"},"total":{"N":"12.50"},"currency":{"S":"GBP"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"}} | body $.Item.currency.S enum EUR, CZK, USD |
        | {"orderId":{"S":"A-1001"},"total":{"N":"12.50"},"currency":{"S":"EUR"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"},"note":{"S":"x"}} | body $.Item.note additionalProperties |

    Scenario: A row for another table is reported
      Given AWS answers 200 with the json body {}
      When the app calls DynamoDB_20120810.GetItem with the json body {"TableName":"invoices","Key":{"orderId":{"S":"A-1001"}}}
      Then the request results are body $.TableName const orders

    Scenario: A bad row is also reported when it is read
      Given AWS answers 200 with the json body {"Item":{"orderId":{"S":"A-1001"},"total":{"N":"12.50"},"currency":{"S":"EUR"},"paidAt":{"S":"2026-11-01T10:00:00Z"}}}
      When the app calls DynamoDB_20120810.GetItem with the json body {"TableName":"orders","Key":{"orderId":{"S":"A-1001"}}}
      Then the record has no request results
      And the reply results are body $.Item.language required

    Scenario: DynamoDB's "already there" reply is a normal error reply
      Given AWS answers 400 with the json body {"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"The conditional request failed"}
      When the app calls DynamoDB_20120810.PutItem with the json body {"TableName":"orders","Item":{"orderId":{"S":"A-1001"},"total":{"N":"12.50"},"currency":{"S":"EUR"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"}},"ConditionExpression":"attribute_not_exists(orderId)"}
      Then the record has no request results
      And the record has no reply results

    Scenario: Results never hold values from the row
      Given AWS answers 200 with the json body {}
      When the app calls DynamoDB_20120810.PutItem with the json body {"TableName":"orders","Item":{"orderId":{"S":"A-1001"},"total":{"N":"12.50"},"currency":{"S":"GBP"},"language":{"S":"en"},"paidAt":{"S":"2026-11-01T10:00:00Z"}}}
      Then the request results are body $.Item.currency.S enum EUR, CZK, USD
      And no result holds A-1001

  Rule: A Kinesis record's base64 data is decoded and checked

    Background:
      Given the shop api aws-kinesis-shop-events
      And AWS answers 200 with the json body {"ShardId":"shardId-000000000000","SequenceNumber":"1"}

    Scenario: A valid shop event has no results, and the record names the stream
      When the app puts a Kinesis record holding {"type":"order-paid","orderId":"A-1001","at":"2026-11-01T10:00:00Z","total":"12.50"}
      Then the record has no request results
      And the record holds messaging.destination.name = shop-events

    Scenario Outline: A mistake in the data is reported at its place
      When the app puts a Kinesis record holding <data>
      Then the request results are <results>

      Examples:
        | data | results |
        | {"type":"order-paid","orderId":"A-1001"} | body $.Data.at required |
        | {"type":"order-shipped","orderId":"A-1001","at":"2026-11-01T10:00:00Z"} | body $.Data.type enum order-paid, invoice-made |
        | order A-1001 paid | body $.Data json |

    Scenario: Data that is not base64 is reported
      When the app calls Kinesis_20131202.PutRecord with the json body {"StreamName":"shop-events","PartitionKey":"A-1001","Data":"not base64!"}
      Then the request results are body $.Data base64

  Rule: The other resources are pinned, and what they carry is checked

    Scenario: An EventBridge event's Detail is checked
      Given the shop api aws-eventbridge-shop
      And AWS answers 200 with the json body {"FailedEntryCount":0,"Entries":[{"EventId":"e-1"}]}
      When the app calls AWSEvents.PutEvents with the json body {"Entries":[{"EventBusName":"shop","Source":"shop.invoices","DetailType":"InvoiceReady","Detail":"{\"orderId\":\"A-1001\",\"eventId\":\"5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10\",\"language\":\"en\",\"createdAt\":\"2026-11-01T10:00:00Z\"}"}]}
      Then the request results are body $.Entries[0].Detail.file required
      And the record holds rpc.method = AWSEvents.PutEvents

    Scenario: An event to another bus is reported
      Given the shop api aws-eventbridge-shop
      And AWS answers 200 with the json body {"FailedEntryCount":0,"Entries":[{"EventId":"e-1"}]}
      When the app calls AWSEvents.PutEvents with the json body {"Entries":[{"EventBusName":"default","Source":"shop.invoices","DetailType":"InvoiceReady","Detail":"{\"orderId\":\"A-1001\",\"eventId\":\"5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10\",\"file\":\"A-1001.pdf\",\"language\":\"en\",\"createdAt\":\"2026-11-01T10:00:00Z\"}"}]}
      Then the request results are body $.Entries[0].EventBusName const shop

    Scenario: A refund's input to the workflow is checked
      Given the shop api aws-sfn-credit-note
      And AWS answers 200 with the json body {"executionArn":"arn:aws:states:eu-west-1:192325781350:execution:credit-note:r-1","startDate":1793527200}
      When the app calls AWSStepFunctions.StartExecution with the json body {"stateMachineArn":"arn:aws:states:eu-west-1:192325781350:stateMachine:credit-note","input":"{\"orderId\":\"A-1001\",\"amount\":\"12.50\",\"currency\":\"EUR\",\"reason\":\"broken\"}"}
      Then the request results are body $.input.reason enum returned, damaged, late, other

    Scenario: The mailer's message, an EventBridge event, is checked when received
      Given the shop api aws-sqs-mail
      And AWS answers 200 with the json body {"Messages":[{"MessageId":"m-1","ReceiptHandle":"r-1","MD5OfBody":"x","Body":"{\"detail-type\":\"InvoiceReady\",\"source\":\"shop.invoices\",\"detail\":{\"orderId\":\"A-1001\",\"eventId\":\"5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10\",\"language\":\"en\",\"createdAt\":\"2026-11-01T10:00:00Z\"}}"}]}
      When the app calls AmazonSQS.ReceiveMessage with the json body {"QueueUrl":"https://sqs.eu-west-1.amazonaws.com/192325781350/mail"}
      Then the reply results are body $.Messages[0].Body.detail.file required

    Scenario Outline: Each app reads only its own secret, setting and key
      Given the shop api <api>
      And AWS answers 200 with the json body {}
      When the app calls <action> with the json body <body>
      Then the request results are <results>

      Examples:
        | api                     | action                         | body | results |
        | aws-secrets-payment     | secretsmanager.GetSecretValue  | {"SecretId":"shop/other"} | body $.SecretId const shop/payment-key |
        | aws-ssm-shop            | AmazonSSM.GetParameter         | {"Name":"/shop/other"} | body $.Name const /shop/invoices/default-template |
        | aws-kms-invoice-signing | TrentService.Sign              | {"KeyId":"alias/other","Message":"AAAA","MessageType":"DIGEST","SigningAlgorithm":"ECDSA_SHA_256"} | body $.KeyId const alias/invoice-signing |
        | aws-logs-shop           | Logs_20140328.PutLogEvents     | {"logGroupName":"/shop/aws-shop","logStreamName":"payments","logEvents":[{"timestamp":1793527200000,"message":"hello"}]} | body $.logStreamName enum checkout, invoices, mailer, refunds |
