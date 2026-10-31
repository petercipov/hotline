# AWS SQS, version 2012-11-05

`2012-11-05.openapi.yaml` is the SQS api over AWS's JSON protocol (`application/x-amz-json-1.0`), the one AWS SDKs have used since November 2023. Every call is `POST /`; the action is named in the `X-Amz-Target` header, so each action is a route such as `/#X-Amz-Target=AmazonSQS.SendMessage`, and the spec says `x-hotline-action-header: X-Amz-Target`. Every request and reply body is marked `x-hotline-body: json`, so the proxy reads AWS's content type as JSON.

- **Source:** AWS's Smithy model (AWS's own api description language), `models/sqs/service/2012-11-05/sqs-2012-11-05.json` in [aws/api-models-aws](https://github.com/aws/api-models-aws) at commit `1b9ff13fe6c28ea9774a527e53cda6a3b91369dc`, taken 2026-10-30. A copy is in [tools/aws-openapi/models](../../../tools/aws-openapi/models).
- **Made by:** [tools/aws-openapi](../../../tools/aws-openapi), never by hand. A test fails when this file differs from what the tool makes from the copy of the model.
  ```
  cd tools/aws-openapi
  go run . -record QueueUrl=messaging.destination.name -record-action messaging.operation.name models/sqs-2012-11-05.json > ../../specs/aws/sqs/2012-11-05.openapi.yaml
  ```
- **Replies:** `200` with each action's body, and `4XX` and `5XX` with AWS's JSON error body (`__type`, `message`).
- **Recorded body field:** `QueueUrl` in every action's input is marked `x-hotline-record: messaging.destination.name`, so records name the queue. Every action also lists its `X-Amz-Target` header, with its own value, marked `x-hotline-record: messaging.operation.name`. Detail: [docs/proxy-body-fields.md](../../../docs/proxy-body-fields.md).
- **Recorded headers:** `x-amzn-RequestId` and `x-amzn-query-error` are marked `x-hotline-redact: false`.
- **Login:** the signed `Authorization` header, named in the security section; the proxy never reads it.
- **Not here:** queue URLs and message schemas. Those go in an overlay per queue, next to the test or example that uses it.
