# AWS S3, version 2006-03-01

`2006-03-01.openapi.yaml` is the S3 api over AWS's REST and XML protocol (AWS calls it `restXml`). Actions are told apart by their method, their path and words in the query, and bodies are XML.

- **Source:** AWS's Smithy model (AWS's own api description language), `models/s3/service/2006-03-01/s3-2006-03-01.json` in [aws/api-models-aws](https://github.com/aws/api-models-aws) at commit `1b9ff13fe6c28ea9774a527e53cda6a3b91369dc`, taken 2026-10-31. A copy is in [tools/aws-openapi/models](../../../tools/aws-openapi/models).
- **Made by:** [tools/aws-openapi](../../../tools/aws-openapi), never by hand. A test fails when this file differs from what the tool makes from the copy of the model.
  ```
  cd tools/aws-openapi
  go run . models/s3-2006-03-01.json > ../../specs/aws/s3/2006-03-01.openapi.yaml
  ```
- **The bucket is in the host name** (`shop-invoices.s3.eu-west-1.amazonaws.com`), as AWS's SDKs send it, so it is not in any route. An api in the config names the bucket's host. Actions that work on no bucket (`ListBuckets`, `ListDirectoryBuckets`, `WriteGetObjectResponse`) go to other hosts and are left out.
- **Routes:** each action is its own route. A word AWS writes in the query goes after `#`, such as `/#?delete` for `DeleteObjects` and `/{Key+}#?tagging` for `GetObjectTagging`. `{Key+}` is the file name, which takes the rest of the path, slashes included. Detail: [docs/proxy-xml-bodies.md](../../../docs/proxy-xml-bodies.md).
- **`x-id`:** AWS's SDKs add `x-id=<action>` to some calls. It is left out of a route where the route is clear without it (then listed as an optional query value), and kept where actions would share a route without it, such as `GetObject` and `ListParts` (`/{Key+}#?x-id=GetObject`). So a call without `x-id`, such as one made by hand, may find no route for those actions.
- **Bodies:** XML bodies are marked `x-hotline-body: xml` and carry OpenAPI `xml` notes made from AWS's tags (`xmlName`, `xmlAttribute`, `xmlFlattened`). Files and other whole bodies (`PutObject`, `GetObject`, a bucket policy) are listed as `*/*` with no schema: they pass and are never read.
- **Replies:** each action's own status with its headers and body, and `4XX` and `5XX` with AWS's XML error body (`<Error><Code>…`).
- **Recorded headers:** `x-amz-request-id` and `x-amz-id-2` are marked `x-hotline-redact: false`.
- **Login:** the signed `Authorization` header, named in the security section; the proxy never reads it.
- **Not here:** bucket names and file name rules. Those go in an overlay per bucket, next to the test or example that uses it.
