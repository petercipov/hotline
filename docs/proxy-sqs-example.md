# A Go app sending to AWS SQS through Hotline, with a checked message: requirements draft

*Draft for Peter, 2026-10-30. Nothing here is agreed or built yet. It builds on [proxy-aws.md](proxy-aws.md) (agreed 2026-10-29, not built) and on the spec checks in [proxy-spec-checks.md](proxy-spec-checks.md) (built). Open questions are at the end, each with a recommendation.*

## What Peter asked for

1. A complete, working example: a Go app that sends to AWS SQS through Hotline, with end-to-end tests.
2. A complete OpenAPI spec for SQS, so every SQS call the app makes is checked, not only `SendMessage`.
3. A check of the message itself. One integration sends a JSON message: a job asking for the invoice PDF of one order (Peter, 2026-10-30, in place of the first idea of a parquet aggregation job in S3).

## The use case

- **The shop** (the app that asks for the invoice) sends one message per paid order: "make the invoice for order `A-1001` with the template `invoice-standard-v2`, in English".
- **The worker** (the app that does the job) receives the message, looks up the order, renders the PDF with the template the message names, and deletes the message from the queue so it is not handled twice.
- **Hotline** sits between both apps and SQS. It records every call and checks it against the spec, including the job message inside it. It never blocks a call: a broken message still reaches SQS, and the check result shows up in the record.

## The job message (version 1)

A first proposal. Field names are open for Peter to change.

```json
{
  "schemaVersion": 1,
  "eventId": "5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10",
  "orderId": "A-1001",
  "templateId": "invoice-standard-v2",
  "language": "en",
  "requestedAt": "2026-10-30T19:30:00Z",
  "sendTo": "customer@example.com"
}
```

| Field | Required? | Rule | Why |
|---|---|---|---|
| `schemaVersion` | yes | whole number, exactly `1` | Lets the message change later without breaking old workers. |
| `eventId` | yes | `uuid` | The ID of this event (Peter, 2026-10-30). The worker uses it to notice an event it already handled (SQS may deliver a message twice). |
| `orderId` | yes | text like `A-1001`: a capital letter, a dash, digits | The order to make the invoice for. |
| `templateId` | yes | text of lower-case letters, digits and dashes, such as `invoice-standard-v2` | Which template renders the PDF (Peter, 2026-10-30). The worker refuses a template it doesn't know; the schema only checks the shape. |
| `language` | yes | one of `en`, `de`, `sk`, `cs` | The invoice's language. |
| `requestedAt` | yes | `date-time` | When the shop asked; helps when looking into a late invoice. |
| `sendTo` | no | `email` | Where to mail the PDF, if anywhere. Mailing it is not part of this step. |

Any other field is reported (`additionalProperties: false`), so a typo like `ordrId` is caught.

## How the message sits in the SQS spec

An SQS `SendMessage` body is JSON, but the message itself is a **string** inside it, in the field `MessageBody`:

```json
{"QueueUrl": "https://sqs.eu-west-1.amazonaws.com/123456789012/invoices",
 "MessageBody": "{\"schemaVersion\":1,\"eventId\":\"5f0c…\", …}"}
```

So the check has to read the string as JSON, then check that against the job schema. OpenAPI 3.1 already has words for this, from JSON Schema: `contentMediaType` (what the string holds) and `contentSchema` (the schema for what it holds).

```yaml
MessageBody:
  type: string
  contentMediaType: application/json
  contentSchema:
    $ref: '#/components/schemas/InvoiceJob'
```

Requirements:

1. **Strings holding JSON are checked** when the spec says `contentMediaType: application/json` with a `contentSchema`. The JSON Schema library the proxy already uses supports this; it only needs turning on. Without these words, a string stays a plain string, as today.
2. **Places carry on into the inner message.** A missing `orderId` inside the message is reported as `body $.MessageBody.orderId required`. A message that is not valid JSON is `body $.MessageBody json`. As always, results never hold a value from the call.
3. **The same in replies.** The worker's `ReceiveMessage` reply holds the messages in `Messages[].Body`; our spec marks those the same way, so a bad message is also reported when the worker receives it (`response body $.Messages[0].Body.orderId required`).
4. **SQS's content type counts as JSON.** SQS sends `Content-Type: application/x-amz-json-1.0`. Today the proxy only reads `application/json` and `…+json`, so it would skip SQS bodies as `unsupported_content_type`. A spec that lists `application/x-amz-json-1.0` gets it read as JSON. (Only this exact AWS name, plus `1.1`, which other AWS services use.)

