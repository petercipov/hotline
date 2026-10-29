Feature: Guarding against calls to internal addresses
  The proxy refuses to connect to internal IPs, so nobody can use it to
  reach the cloud metadata address, cluster services or the proxy itself
  (server side request forgery). The check is on the IP after DNS. An api
  can allow internal IPs for its own hosts (Peter, 2026-10-29).

  Scenario Outline: A vendor name that resolves to an internal IP is refused
    Given the vendor name api.talon.one resolves to <ip>
    When an app sends "GET https://api.talon.one/v2/x"
    Then the proxy answers 403 with "Proxy-Status: hotline; error=destination_ip_prohibited"
    And the proxy opens no connection
    And the record's error.type is destination_ip_prohibited

    Examples:
      | ip              |
      | 127.0.0.1       |
      | ::1             |
      | 169.254.169.254 |
      | fe80::1         |
      | 10.1.2.3        |
      | 172.16.0.5      |
      | 192.168.1.10    |
      | fd00::5         |
      | 100.64.0.1      |
      | 0.0.0.0         |
      | ::ffff:127.0.0.1 |

  Scenario Outline: An internal IP in the address is refused on every ingress
    When an app sends <request>
    Then the proxy answers 403 with "Proxy-Status: hotline; error=destination_ip_prohibited"
    And the proxy opens no connection

    Examples:
      | request                                                                  |
      | "GET http://169.254.169.254/latest/meta-data/"                           |
      | "GET /http://169.254.169.254/latest/meta-data/" with "Host: localhost:8080" |

  Scenario: A call without an ID is refused too
    Given a not-passed call with the built-in settings
    And the vendor name api.x.com resolves to 10.1.2.3
    When an app sends "GET https://api.x.com/a"
    Then the proxy answers 403
    And the record is minimal with hotline.integration.id not-passed and error.type destination_ip_prohibited

  Scenario: A call without an ID still reaches a public host
    Given a not-passed call with the built-in settings
    When an app sends "GET https://api.x.com/a"
    Then the proxy calls the vendor over https on port 443

  Scenario: An api that allows internal IPs
    Given an api for api.talon.one with allowInternalIPs set to true
    And the vendor name api.talon.one resolves to 10.1.2.3
    When an app sends "GET https://api.talon.one/v2/x"
    Then the vendor gets the call

  Scenario: The allow setting covers only that api's hosts
    Given an api for api.talon.one with allowInternalIPs set to true
    And the vendor name api.stripe.com resolves to 10.1.2.3
    When an app sends "GET https://api.stripe.com/v1/payment_intents"
    Then the proxy answers 403

  Scenario: A not-passed entry can allow internal IPs
    Given a not-passed entry with allowInternalIPs set to true
    And the vendor name api.x.com resolves to 10.1.2.3
    When an app sends "GET https://api.x.com/a" with no Proxy-Authorization
    Then the vendor gets the call

  Scenario: A name with public and internal IPs uses only the public ones
    Given the vendor name api.talon.one resolves to 10.1.2.3 and to its public IP
    When an app sends "GET https://api.talon.one/v2/x"
    Then the vendor gets the call on its public IP
    And no connection is opened to 10.1.2.3

  Scenario: A call to the proxy itself is refused
    When an app sends a call whose address is the proxy's own address
    Then the proxy answers 403
    And the proxy opens no connection

  Scenario: The config refuses a bad allow setting
    Given a config file where an api sets allowInternalIPs to "maybe"
    When the proxy loads it
    Then the config is refused with a message naming the problem
