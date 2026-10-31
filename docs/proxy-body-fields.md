# Body fields in the record

*Agreed with Peter and built, 2026-10-31. Scenarios: `src/hotline-proxy/features/record-body-fields.feature`.*

The proxy never records bodies. But some body fields say something useful and safe, such as which SQS queue a message went to: SQS puts the queue address, `QueueUrl`, only in the body. A spec can mark such a field, and the proxy then copies that one field's value into the record.

## The mark

The mark is `x-hotline-record`, on a property of a body schema. Its value is the name of the record attribute.

```yaml
QueueUrl:
  $ref: '#/components/schemas/String'
  x-hotline-record: messaging.destination.name
```

`messaging.destination.name` is OpenTelemetry's standard name for a queue or topic. Grafana shows it as `messaging_destination_name`.

## Rules

1. **Where.** The mark goes on a property of a body schema, in the call or the reply, for JSON bodies (and bodies the spec marks `x-hotline-body: json`) and form bodies, the two kinds the proxy reads for its checks. The proxy follows properties through objects and `$ref`.
2. **Only plain values.** The field must give its type as text (`string`), a number (`number`, `integer`), or true or false (`boolean`); `null` may be listed too. A mark on an object or a list stops the proxy at start.
3. **Never inside a list.** A field inside a list has many values, so it is not copied. Its mark is still checked.
4. **The name.** Letters, digits, dots and underscores only. A name that starts like the proxy's own (`hotline.`, `http.`, `url.`, `server.`, `network.`, `error.`, `dns.`, `tls.`, `log.`) stops the proxy at start, so a copied field can never pass for one the proxy wrote. A name marked in both the call's and the reply's body of one route stops the proxy too.
5. **Copied only when it fits.** The value is copied only when it is there and has the marked type. Otherwise nothing is recorded, and the checks report the problem as usual. A number too large for the record is left out.
6. **Text is cut** at 256 characters.
7. **A promise.** A mark is the spec author's promise that the field holds no secret and no personal data. Mark only fields like queue names, ids of things (not of people) and kinds.
8. **Full records only.** Calls with a known integration and api get these fields. They are signed with the rest of the record ([proxy-signatures.md](proxy-signatures.md)) (Peter, 2026-10-31).
9. **Not in the counter.** The fields go into records only, never into `hotline.requests`, since a field could have very many values and make the counter costly (Peter, 2026-10-31).

## SQS

The shared SQS spec marks `QueueUrl` in every action's input with `x-hotline-record: messaging.destination.name`, so every queue's records name the full queue address, such as `https://sqs.eu-west-1.amazonaws.com/192325781350/invoices` (Peter, 2026-10-31). `tools/aws-openapi` writes the mark when run with `-record QueueUrl=messaging.destination.name`; action outputs are never marked.