### Three integrations, three message schemas

There will be three integrations, each with its own queue and its own message schema, all starting from the same SQS spec (Peter, 2026-10-30). The queues differ in the path of their queue URL: `https://sqs.eu-west-1.amazonaws.com/123456789012/invoices`, `…/123456789012/<second queue>`, and so on.

One thing to know: with SQS's JSON protocol, every call goes to the path `/`. The queue URL is not in the call's path; it travels in the body, in the field `QueueUrl`. So the proxy cannot pick a route by queue, but the spec can check the field.

Requirements:

1. **One spec per integration.** Each is the full SQS spec with two things added: its own message schema on `MessageBody`, and its own queue URL on `QueueUrl` (an exact value, `const`). A message sent to the wrong queue is then reported as `body $.QueueUrl const`.
2. **No config change.** The config already allows several apis on the same host, when different integrations use them (`config.feature`, "The same host in two apis of different integrations").

```yaml
apis:
  aws-sqs-invoices:
    hosts: [sqs.eu-west-1.amazonaws.com]
    spec: ./specs/aws-sqs-invoices.openapi.yaml
    vendorTimeout: 25s             # above SQS's 20 second long polling
  aws-sqs-<second>:
    hosts: [sqs.eu-west-1.amazonaws.com]
    spec: ./specs/aws-sqs-<second>.openapi.yaml
    vendorTimeout: 25s
  aws-sqs-<third>:
    hosts: [sqs.eu-west-1.amazonaws.com]
    spec: ./specs/aws-sqs-<third>.openapi.yaml
    vendorTimeout: 25s

integrations:
  - id: invoices
    apis: [aws-sqs-invoices]
  - id: <second>
    apis: [aws-sqs-<second>]
  - id: <third>
    apis: [aws-sqs-<third>]
```

The message schema and queue URL go into `MessageBody` and `QueueUrl` of `SendMessage`, `SendMessageBatch` (each entry) and the `ReceiveMessage` reply; `QueueUrl` also in every other action that names a queue. See question 2 for how the three files stay in step with the plain SQS spec.

## The complete SQS spec

- **All SQS actions**, one route each, in the agreed form `/#X-Amz-Target=AmazonSQS.SendMessage`, with `x-hotline-action-header: X-Amz-Target` at the top. Generated from AWS's Smithy model (AWS's own API description language), since AWS publishes no OpenAPI for SQS.
- **Every reply** each action can give: `200` with its body, and SQS's error replies (`400` and `500` with `__type` and `message`).
- **Login** named in the security section as the `Authorization` header; as always, its value is never read.
- **Headers to record** marked as agreed: `x-amzn-RequestId` and `x-amzn-query-error` with `x-hotline-redact: false`.
- **Checked as a valid OpenAPI 3.1 file** by a test, and loaded by the proxy's own `validate` command.

This also builds what [proxy-aws.md](proxy-aws.md) agreed but did not build yet: routes named by a header, and the signed call test.

## The example app

One Go program in its own folder and Go module, `examples/aws-sqs-invoices/`, so the proxy's module never depends on the AWS SDK (agreed 2026-10-29). Two commands:

- `request --order A-1001 --template invoice-standard-v2 --language en`: sends the invoice job to the queue.
- `work --out ./invoices`: receives jobs, checks each one itself, writes `A-1001.pdf` to the folder, and deletes the message.

It sets up the SQS client exactly as the AWS setup guide says: the forward proxy set only on the SQS client, the HTTP client turning `https` into `http` after the SDK has signed, and the integration ID added by Go's proxy support. A `README.md` beside it shows how to run it against real AWS by hand.

## End-to-end tests

The detailed rules are tested fast, with the proxy and a fake vendor, in `checks.feature`. The end-to-end tests then prove the whole path once with the real SDK and the real program. Draft scenarios:

