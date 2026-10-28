# Answers to the open questions are in docs/proxy-slice-1.md.
Feature: Loading the config file
  The proxy reads one YAML file, hotline-proxy.yaml, once at start.
  Reloading without a restart comes in a later slice.

  Scenario: A valid config is read in full
    Given a valid config file
    When the proxy loads it
    Then the proxy listen address is read
    And every OTLP exporter is read with its endpoint
    And every integration is read with its apis, their hosts and the spec each api points to
    And spec files are found relative to the folder of the config file

  Scenario: The config's version goes into every record
    The version is a field in the config file, written by whoever changes
    it (Peter, 2026-10-28).
    Given a valid config file with version 2026-10-28.1
    When the proxy loads it
    Then every record carries hotline.config.version 2026-10-28.1

  Scenario: The full example loads
    Given the full example config from proxy-example
    When the proxy loads it
    Then it loads together with both of its specs

  Scenario Outline: Exporter temporality
    Given an exporter that <sets>
    When the proxy loads the config
    Then the exporter's temporality is <temporality>

    Examples:
      | sets                        | temporality |
      | does not set temporality    | cumulative  |
      | sets temporality to delta   | delta       |

  Scenario Outline: Api defaults
    Given an api that <sets>
    When the proxy loads the config
    Then <result>

    Examples:
      | sets                            | result                                       |
      | does not set tlsOrigination     | TLS origination is on                        |
      | sets tlsOrigination to false    | TLS origination is off                       |
      | does not set vendorTimeout      | the proxy waits 60 seconds for the vendor    |
      | sets vendorTimeout to 10s       | the proxy waits 10 seconds for the vendor    |

  Scenario: One api with several hosts
    Given an api that lists the hosts api.talon.one and eu.api.talon.one
    When the proxy loads the config
    Then calls to either host use that api

  Scenario: Hosts match in any case
    Given an api that lists the host API.Talon.One
    When a call goes to api.talon.one
    Then that api is used

  Scenario: Built-in not-passed settings
    Given a config file with no not-passed entry
    When the proxy loads it
    Then not-passed calls use no TLS origination and may go to any host

  Scenario: A not-passed entry overrides the built-in settings
    Given a config file with a not-passed entry that sets tlsOrigination to true
    When the proxy loads it
    Then not-passed calls use TLS origination

  Scenario: The same host in two integrations
    Given two integrations that both list api.talon.one, each with its own spec
    When the proxy loads the config
    Then both are accepted

  Scenario: A config with no integrations
    Given a config file with exporters but no integrations
    When the proxy loads it
    Then it is accepted and every call uses the not-passed settings

  Scenario Outline: Broken configs are refused
    Given a config file where <problem>
    When the proxy loads it
    Then the config is refused with a message naming the problem

    Examples:
      | problem                                              |
      | the file does not exist                              |
      | the file is not valid YAML                           |
      | there is no version                                  |
      | there is no proxy listen address                     |
      | an integration has no id                             |
      | there are no exporters                               |
      | an exporter's temporality is neither cumulative nor delta |
      | an exporter's endpoint is not a valid address        |
      | an exporter's queueSize is zero                      |
      | two integrations have the same ID                    |
      | one integration lists the same host in two apis      |
      | an api points to a spec file that does not exist     |
      | a spec is not a valid OpenAPI 3 document             |
      | a vendorTimeout is zero                              |
      | a vendorTimeout is negative                          |
      | a vendorTimeout is not a duration                    |
      | an x-hotline-redact mark is not true or false        |

  Scenario Outline: Settings the proxy does not use start with a warning
    The proxy starts anyway, so a typo never stops traffic, but each one is
    named in a warning log line (Peter, 2026-10-28).
    Given a config file where <problem>
    When the proxy loads it
    Then the proxy starts and logs a warning naming <name>

    Examples:
      | problem                                               | name          |
      | the not-passed entry has an apis list                 | apis          |
      | a field name has a typo, such as tlsOrigintion        | tlsOrigintion |

  Scenario: A busy listen address
    Given another program listens on the proxy address
    When the proxy loads the config
    Then the proxy does not start, and says the address is in use

  Scenario: Every problem is listed
    Given a config file with three problems
    When the proxy loads it
    Then all three problems are listed, not only the first one

  Scenario: No secret in error messages
    Given a broken config file with an integration that has a fingerprint key
    When the proxy reports the problem
    Then the message holds no secret value
