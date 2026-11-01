# Example: a Go app using AWS SQS and S3 through Hotline

A shop asks for the invoice PDF of an order by putting a job on an SQS queue. A worker takes the job off the queue, renders the PDF with the template the job names, stores it in an S3 bucket (AWS's file storage), and deletes the message. The shop then fetches, lists and cleans up the PDFs in the bucket. Everything talks to AWS through Hotline, with the AWS SDK for Go v2.

This is its own Go module, so the proxy never depends on the AWS SDK.

## The job message

One JSON object per message, version 1. Its schema is [invoice/invoice-job.schema.json](invoice/invoice-job.schema.json).

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

Everything is required except `sendTo`, and no other field is allowed. The worker checks every message against the schema too, because Hotline only reports a bad message and never stops it.

## Commands

```
aws-sqs-invoices request --order A-1001 --template invoice-standard-v2 --language en
aws-sqs-invoices send --body '{"schemaVersion":1}'     # any text, unchanged: to try Hotline's checks
aws-sqs-invoices work (--bucket shop-invoices | --out ./invoices) [--once]
aws-sqs-invoices fetch --order A-1001 --out A-1001.pdf
aws-sqs-invoices list
aws-sqs-invoices cleanup --before 2026-11-01
```

Every command takes `--queue` (or `INVOICES_QUEUE_URL`), `--bucket` (or `INVOICES_BUCKET`), `--proxy` (or `HOTLINE_PROXY`), `--endpoint` (or `SQS_ENDPOINT`) and `--s3-endpoint` (or `S3_ENDPOINT`). The AWS region and credentials come from the usual AWS settings: `AWS_REGION`, `AWS_PROFILE`, the pod's role, and so on. Exit codes: 0 done, 1 failed, 2 a wrong command line.

The worker:

- stores the PDF in the bucket as `<orderId>.pdf`, such as `A-1001.pdf`, and deletes the message. With `--out` instead of `--bucket`, it writes `invoice-<orderId>.pdf` to that folder, creating the folder if needed;
- deletes a message whose `eventId` it already handled, without writing again, since SQS may deliver a message twice;
- leaves a message it can't use on the queue (broken JSON, a schema mistake, an unknown order or template). Give the queue a dead letter queue (a second queue where SQS moves messages that keep failing) with a `maxReceiveCount`, so such messages move aside instead of coming back forever.
- uses long polling (SQS holds each receive call up to 20 seconds), so Hotline's `vendorTimeout` for SQS must be above 20 seconds. With `--once` it stops as soon as the queue is empty.

The shop's S3 commands:

- `fetch` saves an order's PDF to a file, and says plainly when there is none (S3 answers `404` with the XML error `NoSuchKey`);
- `list` prints each PDF's name, size and date, one per line (`ListObjectsV2`, an XML reply);
- `cleanup` deletes every PDF older than the given day, up to 1000 per call (`DeleteObjects`, an XML call).

The orders and the two templates (`invoice-standard-v2`, `invoice-compact-v1`) are built in, so the example needs no database.

## How the SQS and S3 clients reach Hotline

The code is in [invoice/queue.go](invoice/queue.go) and [invoice/files.go](invoice/files.go). Both clients share the same HTTP client. It follows the AWS setup guide in [docs/proxy-aws.md](../../docs/proxy-aws.md):

- only the SQS and S3 clients use the proxy, so the SDK's own credential lookups never reach Hotline;
- the SDK keeps its normal `https` address and signs the call; the app's HTTP client then turns `https` into `http`, which is not part of the signature, so the call reaches the forward proxy as a plain request Hotline can read. Hotline sends it on to AWS over `https`;
- the integration ID is the proxy address's user name (`http://invoices:x@hotline:8080`). Go adds it as `Proxy-Authorization` after signing, so Hotline can remove it without breaking the signature.

## Running it against real AWS

You need an AWS account, a queue, a bucket, and Hotline running with an integration `invoices` whose apis list `sqs.<region>.amazonaws.com` and `<bucket>.s3.<region>.amazonaws.com` (S3 puts the bucket in the host name). The end-to-end test's Hotline config does this, and is a good start:

- [hotline-proxy.yaml](../../src/hotline-proxy/testdata/aws/hotline-proxy.yaml): the proxy config, with the api `aws-sqs-invoices` and the integration `invoices`. It sends records and the proxy's own numbers to an OpenTelemetry collector on `localhost:4317`;
- [invoices.key](../../src/hotline-proxy/testdata/aws/invoices.key): the key that signs the integration's records. It is an example and not secret: put your own secret in it, or read it from the environment with `fingerprintKey: { env: NAME }`;
- [invoices.overlay.yaml](../../src/hotline-proxy/testdata/aws/invoices.overlay.yaml): the overlay that adds this queue's URL and the job schema to the stable SQS spec in [specs/aws/sqs](../../specs/aws/sqs/);
- [invoices-bucket.overlay.yaml](../../src/hotline-proxy/testdata/aws/invoices-bucket.overlay.yaml): the overlay that adds the bucket's name and the PDF name rule to the stable S3 spec in [specs/aws/s3](../../specs/aws/s3/).

The overlay names the queue `https://sqs.eu-west-1.amazonaws.com/192325781350/invoices`, so change the account number and region there (and the host in the config) to yours. Bucket names are shared by all of AWS, so `shop-invoices` is likely taken: pick your own, and change it in the bucket overlay and the config's S3 host too.

```
go run ../../src/hotline-proxy/cmd run --config ../../src/hotline-proxy/testdata/aws/hotline-proxy.yaml   # Hotline, in its own terminal
aws sqs create-queue --queue-name invoices
aws s3 mb s3://<bucket> --region eu-west-1
export AWS_REGION=eu-west-1
export INVOICES_QUEUE_URL=https://sqs.eu-west-1.amazonaws.com/<account>/invoices
export INVOICES_BUCKET=<bucket>
export HOTLINE_PROXY=http://invoices:x@localhost:8080

go run . request --order A-1001 --template invoice-standard-v2 --language en
go run . work --once
go run . list
go run . fetch --order A-1001 --out A-1001.pdf
go run . cleanup --before 2099-01-01
```

Without Hotline, leave `HOTLINE_PROXY` empty. For a local SQS such as LocalStack or ElasticMQ, set `SQS_ENDPOINT=http://localhost:4566`, and `S3_ENDPOINT` the same way for a local S3; with an S3 endpoint set, the bucket goes in the path instead of the host name.

## Tests

`go test ./...` here. The tests use a small fake SQS ([internal/fakesqs](internal/fakesqs/fakesqs.go)) that also plays Hotline's forward proxy, so they check what the app sends to Hotline: the `http` request line, `Proxy-Authorization`, the signed `Authorization`, and `X-Amz-Target`. A fake S3 ([internal/fakes3](internal/fakes3/fakes3.go)) does the same for S3, including uploads signed in parts with a checksum at the end. They need no AWS account and no network. The proxy's own tests (`src/hotline-proxy/features/aws-s3.feature`) run this app's S3 commands through Hotline.
