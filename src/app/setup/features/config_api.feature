Feature: Config API
  1) list slo definitions
  2) update slo definitions
  3) delete slo defnitions

  Scenario: no definitions
    Given hotline is running
    Then slo configuration for "IN-dd0391f11aba" is:
      """
      """

  Scenario: upsert definition
    Given hotline is running
    And slo configuration for "IN-dd0391f11aba" is set to:
      """
        {
          "route": { "method": "GET", "host": "127.0.0.1", "path": "/bookings" },
          "latency": {
            "percentiles": [{ "percentile": "99.9%", "breachLatency": "2s" }],
            "windowDuration": "1m0s"
          },
          "status": { "expected": [ "200" ], "breachThreshold": "99.9%", "windowDuration": "1h0m0s" }
        }
      """
    Then slo configuration for "IN-dd0391f11aba" is:
      """
        {
          "route": { "method": "GET", "host": "127.0.0.1", "path": "/bookings" },
          "routeKey": "GET:127.0.0.1::/bookings",
          "latency": {
            "percentiles": [{ "percentile": "99.9%", "breachLatency": "2s" }],
            "windowDuration": "1m0s"
          },
          "status": { "expected": [ "200" ], "breachThreshold": "99.9%", "windowDuration": "1h0m0s" }
        }
      """
  Scenario: delete definition
    Given hotline is running
    And slo configuration for "IN-dd0391f11aba" is set to:
      """
        {
          "route": { "method": "GET", "host": "127.0.0.1", "path": "/bookings" },
          "latency": {
            "percentiles": [{ "percentile": "99.9%", "breachLatency": "2s" }],
            "windowDuration": "1m0s"
          },
          "status": { "expected": [ "200" ], "breachThreshold": "99.9%", "windowDuration": "1h0m0s" }
        }
      """
    When slo configuration for "IN-dd0391f11aba" and routeKey "GET:127.0.0.1::/bookings" is deleted
    Then slo configuration for "IN-dd0391f11aba" is:
      """
      """