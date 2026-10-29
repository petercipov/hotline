@later
Feature: TLS from apps to the proxy
  With listen.tls set, the app opens TLS to the proxy and sends the same
  request inside it, with the full vendor address in the request line. The
  proxy shows its own certificate, read from the cert and key files; it
  never makes one of its own. The proxy port then accepts only TLS, versions
  1.2 and 1.3, over HTTP/1.1. Client certificates (mutual TLS) come later.
  These scenarios wait for this slice to be built (Peter, 2026-10-28).

  Background:
    Given a test issuer, and a proxy certificate and key from it
    And a config file whose listen.tls points to those files

  Scenario: A call over TLS reaches the vendor
    Given an app that trusts the test issuer
    When the app opens TLS to the proxy and sends PUT https://api.talon.one/v2/x with ID talon-shop-a
    Then the vendor gets the call over https
    And the app gets the vendor's status, headers and body unchanged
    And the record is the same as for the same call without TLS to the proxy

  Scenario: The proxy shows the same certificate whatever name the app asks for
    Given an app that trusts the test issuer
    When the app opens TLS to the proxy asking for the name api.talon.one
    Then the proxy shows its own certificate

  Scenario: The proxy port accepts only TLS
    When an app sends a plain HTTP request to the proxy port
    Then the app gets 400
    And nothing reaches the vendor
    And no record is sent

  Scenario Outline: A connection that fails to set up TLS is not a call
    When <app> opens TLS to the proxy
    Then the TLS setup fails
    And nothing reaches the vendor
    And no record is sent
    And the proxy logs the failure at debug level

    Examples:
      | app                                     |
      | an app that does not trust the issuer   |
      | an app that allows only TLS 1.1 or less |

  Scenario Outline: TLS versions
    Given an app that trusts the test issuer and allows only <version>
    When the app sends a call to the proxy
    Then the call reaches the vendor

    Examples:
      | version |
      | TLS 1.2 |
      | TLS 1.3 |

  Scenario: An app that asks for HTTP/2 gets HTTP/1.1
    Given an app that trusts the test issuer and offers HTTP/2 and HTTP/1.1
    When the app sends a call to the proxy
    Then the call uses HTTP/1.1 and reaches the vendor

  Scenario Outline: A bad certificate stops the proxy at start
    Given <problem>
    When the proxy starts
    Then it refuses to start with exit code 1, saying why
    And hotline-proxy validate reports the same problem

    Examples:
      | problem                                         |
      | the cert file does not exist                    |
      | the key file cannot be read                     |
      | the key does not belong to the certificate      |
      | the certificate has already expired             |
      | listen.tls has a cert file but no key file      |

  Scenario: A renewed certificate is used without a restart
    Given the proxy is running with the first certificate
    And an app has a connection open to the proxy
    When the cert and key files are replaced with a second certificate
    Then a new connection gets the second certificate
    And the open connection keeps working

  Scenario: A broken renewal keeps the old certificate
    Given the proxy is running with the first certificate
    When the key file is replaced with a key that does not belong to the certificate
    Then new connections still get the first certificate
    And the proxy logs why it kept it
