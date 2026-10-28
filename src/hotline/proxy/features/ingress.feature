Feature: Finding the ingress
  The ingress finder works out which ingress a call used. Slice 1 builds
  only the forward proxy ingress: the full vendor address in the request line.

  Scenario Outline: A full address in the request line is the forward proxy ingress
    Given the proxy is running
    When an app sends "PUT <address>"
    Then the call uses the forward proxy ingress
    And the vendor is <host> on port <port>

    Examples:
      | address                                    | host          | port |
      | https://api.talon.one/v2/customer_sessions/1 | api.talon.one | 443  |
      | http://api.talon.one/v2/customer_sessions/1  | api.talon.one | 80   |
      | https://api.talon.one:8443/v2/x            | api.talon.one | 8443 |
      | HTTPS://API.TALON.ONE/v2/x                 | api.talon.one | 443  |

  Scenario: The request line wins over the Host header
    Given the proxy is running
    When an app sends "PUT https://api.talon.one/v2/x" with the header "Host: other.example"
    Then the vendor is api.talon.one

  Scenario Outline: Requests meant for later ingresses
    Given the proxy is running
    When an app sends <request>
    Then the proxy answers 501 Not Implemented with a Proxy-Status header
    And nothing is sent to any vendor

    Examples:
      | request                                             |
      | "GET /https://api.stripe.com/v1/payment_intents"    |
      | "GET /v1/payment_intents" with "Host: api.stripe.com" |
      | "CONNECT api.stripe.com:443"                        |

  Scenario Outline: Addresses the proxy refuses
    Given the proxy is running
    When an app sends "GET <address>"
    Then the proxy refuses the call
    And nothing is sent to any vendor

    Examples:
      | address                    |
      | ftp://files.example/a.txt  |
      | https:///v1/no-host        |
