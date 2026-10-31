Feature: The proxy's own telemetry
  The proxy sends numbers about itself as OpenTelemetry metrics to the
  telemetry destination only, never mixed with the records of vendor calls:
  calls in flight, how long calls take and how much the proxy adds, its
  export queues, config reloads, certificates and the Go runtime. Standard
  names where OpenTelemetry has one, hotline. for our own. No number is
  split by integration, vendor, host, URL or route, so a proxy with a
  million integrations sends as few numbers as one with one (Peter,
  2026-10-31). Every metric can be turned off by name.

  Background:
    Given a fake OTLP receiver for the proxy's own telemetry

  Scenario: Own data goes only to the telemetry destination
    When a call ends
    Then the telemetry receiver gets the metric http.server.request.duration
    And the telemetry receiver gets no log records
    And the product receiver gets no metric but hotline.requests

  Scenario: Calls in flight, as the standard counts them
    Given a vendor that keeps the call waiting
    When an app starts a forward proxy GET call over http
    Then http.server.active_requests is 1 for http.request.method GET, url.scheme http and hotline.ingress forward_proxy
    And once the call ends it is 0

  Scenario: The time each call takes, as the standard measures it
    When an app makes a forward proxy GET call over http and the vendor answers 200
    Then the telemetry receiver gets the histogram http.server.request.duration with unit s
    And its range limits are 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5 and 10
    And it counts 1 call for http.request.method GET, url.scheme http, http.response.status_code 200 and hotline.ingress forward_proxy
    And it has no label but http.request.method, url.scheme, http.response.status_code, error.type and hotline.ingress

  Scenario: A call the proxy fails names its error type
    Given the vendor's name does not resolve
    When an app makes a forward proxy GET call over http
    Then http.server.request.duration counts 1 call with error.type dns_error

  Scenario: An unusual method counts as _OTHER
    When an app makes a forward proxy call with the method PURGE
    Then http.server.request.duration counts 1 call for http.request.method _OTHER

  Scenario: The proxy's own time on each call
    When an app makes a forward proxy GET call over http and the vendor answers 200
    Then the telemetry receiver gets the histogram hotline.proxy.overhead with unit s
    And its range limits are 0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.005, 0.025 and 0.1
    And it counts 1 call for hotline.ingress forward_proxy
    And it has no label but hotline.ingress

  Scenario: An ingress nobody used sends nothing
    When an app makes a forward proxy GET call over http and the vendor answers 200
    Then hotline.proxy.overhead has no numbers for hotline.ingress host_header

  Scenario: The queue of each destination
    Given a product destination with queueSize 7
    When a call ends
    Then otel.sdk.processor.log.queue.capacity is 7 for that destination
    And otel.sdk.processor.log.queue.size is sent for that destination

  Scenario: The span queue of each destination
    Spans have their own queue, of the same size, next to the records'.
    Given a product destination with queueSize 7
    When a call ends
    Then otel.sdk.processor.span.queue.capacity is 7 for that destination
    And otel.sdk.processor.span.queue.size is sent for that destination

  Scenario: Spans sent fine are counted
    Given a vendor reached over a new https connection
    When a call ends
    Then otel.sdk.exporter.span.exported counts 12 spans with no error.type for that destination

  Scenario: Records dropped from a full queue are counted
    Given an exporter with queueSize 3 whose destination is busy with the first record
    When 4 more calls end
    Then otel.sdk.processor.log.processed counts 1 record with error.type queue_full for that destination

  Scenario: Records sent fine are counted
    When 3 calls end
    Then otel.sdk.exporter.log.exported counts 3 records with no error.type for that destination

  Scenario: Records lost in a refused send are counted
    Given a product destination over HTTP that refuses every log export with 400
    When a call ends
    Then otel.sdk.exporter.log.exported counts 1 record with an error.type for that destination

  Scenario: Counts sent to a destination are counted too
    When a call ends
    Then otel.sdk.exporter.metric_data_point.exported counts data points with no error.type for that destination

  Scenario: A destination that works shows as up
    When a call ends
    Then hotline.destination.state is 1 for that destination

  Scenario: A destination that is down shows as down, and up again once it works
    Given a product destination that is down
    When a call ends
    Then hotline.destination.state turns 0 for that destination
    When the destination comes back and another call ends
    Then hotline.destination.state turns 1 for that destination

  Scenario: The config in use
    When the proxy starts
    Then hotline.config.info is 1 with the config's hotline.config.version
    And it says hotline.listen.tls false and hotline.listen.connect false
    And hotline.config.reload.last_success is 1
    And hotline.config.restart_needed is 0

  Scenario: A failed reload is counted and stays visible
    Given the proxy runs
    When the config file changes so that it is not valid YAML
    And the proxy checks its files
    Then hotline.config.reloads counts 1 with hotline.reload.result failure
    And hotline.config.reload.last_success turns 0
    And hotline.config.info still names the first config version

  Scenario: A good reload after a failed one clears the failure
    Given the proxy runs
    And the config file changes so that it is not valid YAML
    And the proxy checks its files
    When the config file changes so that talon-shop-b also uses Talon.One
    And the proxy checks its files
    Then hotline.config.reloads counts 1 with hotline.reload.result success
    And hotline.config.reload.last_success turns 1
    And hotline.config.info names the new config version

  Scenario: A change that needs a restart shows until the restart
    Given the proxy runs
    When the config file changes so that listen.proxy moves to another port
    And the proxy checks its files
    Then hotline.config.restart_needed turns 1

  Scenario: A change put back no longer needs a restart
    Given the proxy runs
    And the config file changes so that listen.proxy moves to another port
    And the proxy checks its files
    When the config file is put back as it was
    And the proxy checks its files
    Then hotline.config.restart_needed turns 0

  Scenario: The proxy's own certificate
    The expiry is a time in seconds since 1970, so an alert reads "expires
    within 7 days" and the number does not change every day.
    Given the proxy port takes TLS with a certificate that expires in 90 days
    When the proxy starts
    Then hotline.certificate.not_after is that certificate's expiry for hotline.certificate.use listen.tls
    And hotline.config.info says hotline.listen.tls true

  Scenario: A renewed certificate shows its new date
    Given the proxy port takes TLS with a certificate that expires in 90 days
    And the proxy runs
    When the cert and key files are replaced with a certificate that expires in 120 days
    And the proxy checks its files
    Then hotline.certificate.not_after turns to the new certificate's expiry for hotline.certificate.use listen.tls

  Scenario: The CONNECT issuer
    Given CONNECT is an ingress with an issuer that expires in 200 days
    When the proxy starts
    Then hotline.certificate.not_after is that issuer's expiry for hotline.certificate.use listen.connect

  Scenario Outline: Failed TLS setups with apps are counted
    Given the proxy port takes TLS
    When an app <does>
    Then hotline.app_tls.failures counts 1 with error.type <type> and hotline.ingress <ingress>

    Examples:
      | does                                             | type                 | ingress   |
      | that does not trust the proxy opens TLS          | tls_untrusted_by_app | none      |
      | sends plain HTTP to the TLS port                 | plain_http           | none      |
      | that allows only TLS 1.1 opens TLS               | tls_version          | none      |
      | that does not trust the issuer opens a tunnel    | tls_untrusted_by_app | connect   |

  Scenario: The Go runtime
    When the proxy starts
    Then the telemetry receiver gets the metrics go.memory.used and go.goroutine.count

  Scenario: One metric turned off
    Given the telemetry section turns off http.server.request.duration
    When a call ends
    Then the telemetry receiver gets the histogram hotline.proxy.overhead
    And the telemetry receiver gets no metric http.server.request.duration

  Scenario: A group of metrics turned off with a star
    Given the telemetry section turns off go.*
    When the proxy starts
    Then the telemetry receiver gets no metric starting with go.
    And the telemetry receiver gets the metric hotline.destination.state

  Scenario: A name that matches none of the proxy's metrics is a warning
    Given the telemetry section turns off hotline.destination.stat
    When the proxy starts
    Then the proxy logs a warning naming hotline.destination.stat
    And the proxy runs

  Scenario: Without a telemetry section nothing of the proxy's own is sent
    Given a config with no telemetry section
    When a call ends
    Then the proxy logs a warning that its own telemetry is not sent
    And the product receiver gets no metric but hotline.requests

  Scenario: A change to the telemetry section needs a restart
    Given the proxy runs
    When the config file changes so that the telemetry section turns off go.*
    And the proxy checks its files
    Then the proxy logs a warning that telemetry needs a restart