```gherkin
# checks.feature: a string that holds JSON
  Scenario Outline: The job message inside an SQS call is checked
    Given the SQS spec with the invoice job schema
    When the app sends SendMessage with the message <message>
    Then the request results are <results>

    Examples:
      | message                                     | results                                                  |
      | a valid job                                 | none                                                     |
      | a job with no orderId                       | body $.MessageBody.orderId required                      |
      | a job whose orderId is 1001, a number       | body $.MessageBody.orderId type string                   |
      | a job whose orderId is a-1001               | body $.MessageBody.orderId pattern                       |
      | a job with no templateId                    | body $.MessageBody.templateId required                   |
      | a job whose templateId is Invoice Standard  | body $.MessageBody.templateId pattern                    |
      | a job whose eventId is not a uuid           | body $.MessageBody.eventId format uuid                   |
      | a job in the language fr                    | body $.MessageBody.language enum en, de, sk, cs          |
      | a job with an extra field priority          | body $.MessageBody.priority additionalProperties         |
      | text that is not JSON                       | body $.MessageBody json                                  |

  Scenario: Each integration is checked against its own message and queue
    Given the invoices integration and a second integration, each with its own SQS spec
    When the second integration sends an invoice job to its own queue
    Then the request results report the message against the second integration's schema
    When the invoices integration sends an invoice job to the second integration's queue
    Then the request results are body $.QueueUrl const

  Scenario: The plain SQS spec does not check the message
    Given the plain SQS spec
    When the app sends SendMessage with the message text that is not JSON
    Then the record has no request results

  Scenario: A bad message is also reported when the worker receives it
    Given the SQS spec with the invoice job schema
    And SQS answers ReceiveMessage with one job that has no orderId
    When the worker sends ReceiveMessage
    Then the response results are body $.Messages[0].Body.orderId required

  Scenario: Results never hold values from the message
    When the app sends SendMessage with a job whose sendTo is secret@example.com
    Then no result holds secret@example.com

# end-to-end.feature
  Scenario: An invoice job goes through Hotline to SQS and back
    Given the fake SQS
    And the hotline-proxy program runs with the integration invoices, used by both the shop and the worker
    When the example app requests the invoice for order A-1001
    And the example app works through the queue
    Then the fake SQS got the job unchanged and its signature parts intact
    And the worker wrote A-1001.pdf and deleted the message
    And the receiver got records for SendMessage, ReceiveMessage and DeleteMessage, each with its own url.template and no results

  Scenario: A broken job is still delivered, and reported
    When the example app sends a job with no orderId
    Then the fake SQS got the job
    And the receiver got a SendMessage record with the result body $.MessageBody.orderId required
```

## Open questions, each with my recommendation

1. **A fake SQS or real AWS for the tests?** I recommend a small fake SQS inside the end-to-end tests: it answers `SendMessage`, `ReceiveMessage` and `DeleteMessage` like SQS (including the MD5 hash of the message, which the SDK checks). Tests then run anywhere, offline, with no AWS account or cost. Running against real AWS stays a manual step in the example's README. (LocalStack, a local copy of AWS run in Docker, is the other choice, but it adds Docker to the tests.)
2. **How do the three specs stay in step with the plain SQS spec?** I recommend generating them: one small generator turns the Smithy model into `aws-sqs.openapi.yaml`, and, for each integration, takes a small file with its queue URL and message schema and writes that integration's spec. A test fails when the committed files differ from what the generator makes. The other choice is letting the config add a schema to fields of a shared spec, which is a new config feature.
3. **What are the other two messages, and should the example show all three?** I recommend the example app and its end-to-end tests cover the invoice integration fully, and `checks.feature` uses a second integration on the same host to prove each one gets only its own schema and queue. The other two message schemas can be added once you name them.
4. **Where does the worker find the order and the template?** I recommend a small built-in list of made-up orders and two built-in templates in the example, so it needs no database or shop. The order's lines stay out of the message, which keeps the message small and the order data in one place.
5. **Where does the PDF go?** I recommend a local folder in this step. Storing it in S3 (and its calls going through Hotline with an S3 spec) is a later step. The PDF is a plain one-page file written with a small PDF library in the example's own module.
6. **Should the worker also refuse bad messages itself?** I recommend yes: Hotline only reports, it never blocks, so the worker checks the message against the same schema and moves a bad one aside (to a "dead letter" queue, the usual SQS place for messages that cannot be handled) instead of retrying it forever.

## Not in this step

- SQS's older query protocol (form bodies with `Action=SendMessage`).
- Other AWS services, including S3 through Hotline.
- Mailing the invoice to `sendTo`.
- Blocking calls whose message fails the check.
