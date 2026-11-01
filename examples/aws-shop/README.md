# Example: a small shop on AWS, through Hotline

A small shop made of four apps, each its own Hotline integration, all in one Go program, `aws-shop`, using the AWS SDK for Go v2:

| App | What it does | AWS it uses |
|---|---|---|
| `checkout` | takes a payment: reads the payment key, saves the order, announces it, and asks for the invoice | Secrets Manager, DynamoDB `orders`, Kinesis, SQS `invoices` |
| `invoices` | makes the invoice: reads the default template, renders the PDF, stores it, signs it, saves its row, announces it | SQS `invoices`, SSM, S3, KMS, DynamoDB `invoices`, EventBridge, Kinesis |
| `mailer` | mails the invoice: takes the announcement off its queue and writes the email, with the PDF attached | SQS `mail`, S3 |
| `refunds` | starts a credit note for a refund | DynamoDB `orders`, Step Functions |

Every app also writes its log lines to CloudWatch Logs, in a log stream named after the app. Each app puts its own name in the proxy address as the user name (`http://checkout:x@localhost:8080`), so Hotline's records say which app made each call. This is its own Go module, so the proxy never depends on the AWS SDK.

## The whole story in one go

[run-shop.sh](run-shop.sh) runs it all against the AWS resources that already exist, through Hotline: checkout pays order A-1001, invoices makes the invoice, the mailer mails it, refunds starts a credit note, and then two mistakes on purpose (a broken invoice job, and an order written to a table that is not checkout's own). It prints a summary, and stops at the first step that fails. It never creates or removes anything.

```
go run ../../src/hotline-proxy/cmd run --config ../../src/hotline-proxy/testdata/aws/hotline-proxy.yaml   # Hotline, in its own terminal
export AWS_PROFILE=<your profile> AWS_REGION=eu-west-1
HOTLINE_PROXY=http://localhost:8080 ./run-shop.sh
```

The steps should have no results in Hotline's records. The broken job should be reported with `body $.MessageBody.eventId required` and more, and the wrong table with `body $.TableName const orders`.

## Commands

```
aws-shop checkout pay --order A-1001 --language en [--template default] [--send-to EMAIL]
aws-shop checkout send --body TEXT          # any text to the invoices queue, unchanged: to try Hotline's checks
aws-shop invoices work [--once] [--out FOLDER]
aws-shop invoices status --order A-1001
aws-shop invoices fetch --order A-1001 --out FILE
aws-shop invoices list
aws-shop invoices cleanup --before 2026-11-01
aws-shop mailer work --out FOLDER [--once]
aws-shop refunds start --order A-1001 --amount 10.00 --reason returned
```

Every command takes `--proxy` (or `HOTLINE_PROXY`), `--endpoint` (or `SQS_ENDPOINT`, a local SQS) and `--s3-endpoint` (or `S3_ENDPOINT`, a local S3). The AWS region and credentials come from the usual AWS settings. Exit codes: 0 done, 1 failed, 2 a wrong command line.

- `invoices work` takes jobs off the queue. With `--out` it writes `invoice-<order>.pdf` to that folder instead of the bucket. It deletes a job it already handled, since SQS may deliver a message twice, and leaves a job it can't use on the queue. Give the queue a dead letter queue (a second queue where SQS moves messages that keep failing), so such jobs move aside. It waits up to 20 seconds for each receive, so Hotline's `vendorTimeout` for SQS must be above that. With `--once` it stops when the queue is empty.
- `mailer work` writes `<order>.eml` to its folder. Sending it with SES comes in a later step.
- The orders and the two templates (`invoice-standard-v2`, `invoice-compact-v1`) are built in.

## The AWS resources

All in `eu-west-1`, in account `192325781350`. Each name comes from its environment variable, else the name here:

| Resource | Name | Variable |
|---|---|---|
| SQS queue for invoice jobs | `invoices` | `INVOICES_QUEUE_URL` (the whole URL) |
| SQS queue for the mailer | `mail` | `MAIL_QUEUE_URL` |
| S3 bucket | `shop-invoices-192325781350` | `INVOICES_BUCKET` |
| DynamoDB tables | `orders`, `invoices` | `ORDERS_TABLE`, `INVOICES_TABLE` |
| Kinesis stream | `shop-events` | `SHOP_STREAM` |
| EventBridge bus | `shop` | `SHOP_BUS` |
| Secrets Manager secret | `shop/payment-key` | `PAYMENT_SECRET` |
| SSM parameter | `/shop/invoices/default-template` | `TEMPLATE_PARAMETER` |
| KMS signing key | `alias/invoice-signing` | `SIGNING_KEY` |
| Step Functions workflow | `credit-note` | `CREDIT_NOTE_WORKFLOW` (the whole ARN) |
| CloudWatch Logs group | `/shop/aws-shop` | `SHOP_LOG_GROUP` (empty: no CloudWatch Logs) |

To create the ones that don't exist yet (the queue `invoices` does). The Kinesis stream and the KMS key cost money while they exist.

```
export AWS_REGION=eu-west-1 ACCOUNT=192325781350

aws sqs create-queue --queue-name mail
aws s3 mb s3://shop-invoices-$ACCOUNT
for table in orders invoices; do
  aws dynamodb create-table --table-name $table --billing-mode PAY_PER_REQUEST \
    --attribute-definitions AttributeName=orderId,AttributeType=S --key-schema AttributeName=orderId,KeyType=HASH
done
aws kinesis create-stream --stream-name shop-events --stream-mode-details StreamMode=ON_DEMAND
aws secretsmanager create-secret --name shop/payment-key --secret-string "any value, for the example"
aws ssm put-parameter --name /shop/invoices/default-template --type String --value invoice-standard-v2
key=$(aws kms create-key --key-spec ECC_NIST_P256 --key-usage SIGN_VERIFY --query KeyMetadata.KeyId --output text)
aws kms create-alias --alias-name alias/invoice-signing --target-key-id $key
aws logs create-log-group --log-group-name /shop/aws-shop

# The bus, and a rule that puts each InvoiceReady event on the mail queue.
aws events create-event-bus --name shop
aws events put-rule --name invoice-ready-to-mail --event-bus-name shop \
  --event-pattern '{"source":["shop.invoices"],"detail-type":["InvoiceReady"]}'
aws events put-targets --rule invoice-ready-to-mail --event-bus-name shop \
  --targets Id=mail,Arn=arn:aws:sqs:$AWS_REGION:$ACCOUNT:mail
aws sqs set-queue-attributes --queue-url https://sqs.$AWS_REGION.amazonaws.com/$ACCOUNT/mail --attributes "{\"Policy\":\"{\\\"Statement\\\":[{\\\"Effect\\\":\\\"Allow\\\",\\\"Principal\\\":{\\\"Service\\\":\\\"events.amazonaws.com\\\"},\\\"Action\\\":\\\"sqs:SendMessage\\\",\\\"Resource\\\":\\\"arn:aws:sqs:$AWS_REGION:$ACCOUNT:mail\\\",\\\"Condition\\\":{\\\"ArnEquals\\\":{\\\"aws:SourceArn\\\":\\\"arn:aws:events:$AWS_REGION:$ACCOUNT:rule/shop/invoice-ready-to-mail\\\"}}}]}\"}"

# The credit note workflow: for now a single step that passes the refund on.
aws iam create-role --role-name credit-note-workflow \
  --assume-role-policy-document '{"Statement":[{"Effect":"Allow","Principal":{"Service":"states.amazonaws.com"},"Action":"sts:AssumeRole"}]}'
aws stepfunctions create-state-machine --name credit-note --role-arn arn:aws:iam::$ACCOUNT:role/credit-note-workflow \
  --definition '{"StartAt":"CreditNote","States":{"CreditNote":{"Type":"Pass","End":true}}}'
```

## The Hotline config

[hotline-proxy.yaml](../../src/hotline-proxy/testdata/aws/hotline-proxy.yaml) is the config the proxy's tests use, and a good start. Each AWS resource is an api, with the stable spec for its AWS service from [specs/aws](../../specs/aws/) and an overlay naming that resource and the shape of what is sent to it, such as [orders-table.overlay.yaml](../../src/hotline-proxy/testdata/aws/orders-table.overlay.yaml). Each app is an integration listing only the apis it uses. [shop.key](../../src/hotline-proxy/testdata/aws/shop.key) signs the records; it is an example and not secret, so put your own secret there, or use `fingerprintKey: { env: NAME }`. The overlays name the resources above, so change them for another account or region.

## How the clients reach Hotline

The code is in [shop/shop.go](shop/shop.go) and [invoice/queue.go](invoice/queue.go), following [docs/proxy-aws.md](../../docs/proxy-aws.md):

- only the clients of the shop's own resources use the proxy, so the SDK's credential lookups never reach Hotline;
- the SDK keeps its normal `https` address and signs the call; the app's HTTP client then turns `https` into `http`, which is not signed, so the call reaches the forward proxy as a plain request Hotline can read. Hotline sends it on to AWS over `https`;
- the integration ID is the proxy address's user name. Go adds it as `Proxy-Authorization` after signing, so Hotline can remove it without breaking the signature.

## Tests

`go test ./...` here. Small fakes of SQS ([internal/fakesqs](internal/fakesqs/fakesqs.go)), S3 ([internal/fakes3](internal/fakes3/fakes3.go)) and the other AWS apis ([internal/fakeaws](internal/fakeaws/fakeaws.go)) also play Hotline's forward proxy, so the tests check what each app sends: the request line, `Proxy-Authorization`, the signed `Authorization` and `X-Amz-Target`. They need no AWS account and no network.

The proxy's own tests run this program through Hotline: `aws-shop.feature` runs the whole story with a fake AWS for every host, and `end-to-end-aws.feature` runs checkout through the real hotline-proxy program to a fake SQS over TLS.
