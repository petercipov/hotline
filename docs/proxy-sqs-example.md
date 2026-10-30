# A Go app sending to AWS SQS through Hotline, with a checked message: requirements draft

*Draft for Peter, 2026-10-30. Nothing here is agreed or built yet. It builds on [proxy-aws.md](proxy-aws.md) (agreed 2026-10-29, not built) and on the spec checks in [proxy-spec-checks.md](proxy-spec-checks.md) (built). Open questions are at the end, each with a recommendation.*

## What Peter asked for

1. A complete, working example: a Go app that sends to AWS SQS through Hotline, with end-to-end tests.
2. A complete OpenAPI spec for SQS, so every SQS call the app makes is checked, not only `SendMessage`.
3. A check of the message itself. One integration sends a JSON message: a job that points to a folder of today's parquet files in S3 (Amazon's file storage). A worker reads the job, adds up the day's data, and writes the result (the "projection", a smaller summary table) back to S3 as parquet.

## The use case, as I understand it

- **The producer** (the app that starts the job) sends one message per day and dataset: "aggregate the folder `s3://shop-data/sales/date=2026-10-30/` and write the result to `s3://shop-data/sales-daily/date=2026-10-30/`".
- **The worker** (the app that does the job) receives the message, reads every parquet file in the input folder, adds the rows up per day (the exact sums are Peter's to name), writes the projection to the output folder, and deletes the message from the queue so it is not handled twice.
- **Hotline** sits between both apps and SQS. It records every call and checks it against the spec, including the job message inside it. It never blocks a call: a broken message still reaches SQS, and the check result shows up in the record.

## The job message (version 1)

A first proposal. Field names are open for Peter to change.

```json
{
  "schemaVersion": 1,
  "jobId": "5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10",
  "date": "2026-10-30",
  "input": "s3://shop-data/sales/date=2026-10-30/",
  "output": "s3://shop-data/sales-daily/date=2026-10-30/",
  "format": "parquet",
  "requestedAt": "2026-10-30T23:05:00Z"
}
```

| Field | Required? | Rule | Why |
|---|---|---|---|
| `schemaVersion` | yes | whole number, exactly `1` | Lets the message change later without breaking old workers. |
| `jobId` | yes | `uuid` | The worker uses it to notice a job it already did (SQS may deliver a message twice). |
| `date` | yes | `date` (`2026-10-30`) | The day being added up. |
| `input` | yes | text starting `s3://`, a bucket name, then a folder ending in `/` | The folder of parquet files to read. |
| `output` | yes | same rule as `input` | Where the projection is written. |
| `format` | no | `parquet` only, for now | Leaves room for other formats later. |
| `requestedAt` | no | `date-time` | When the producer asked; helps when looking into a late job. |

Any other field is reported (`additionalProperties: false`), so a typo like `inptu` is caught.

## How the message sits in the SQS spec

An SQS `SendMessage` body is JSON, but the message itself is a **string** inside it, in the field `MessageBody`:

```json
{"QueueUrl": "https://sqs.eu-west-1.amazonaws.com/123456789012/file-jobs",
 "MessageBody": "{\"schemaVersion\":1,\"jobId\":\"5f0c…\", …}"}
```

So the check has to read the string as JSON, then check that against the job schema. OpenAPI 3.1 already has words for this, from JSON Schema: `contentMediaType` (what the string holds) and `contentSchema` (the schema for what it holds).

```yaml
MessageBody:
  type: string
  contentMediaType: application/json
  contentSchema:
    $ref: '#/components/schemas/FileJob'
```

Requirements:

1. **Strings holding JSON are checked** when the spec says `contentMediaType: application/json` with a `contentSchema`. The JSON Schema library the proxy already uses supports this; it only needs turning on. Without these words, a string stays a plain string, as today.
2. **Places carry on into the inner message.** A missing `date` inside the message is reported as `body $.MessageBody.date required`. A message that is not valid JSON is `body $.MessageBody json`. As always, results never hold a value from the call.
3. **The same in replies.** The worker's `ReceiveMessage` reply holds the messages in `Messages[].Body`; our spec marks those the same way, so a bad message is also reported when the worker receives it (`response body $.Messages[0].Body.date required`).
4. **SQS's content type counts as JSON.** SQS sends `Content-Type: application/x-amz-json-1.0`. Today the proxy only reads `application/json` and `…+json`, so it would skip SQS bodies as `unsupported_content_type`. A spec that lists `application/x-amz-json-1.0` gets it read as JSON. (Only this exact AWS name, plus `1.1`, which other AWS services use.)

### Which integration gets the job schema

The job schema belongs to one app's use of SQS, not to SQS. The config already allows this without any change: two apis may name the same host when different integrations use them (`config.feature`, "The same host in two apis of different integrations").

```yaml
apis:
  aws-sqs:                         # plain SQS, for any app
    hosts: [sqs.eu-west-1.amazonaws.com]
    spec: ./specs/aws-sqs.openapi.yaml
    vendorTimeout: 25s             # above SQS's 20 second long polling
  aws-sqs-file-jobs:               # SQS carrying the file jobs
    hosts: [sqs.eu-west-1.amazonaws.com]
    spec: ./specs/aws-sqs-file-jobs.openapi.yaml
    vendorTimeout: 25s

integrations:
  - id: file-jobs-producer
    apis: [aws-sqs-file-jobs]
  - id: file-jobs-worker
    apis: [aws-sqs-file-jobs]
```

`aws-sqs-file-jobs.openapi.yaml` is the full SQS spec with the job schema added to `MessageBody` (in `SendMessage`, `SendMessageBatch` and the `ReceiveMessage` reply). See question 2 for how the two files stay in step.

## The complete SQS spec

- **All SQS actions**, one route each, in the agreed form `/#X-Amz-Target=AmazonSQS.SendMessage`, with `x-hotline-action-header: X-Amz-Target` at the top. Generated from AWS's Smithy model (AWS's own API description language), since AWS publishes no OpenAPI for SQS.
- **Every reply** each action can give: `200` with its body, and SQS's error replies (`400` and `500` with `__type` and `message`).
- **Login** named in the security section as the `Authorization` header; as always, its value is never read.
- **Headers to record** marked as agreed: `x-amzn-RequestId` and `x-amzn-query-error` with `x-hotline-redact: false`.
- **Checked as a valid OpenAPI 3.1 file** by a test, and loaded by the proxy's own `validate` command.

