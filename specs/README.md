# Vendor specs

Stable OpenAPI specs of the vendor apis Hotline knows, ready to point an api at in the proxy config (Peter, 2026-10-30).

## Layout

One folder per vendor and api. Each file is named after the vendor's own version of that api:

```
specs/
  aws/
    sqs/
      2012-11-05.openapi.yaml   # SQS, JSON protocol; 2012-11-05 is AWS's version name for the SQS api
      README.md                 # where it came from and how it was made
```

## Rules

1. **Stable.** A file here is only ever fixed, never reshaped. A new version from the vendor is a new file beside the old one.
2. **The vendor's api only.** Nothing that belongs to one customer or one use: no queue URLs, no message schemas, no account IDs. Those go in overlays (short files in the OpenAPI Overlay 1.1 format that change a spec at given spots; see [docs/proxy-overlays.md](../docs/proxy-overlays.md)).
3. **Overlays live with their tests and examples**, never here: in `src/hotline-proxy/testdata/` and in each example's own folder.
4. **Hotline's own marks are included.** The marks the proxy reads, such as `x-hotline-action-header` and `x-hotline-redact`, are part of these files.
5. **Each api's README says where the spec came from** (for SQS: AWS's Smithy model, the file and its commit), how it was made, and on what date.
6. **Every file here passes `hotline-proxy validate`**, checked by a test.
