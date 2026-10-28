@q10
Feature: Measuring each phase of a call
  Phase timings come from net/http/httptrace and an injected clock, so
  tests need no real waiting. All values are in seconds.

  Scenario: A call on a new https connection
    Given a vendor reached over a new https connection
    When a call ends
    Then the record has hotline.timing.dns, connect, tls, first_byte and total
    And network.peer.address holds the vendor IP
    And dns.question.name holds the name looked up
    And tls.protocol.version holds the vendor's TLS version
    And tls.server.not_after holds when the vendor's certificate expires
    And network.protocol.version holds the HTTP version used to the vendor
    And hotline.timing.proxy holds the proxy's own added time
    And hotline.connection.reused is false

  Scenario: Total ends with the last byte to the app
    Given a vendor reached over a new https connection
    When the last byte of the reply has gone to the app
    Then total ends at that moment

  Scenario: Plain http has no TLS phase
    Given a vendor reached over plain http
    When a call ends
    Then there is no tls timing

  Scenario: An IP address needs no lookup
    Given a vendor whose host is an IP address
    When a call ends
    Then there is no dns timing

  Scenario: A reused connection
    Given a connection to the vendor that is already open
    When a call reuses it
    Then hotline.connection.reused is true
    And there are no dns, connect or tls timings
    And network.peer.address is still recorded

  Scenario Outline: Timings of failed calls
    Given a call where <failure>
    When the call ends
    Then the record has <timings>

    Examples:
      | failure                    | timings                                          |
      | DNS fails                  | the dns timing up to the failure and the total   |
      | the TLS handshake fails    | the dns and connect timings and the total        |
      | the vendor never answers   | no first_byte timing                             |

  Scenario: Parallel connection attempts are not counted twice
    Given a client that tries IPv4 and IPv6 at the same time
    When the phases overlap
    Then nothing is counted twice
