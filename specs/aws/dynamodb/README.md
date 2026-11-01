# AWS DynamoDB, version 2012-08-10

`2012-08-10.openapi.yaml` is the DynamoDB api over AWS's JSON protocol (`application/x-amz-json-1.0`). Every call is `POST /`; the action is named in the `X-Amz-Target` header, so each action is a route such as `/#X-Amz-Target=DynamoDB_20120810.<Action>`, and the spec says `x-hotline-action-header: X-Amz-Target`. Every request and reply body is marked `x-hotline-body: json`.

- **Source:** AWS's Smithy model (AWS's own api description language), `models/dynamodb/service/2012-08-10/dynamodb-2012-08-10.json` in [aws/api-models-aws](https://github.com/aws/api-models-aws) at commit `1b9ff13fe6c28ea9774a527e53cda6a3b91369dc`, taken 2026-11-01. A copy is in [tools/aws-openapi/models](../../../tools/aws-openapi/models).
- **Made by:** [tools/aws-openapi](../../../tools/aws-openapi), never by hand. A test fails when this file differs from what the tool makes from the copy of the model.
  ```
  cd tools/aws-openapi
  go run . -record TableName=db.collection.name -record-action db.operation.name models/dynamodb-2012-08-10.json > ../../specs/aws/dynamodb/2012-08-10.openapi.yaml
  ```
- **Patterns:** AWS writes its text patterns for Java. The tool rewrites the ones Go can read differently (`\u0020`, `\p{Print}`); a pattern Go can't read at all (a look ahead, a repeat above 1000) is left out and kept as `x-aws-pattern`, unchecked.
- **Replies:** `200` with each action's body, and `4XX` and `5XX` with AWS's JSON error body (`__type`, `message`).
- **Recorded:** `TableName` in every action's input is marked `x-hotline-record: db.collection.name`, and the `X-Amz-Target` header `db.operation.name`, the OpenTelemetry names for databases.
- **Login:** the signed `Authorization` header, named in the security section; the proxy never reads it.
- **Not here:** table names and item shapes. Those go in an overlay per table, next to the test or example that uses it.
