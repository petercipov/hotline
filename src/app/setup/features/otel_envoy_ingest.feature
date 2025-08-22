Feature: Hotline should be able to
  1) ingest envoy http traces,
  2) compute slo
  3) report slos to otel

  Scenario: otel envoy http traffic is ingested and slos are computed
    Given OTEL ingestion is enabled
    And slo reporter is pointing to collector
    And hotline is running
    And slo configuration for "IN-dd0391f11aba" is set to:
      """
        {
          "route": { "path": "/" },
          "latency": {
            "percentiles": [{ "percentile": "99.9%", "breachLatency": "2s" }],
            "windowDuration": "1m0s"
          },
          "status": { "expected": [ "200", "201" ], "breachThreshold": "99.9%", "windowDuration": "1h" }
        }
      """

    When envoy otel traffic is sent for ingestion for integration ID "IN-dd0391f11aba"
    And advance time by 10s

    Then slo metrics are received in collector:
      | Timestamp UTC        | Name                                              | Type  | Value     | Unit | Attributes                                                                                     |
      | 2025-02-22T12:02:20Z | service_levels_http_route_latency                 | Gauge | 10140.455 | ms   | metric:p99.9; integration_id:IN-dd0391f11aba; breached:true; http_route::::/                        |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status                  | Gauge | 38.66     | %    | metric:expected; integration_id:IN-dd0391f11aba; breached:true; http_route::::/                   |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status                  | Gauge | 61.34     | %    | metric:unexpected; integration_id:IN-dd0391f11aba; breached:true; http_route::::/                 |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 40.88     | %    | metric:unexpected; breakdown:5xx; integration_id:IN-dd0391f11aba; breached:true; http_route::::/  |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 19.32     | %    | metric:expected; breakdown:200; integration_id:IN-dd0391f11aba; breached:true; http_route::::/    |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 19.34     | %    | metric:expected; breakdown:201; integration_id:IN-dd0391f11aba; breached:true; http_route::::/    |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 20.46     | %    | metric:unexpected; breakdown:4xx; integration_id:IN-dd0391f11aba; breached:true; http_route::::/  |

      | 2025-02-22T12:02:20Z | service_levels_http_route_latency_events          | Sum   | 10000     | #    | metric:p99.9; breached:true; http_route::::/; integration_id:IN-dd0391f11aba;                       |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 1932      | #    | metric:expected; breakdown:200; breached:true; http_route::::/; integration_id:IN-dd0391f11aba;   |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 1934      | #    | metric:expected; breakdown:201; breached:true; http_route::::/; integration_id:IN-dd0391f11aba;   |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 4088      | #    | metric:unexpected; breakdown:5xx; breached:true; http_route::::/; integration_id:IN-dd0391f11aba; |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 2046      | #    | metric:unexpected; breakdown:4xx; breached:true; http_route::::/; integration_id:IN-dd0391f11aba; |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_events           | Sum   | 3866      | #    | metric:expected; breached:true; http_route::::/; integration_id:IN-dd0391f11aba;                  |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_events           | Sum   | 6134      | #    | metric:unexpected; breached:true; http_route::::/; integration_id:IN-dd0391f11aba;                |
