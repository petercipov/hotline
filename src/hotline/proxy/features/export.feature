Feature: Sending OpenTelemetry data
  Records and counts go out over OTLP. Standard names come from the
  semconv Go package; Hotline-only fields use the hotline. prefix.
  Tests use a fake OTLP receiver running locally.

  Background:
    Given a fake OTLP receiver

  Scenario: Resource on every signal
    When the proxy sends any signal
    Then its resource has service.name hotline-proxy, service.version and service.instance.id

  Scenario: One log record per call
    When a call ends
    Then the receiver gets one OTLP log record with all of the record's attributes
    And standard names come from the semconv constants, such as http.request.method and url.full
    And every other field has the exact hotline. name from the spec
    And numbers are numbers, true and false are booleans and header values are lists of strings

  Scenario: The hotline.requests counter
    When a call ends
    Then hotline.requests goes up by one, with unit {request}
    And it carries hotline.integration.id, url.template, http.request.method, http.response.status_code and error.type

  Scenario: The counter for not-passed calls
    When a not-passed call ends
    Then hotline.requests carries no url.template

  Scenario Outline: Temporality
    Given an exporter that <sets>
    When calls end
    Then the totals are <kind>

    Examples:
      | sets                       | kind       |
      | does not set temporality   | cumulative |
      | sets temporality to delta  | delta      |

  Scenario: No latency histogram
    When calls end
    Then no histogram is sent

  Scenario: Several exporters get the same data
    Given two exporters
    When a call ends
    Then both get the same record and count

  Scenario: One exporter down
    Given two exporters, one of them down
    When calls end
    Then the other keeps receiving

  Scenario: Default queue size
    Given an exporter with no queue size set
    When the proxy starts
    Then its queue holds 10 000 records

  Scenario: A full queue drops the oldest
    Until the proxy's own health data exists, drops are counted in a
    warning log line, at most once a minute per destination (Peter,
    2026-10-28).
    Given an exporter with queueSize 3 whose destination is busy with the first record
    When 4 more calls end
    Then the destination gets records 1, 3, 4 and 5
    And the proxy logs a warning that 1 record was dropped for that destination
    And no call was blocked or slowed

  Scenario: Flushing on shutdown
    Given records waiting in the queue
    When the proxy shuts down
    Then they are sent first, within a time limit

  Scenario: Left for later slices
    When the proxy runs
    Then it sends no trace spans
    And it sends no proxy health data to the telemetry exporter
