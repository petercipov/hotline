# Overlays on an api's spec: requirements draft

*Draft for Peter, 2026-10-30. Nothing here is agreed or built yet. It comes before the SQS example ([proxy-sqs-example.md](proxy-sqs-example.md), pull request #33), which needs it. Open questions are at the end, each with a recommendation.*

## Why

The SQS example has three integrations. All three use the same SQS spec, but each one has its own queue and its own message schema. Copying the whole SQS spec three times would mean three big files to keep in step. Instead, each api keeps the shared spec and adds a small **overlay**.

An overlay is a short file in the official OpenAPI Overlay format. It is a list of actions, and each action says "at this spot in the spec, change this". The spot is found with JSONPath, a small query language for picking parts of a JSON or YAML document (written down in RFC 9535). There are two versions of the format: 1.0 (October 2024) and 1.1 (January 2026), which adds a `copy` action.

An example overlay for the invoices queue:

```yaml
overlay: 1.1.0
info: { title: Invoices queue, version: "1" }
actions:
  - target: $.components.schemas.SendMessageRequest.properties.QueueUrl
    update: { const: https://sqs.eu-west-1.amazonaws.com/123456789012/invoices }
  - target: $.components.schemas.SendMessageRequest.properties.MessageBody
    update:
      contentMediaType: application/json
      contentSchema: { $ref: '#/components/schemas/InvoiceJob' }
  - target: $.components.schemas
    update:
      InvoiceJob: { type: object, required: [orderId, templateId] }
```

## The config

Each api gets an optional `overlays` list, next to its `spec`:

```yaml
apis:
  aws-sqs-invoices:
    hosts: [sqs.eu-west-1.amazonaws.com]
    spec: ./specs/aws-sqs.openapi.yaml
    overlays: [./specs/aws-sqs-invoices.overlay.yaml]
  aws-sqs-refunds:
    hosts: [sqs.eu-west-1.amazonaws.com]
    spec: ./specs/aws-sqs.openapi.yaml
    overlays: [./specs/aws-sqs-refunds.overlay.yaml]
```

## Requirements

1. **Overlays belong to the api.** `overlays` is a list of file paths on an api in the `apis` section, read the same way as `spec`. An integration cannot add overlays (decision 11 of the spec checks: an integration may change only `vendorTimeout`).
2. **Applied once, at load.** When the proxy reads the config, it reads the spec, applies the overlays, and then treats the result exactly like a spec written by hand: every rule for specs applies to it (OpenAPI 3.1 only, `$ref` within the same file, the marks such as `x-hotline-redact` and `x-hotline-action-header`). Calls are checked only against this merged spec.
3. **Order.** The overlays are applied in the order the list gives. Inside one file, the actions run in the order written, each on the result of the one before, as the format says.
4. **Both versions.** Files with `overlay: 1.0.x` and `1.1.x` are read. Actions `update`, `remove` and `copy` (1.1) work as the format says. Anything else, such as `overlay: 2.0.0`, is refused.
5. **Standard JSONPath only.** Targets are read as RFC 9535 JSONPath, for both versions. Tool-specific extras are not accepted.
6. **A target that matches nothing is refused.** The format itself allows it and does nothing. We are stricter: such an overlay is usually stale (the spec changed under it), and a quietly skipped action would mean a check that silently stops happening. This holds for `update`, `remove` and the source of a `copy`.
7. **A broken overlay is a config problem**, like a broken spec: refused at start, listed by `hotline-proxy validate`, and on a later reload the old config stays. Broken means: a file that can't be read, isn't YAML or JSON, or isn't an overlay (no `overlay`, `info` or `actions`); a target that isn't valid JSONPath; a target that matches nothing; an `update` of the wrong kind for its spot (a list onto an object, for example); or a merged spec that breaks a spec rule. Each problem names the api, the overlay file and the action's number, for example `api aws-sqs-invoices: overlay ./specs/aws-sqs-invoices.overlay.yaml, action 2: target $.components.schemas.Nope matches nothing`.
8. **`extends` is ignored.** An overlay file may name the spec it was written for in `extends`. The config already says which spec the overlay goes on, so the proxy does not read `extends`.
9. **Sharing stays cheap.** The base spec file is read once, however many apis use it. Apis with the same spec and the same overlay list in the same order share one merged spec, built once. Memory now grows with the number of different (spec, overlays) pairs, not with the number of integrations.
10. **Records don't change.** `url.template`, the check results and their `place` all come from the merged spec. No new record field: the config's `version`, which every record carries as `hotline.config.version`, covers overlays too, so whoever changes an overlay file changes the config version.
11. **Reload.** Overlays are config: whenever the config is read again (the reload slice, still to come), its overlay files are read again with it.

## Library

I recommend [`github.com/speakeasy-api/openapi/overlay`](https://pkg.go.dev/github.com/speakeasy-api/openapi/overlay), MIT license. It supports 1.0 and 1.1 with `copy`, has a strict mode that reports a target matching nothing, and works on `go.yaml.in/yaml/v3` nodes, the same YAML library the proxy already uses; it also uses the same JSON Schema library. I tried it on the SQS case above: the three updates landed where expected, and a target that matched nothing gave a clear error in strict mode. Two things to handle on our side: it reads 1.0 files with an older JSONPath by default, so the proxy sets RFC 9535 for every file; and its own docs still call the overlay part early ("alpha"), so our tests cover each rule above.

The other choices: `pb33f/libopenapi` (also supports 1.1, but brings its own YAML library and a whole OpenAPI model we don't use), or our own small code on top of a JSONPath library (more code to own, for no gain).

## What changes in the proxy spec

Section 2 (config file): the `apis` example gains `overlays`, and a new rule says what overlays are and requirements 2, 3, 6 and 7 in short. Proposed in a pull request to `hotline-strategic`.

## Draft scenarios

```gherkin
Feature: Overlays on an api's spec

  Scenario: An overlay changes what is checked
    Given the api stripe with the spec stripe.openapi.yaml
    And the overlay that makes the query value limit at most 10
    When the app sends GET /v1/customers?limit=50
    Then the request results are query limit maximum 10

  Scenario: The plain spec is unchanged for another api
    Given the apis invoices and refunds share one SQS spec, each with its own overlay
    When the refunds integration sends a message to the invoices queue
    Then the request results are body $.QueueUrl const
    When the invoices integration sends a message to the invoices queue
    Then the record has no request results

  Scenario: Overlays are applied in the order listed
    Given an api with two overlays that both set the maximum of limit, first to 10, then to 20
    When the app sends GET /v1/customers?limit=15
    Then the record has no request results

  Scenario: An overlay can add a route
    Given the overlay that adds the route GET /v1/balance
    When the app sends GET /v1/balance
    Then the record has url.template /v1/balance

  Scenario: An overlay can mark a header to keep
    Given the overlay that marks the reply header x-amzn-RequestId with x-hotline-redact false
    When the vendor replies with x-amzn-RequestId abc
    Then the record has the reply header x-amzn-RequestId abc

  Scenario: An overlay can remove a route
    Given the overlay that removes the route DELETE /v1/customers/{id}
    When the app sends DELETE /v1/customers/cus_1
    Then the record has no url.template

  Scenario: An overlay can copy a schema
    Given the overlay that copies the schema Customer onto the reply of GET /v1/me
    When the vendor replies to GET /v1/me with a customer with no id
    Then the response results are body $.id required

  Scenario Outline: A broken overlay stops the proxy at start
    Given the api stripe with the overlay <overlay>
    When the proxy starts
    Then it refuses the config with <problem>
    And hotline-proxy validate lists the same problem

    Examples:
      | overlay                                   | problem                                                     |
      | a file that is not there                  | overlay ./o.yaml cannot be read                             |
      | a file that is not YAML                   | overlay ./o.yaml is not an overlay document                 |
      | a file with no actions                    | overlay ./o.yaml is not an overlay document                 |
      | a file with overlay 2.0.0                 | overlay ./o.yaml: version 2.0.0 is not supported            |
      | a target that is not valid JSONPath       | action 1: target $.paths[ is not valid JSONPath             |
      | a target that matches nothing             | action 1: target $.components.schemas.Nope matches nothing  |
      | a remove that matches nothing             | action 1: target $.paths['/gone'] matches nothing           |
      | a copy whose source matches nothing       | action 1: copy $.components.schemas.Nope matches nothing    |
      | a list onto an object                     | action 1: update is a list but the target is an object      |
      | a $ref to another file                    | the spec's own problem, with the api and overlay named      |
      | an openapi version changed to 3.0.3       | the spec's own problem, with the api and overlay named      |

  Scenario: The format's tool-specific JSONPath is not used
    Given an overlay with overlay 1.0.0 whose target uses the in keyword
    When the proxy starts
    Then it refuses the config with a JSONPath problem

  Scenario: extends is ignored
    Given an overlay whose extends names another file
    When the proxy starts
    Then the overlay is applied to the api's own spec

  Scenario: An integration cannot add overlays
    Given the integration shop-a names { api: stripe, overlays: [./o.yaml] }
    When the proxy starts
    Then the config is loaded with the warning that only vendorTimeout can be changed per integration
```

## Open questions, each with my recommendation

1. **A target that matches nothing: refuse, or only warn?** I recommend refusing at start (requirement 6), as you suggested. The format allows it, so an overlay that works in other tools might be refused by us; the error names the action, so it is easy to fix.
2. **Overlays on the api only, or also on an integration?** I recommend on the api only. Three queues are then three short api entries that repeat `hosts`, but the rule "an integration may change only `vendorTimeout`" stays simple, and one integration never sees a different spec from another on the same api.
3. **Which library?** I recommend `speakeasy-api/openapi/overlay` (above).
4. **Both versions 1.0 and 1.1?** I recommend both, read with standard JSONPath. 1.1 is a small step up and adds `copy`, which saves repeating a schema.

## Not in this step

- Overlays fetched from a web address; only local files.
- Overlays written inside the config file itself.
- A command that prints the merged spec. Easy to add later if debugging an overlay needs it.
