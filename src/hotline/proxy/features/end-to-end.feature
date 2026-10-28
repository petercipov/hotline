@e2e
Feature: Calling through the forward proxy ingress, end to end
  Real clients call the real proxy binary, which calls a local TLS test
  vendor; a fake OTLP receiver collects what the proxy sends. The clients
  come from the research folder (matrix-clients/go and node); case numbers
  are those in egress-proxy-options.md. Only cases 1, 2, 4 and 5 put a full
  address in the request line.

  Background:
    Given the proxy runs with integration test-shop for the test vendor
    And a fake OTLP receiver

  Scenario Outline: The client matrix
    When the <client> client sends case <case>: <how>
    Then the app gets the test vendor's status, headers and body unchanged
    And the test vendor got the call over https and never saw Proxy-Authorization
    And the receiver got one hotline.request record with hotline.ingress forward_proxy and ID test-shop
    And the receiver saw hotline.requests go up by one
    And the record holds the redacted url.full and no secret

    Examples:
      | client | case | how                                                            |
      | Go     | 1    | POST https://… over plain HTTP/1.1, using URL.Opaque           |
      | Go     | 2    | POST http://… through Transport.Proxy                          |
      | Node   | 1    | POST https://… with http.request and a full URL as the path    |
      | Node   | 2    | POST http://… with http.request                                |

  @later
  Scenario Outline: The client matrix over TLS to the proxy
    When the <client> client sends case <case>: <how>
    Then the app gets the test vendor's answer

    Examples:
      | client | case | how                                                       |
      | Go     | 4    | the https address over TLS to the proxy, using URL.Opaque |
      | Go     | 5    | the http address over TLS through Transport.Proxy         |
      | Node   | 4    | https.request with servername set to the proxy            |
      | Node   | 5    | the http address with https.request                       |

  Scenario Outline: Failures end to end
    When the Go client calls <target>
    Then <result>

    Examples:
      | target                                       | result                                                                    |
      | a vendor with an untrusted certificate       | the app gets 502 with Proxy-Status and the record's error.type is tls_certificate_error |
      | a host that does not exist                   | the app gets 502 with Proxy-Status and the record's error.type is dns_error |
      | the test vendor without Proxy-Authorization  | the call reaches the vendor and the record is minimal with ID not-passed  |
