# The shop example: four apps on AWS, through Hotline

*Agreed with Peter, 2026-11-01; step 1 built the same day. Scenarios: `src/hotline-proxy/features/aws-shop.feature` (the shop's apis, and the whole story through the proxy) and `end-to-end-aws.feature`. The example is [examples/aws-shop](../examples/aws-shop/README.md). It grows the invoice example of [proxy-sqs-example.md](proxy-sqs-example.md) and [proxy-xml-bodies.md](proxy-xml-bodies.md).*

## What Peter asked for

More examples, for AWS only for now (Peter, 2026-11-01): DynamoDB, EventBridge, Kinesis and the other most used AWS apis, in one story a reader can follow.

## Decisions

1. **One program, four apps.** `examples/aws-sqs-invoices` is renamed `examples/aws-shop`, one Go program with a group of commands per app: `checkout`, `invoices`, `mailer`, `refunds` (Peter, 2026-11-01: "1 single program").
2. **Each app is its own integration.** Its name is the user name in the proxy address, so the records say which app made each call. Each integration lists only the apis that app uses.
3. **One script runs the whole story.** `run-shop.sh`: checkout pays, invoices makes the invoice, the mailer mails it, refunds starts a credit note, then two mistakes on purpose (a broken invoice job; an order written to a table that is not checkout's own), then a summary. It stops at the first failing step (Peter, 2026-11-01: "one sh script for one complete scenario").
4. **Existing AWS resources only.** The program and the script never create or remove anything. Only the queue `invoices` exists so far; Peter creates the rest with the names in the README, which lists the AWS commands to do it (Peter, 2026-11-01).
5. **One api per AWS resource**, each with the stable spec for its service from `specs/aws` and an overlay naming the resource and the shape of what is sent to it. The overlays live in `src/hotline-proxy/testdata/aws/`.
6. **Step 1** adds the JSON apis DynamoDB, EventBridge, Kinesis, Secrets Manager, SSM Parameter Store, KMS, CloudWatch Logs and Step Functions, beside SQS and S3. Until SNS (step 3), checkout puts the invoice job on SQS itself.

## What the proxy learned for it

- **Base64 text holding JSON.** Kinesis sends each record as base64 text. A string schema with `contentEncoding: base64` beside `contentMediaType` and `contentSchema` is decoded first, then checked. Text that is not base64 is reported with the rule `base64`.
- **AWS's patterns readable by Go.** AWS's models write patterns for Java's regular expressions. `tools/aws-openapi` rewrites the forms Go can't read (`\uXXXX`, `\p{Print}`), and keeps any pattern Go still can't read as `x-aws-pattern`, which the proxy doesn't check.
- **Body fields in the record.** DynamoDB records `TableName` as `db.collection.name` and the action as `db.operation.name`; Kinesis records `StreamName` as `messaging.destination.name`; the others record the action as `rpc.method`.

## Tests

- The program's own tests use small fakes that also play the forward proxy.
- `aws-shop.feature` checks each new api's overlay, and runs the whole story with the real program through the proxy inside the test, with a fake AWS for every host. Like the shop's EventBridge rule, the fake puts each `InvoiceReady` event on the mail queue.
- `end-to-end-aws.feature` runs checkout through the real hotline-proxy program to a fake SQS over TLS. Only the invoices queue can be reached this way: one integration can't name two apis with the same host, so the shop's other apis can't all be on `localhost`.

## Still to come

The later steps add the rest of the shop's apis: SNS between checkout and the invoices queue, STS, Lambda, Bedrock, and SES for the mailer. SNS and STS use AWS's older form, with the action in a form field, so the proxy will need routes named by a body field. Lambda, SES and Bedrock use plain REST.