This also builds what [proxy-aws.md](proxy-aws.md) agreed but did not build yet: routes named by a header, and the signed call test.

## The example app

One Go program in its own folder and Go module, `examples/aws-sqs-file-jobs/`, so the proxy's module never depends on the AWS SDK (agreed 2026-10-29). Two commands:

- `produce`: sends today's job message to the queue.
- `work`: receives messages, checks each one itself, does the job, and deletes the message.

It sets up the SQS client exactly as the AWS setup guide says: the forward proxy set only on the SQS client, the HTTP client turning `https` into `http` after the SDK has signed, and the integration ID added by Go's proxy support. A `README.md` beside it shows how to run it against real AWS by hand.

## End-to-end tests

The detailed rules are tested fast, with the proxy and a fake vendor, in `checks.feature`. The end-to-end tests then prove the whole path once with the real SDK and the real program. Draft scenarios:

```gherkin
# checks.feature: a string that holds JSON
  Scenario Outline: The job message inside an SQS call is checked
    Given the SQS spec with the file job schema
    When the app sends SendMessage with the message <message>
    Then the request results are <results>

    Examples:
      | message                                     | results                                                  |
      | a valid job                                 | none                                                     |
      | a job with no date                          | body $.MessageBody.date required                         |
      | a job whose input is https://… not s3://…   | body $.MessageBody.input pattern                         |
      | a job whose schemaVersion is "1", a string  | body $.MessageBody.schemaVersion type integer            |
      | a job with an extra field priority          | body $.MessageBody.priority additionalProperties         |
      | text that is not JSON                       | body $.MessageBody json                                  |

  Scenario: The plain SQS spec does not check the message
    Given the plain SQS spec
    When the app sends SendMessage with the message text that is not JSON
    Then the record has no request results

  Scenario: A bad message is also reported when the worker receives it
    Given the SQS spec with the file job schema
    And SQS answers ReceiveMessage with one job that has no date
    When the worker sends ReceiveMessage
    Then the response results are body $.Messages[0].Body.date required

  Scenario: Results never hold values from the message
    When the app sends SendMessage with a job whose input is s3://secret-bucket/x/
    Then no result holds secret-bucket

# end-to-end.feature
  Scenario: A job goes through Hotline to SQS and back
    Given the fake SQS
    And the hotline-proxy program runs with the integrations file-jobs-producer and file-jobs-worker
    When the example app produces today's job
    And the example app works through the queue
    Then the fake SQS got the job unchanged and its signature parts intact
    And the worker deleted the message
    And the receiver got records for SendMessage, ReceiveMessage and DeleteMessage, each with its own url.template and no results

  Scenario: A broken job is still delivered, and reported
    When the example app produces a job with no date
    Then the fake SQS got the job
    And the receiver got a SendMessage record with the result body $.MessageBody.date required
```

## Open questions, each with my recommendation

1. **A fake SQS or real AWS for the tests?** I recommend a small fake SQS inside the end-to-end tests: it answers `SendMessage`, `ReceiveMessage` and `DeleteMessage` like SQS (including the MD5 hash of the message, which the SDK checks). Tests then run anywhere, offline, with no AWS account or cost. Running against real AWS stays a manual step in the example's README. (LocalStack, a local copy of AWS run in Docker, is the other choice, but it adds Docker to the tests.)
2. **How does the job's spec stay in step with the plain SQS spec?** I recommend generating both: one small generator turns the Smithy model into `aws-sqs.openapi.yaml`, and, given the job schema file, also writes `aws-sqs-file-jobs.openapi.yaml`. A test fails when the committed files differ from what the generator makes. The other choice is letting the config add a schema to one field of a shared spec, which is a new config feature.
3. **One job schema per integration, or per queue?** I recommend per integration for now (the setup above). If one integration sends different messages to different queues, the spec can choose the schema by `QueueUrl` later, using JSON Schema's `if`/`then`.
4. **`s3://` folder or a ready-made download link?** I recommend `s3://` folders: a parquet folder holds many files, and a pre-signed download link (a link with a time-limited signature in it) works for one file only. The worker reads the files with its own AWS credentials.
5. **How real is the worker?** I recommend that in this step the worker really receives, checks and deletes messages through Hotline, but the parquet reading, adding up and writing to S3 stays behind a small interface with an in-memory version for the tests. The real parquet and S3 part comes next, once you name the sums the projection needs. Its S3 calls would go through Hotline too, with an S3 spec, as a later step.
6. **Should the worker also refuse bad messages itself?** I recommend yes: Hotline only reports, it never blocks, so the worker checks the message against the same schema and moves a bad one aside (to a "dead letter" queue, the usual SQS place for messages that cannot be handled) instead of retrying it forever.

## Not in this step

- SQS's older query protocol (form bodies with `Action=SendMessage`).
- Other AWS services, including S3 through Hotline.
- Blocking calls whose message fails the check.
