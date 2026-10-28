Feature: Building the request record
  One OpenTelemetry log record per call, built after the reply has gone
  back to the app. Field names follow proxy-spec.md section 4.1.

  Scenario: Full record for a known integration
    Given integration talon-shop-a with an api for api.talon.one
    When the Talon.One sample call ends
    Then event.name is hotline.request
    And hotline.integration.id is talon-shop-a
    And hotline.ingress is forward_proxy
    And http.request.method is PUT and http.response.status_code is 200
    And url.full holds the redacted address
    And url.template is /v2/customer_sessions/{customerSessionId}
    And server.address is api.talon.one and server.port is 443
    And the allowed headers, phase timings, network and TLS fields are recorded
    And hotline.client.cancelled, hotline.connection.reused and hotline.config.version are recorded
    And there is no error.type

  Scenario: It matches the sample in proxy-example
    Given the config and specs from proxy-example
    When the Talon.One sample call ends
    Then the record matches the sample record, apart from the schema errors that come in a later slice

  Scenario: The scheme actually sent
    Given an api with TLS origination on
    When an app sends an http address
    Then url.scheme is https

  @q3
  Scenario: No route in the spec
    Given integration talon-shop-a with an api for api.talon.one
    When a call goes to a path the spec does not know
    Then url.template is left out

  Scenario: Body sizes count the bytes that passed
    Given a call whose Content-Length header says 100 but whose body is 80 bytes
    When the call ends
    Then http.request.body.size is 80

  Scenario: A failed call
    Given a vendor host that does not exist
    When the call ends
    Then error.type is dns_error

  Scenario Outline: Minimal record
    Given <call>
    When the call ends
    Then the record holds only hotline.integration.id, hotline.ingress, server.address, http.request.method, http.response.status_code, error.type and the hotline.timing fields
    And hotline.integration.id is <id>

    Examples:
      | call                                                | id           |
      | a call without Proxy-Authorization                  | not-passed   |
      | a call with the unknown ID talon-shop-x             | talon-shop-x |
      | a call from talon-shop-a to api.stripe.com, outside its apis | talon-shop-a |

  @q12
  Scenario: Fields for later slices are left out
    Given integration talon-shop-a with an api for api.talon.one
    When the Talon.One sample call ends
    Then there are no hotline.schema.request.errors or hotline.schema.response.errors
    And there are no hotline.body.request.hmac_sha256 or hotline.body.response.hmac_sha256

  Scenario: No secret and no body
    Given a call with Authorization, Proxy-Authorization, a cookie, an API key and a JSON body
    When the call ends
    Then none of those values and no byte of either body appears anywhere in the record

  Scenario: The record never slows the call
    Given a record builder that takes 2 seconds
    When an app makes a call
    Then the app gets the reply without waiting for the record
