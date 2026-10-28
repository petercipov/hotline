Feature: Measuring each phase of a call
  Phase timings come from net/http/httptrace and an injected clock. All
  values are in seconds. Each timing is the length of its own phase, except
  first_byte, which counts from when the call reached the proxy. The
  proxy's own time is split in two: before the vendor call starts
  (proxy_request) and after the vendor's reply ends (proxy_response).
  (Peter, 2026-10-28)

  Scenario: A call on a new https connection
    Given a vendor reached over a new https connection
    When a call ends
    Then the record has hotline.timing.dns, connect, tls, first_byte and total
    And first_byte is at least dns, connect and tls together
    And network.peer.address holds the vendor IP
    And dns.question.name holds the name looked up
    And tls.protocol.version holds the vendor's TLS version
    And tls.server.not_after holds when the vendor's certificate expires
    And network.protocol.version holds the HTTP version used to the vendor
    And hotline.timing.proxy_request and proxy_response hold the proxy's own added time
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

  Scenario: A call that gets a freed connection while its own is still being opened
    The connection being opened goes on in the background. Its DNS and
    connect events belong to no call, so they must not change this one.
    Given a call that waits for a new connection while another call holds the only open one
    When the other call ends first and frees its connection
    Then the waiting call's record says hotline.connection.reused true, with no dns, connect or tls timings

  Scenario Outline: Timings of failed calls
    Given a call where <failure>
    When the call ends
    Then the record has <timings>

    Examples:
      | failure                    | timings                                          |
      | DNS fails                  | the dns timing up to the failure and the total   |
      | the TLS handshake fails    | the dns and connect timings and the total        |
      | the vendor never answers   | no first_byte timing                             |

  Scenario: Several addresses are tried one after another
    Given a vendor whose IPv6 address cannot be reached and whose IPv4 address can
    When a call ends
    Then the call goes to the IPv4 address
    And the connect phase is counted once, from the first try until connected

  Scenario: An address that never answers gets only its share of the wait
    The wait is shared between the addresses still to try, as Go's own
    dialer does, so one dead address can't use up all of vendorTimeout.
    Given a vendor whose first address never answers, with a vendorTimeout of 10 seconds
    When a call ends
    Then the call goes to its second address
    And the first address was given 5 seconds, its share of the wait
