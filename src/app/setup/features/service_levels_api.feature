Feature: Service Levels API
  1) CRUD service levels

  Scenario: no definitions
    Given hotline is running:
      | Feature           | Enabled |
    Then service levels for "IN-dd0391f11aba" are:
      """
      """

  Scenario: upsert definition
    Given hotline is running:
      | Feature           | Enabled |
    And service levels for "IN-dd0391f11aba" is set to:
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
    Then service levels for "IN-dd0391f11aba" are:
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
    Given hotline is running:
      | Feature           | Enabled |
    And service levels for "IN-dd0391f11aba" is set to:
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
    When service levels for "IN-dd0391f11aba" and routeKey "GET:127.0.0.1::/bookings" are deleted
    Then service levels for "IN-dd0391f11aba" are:
      """
      """