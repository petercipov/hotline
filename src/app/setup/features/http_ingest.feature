Feature: Hotline should be able to
  1) ingest otel http traces,
  2) compute slo
  3 report slos to otel

  Scenario: Integration traffic is inspected and slos are computed
    And OTEL ingestion is enabled
    And slo reporter is pointing to collector
    And hotline is running

    When traffic is sent to OTEL ingestion for integration ID "IN-dd0391f11aba"
    And advance time by 10s

    Then slo metrics are received in collector:
      | Timestamp UTC        | Metric Name                                           | Metric Type | Metric Value | Metric Unit | Metric Attributes                                                          |
      | 2025-02-22T12:02:20Z | service_levels_http_route_latency_p99                 | Gauge       | 10140.455    | ms          | integration_id:IN-dd0391f11aba; breached:true; http_route:/                |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_expected             | Gauge       | 38.66        | %           | integration_id:IN-dd0391f11aba; breached:true; http_route:/                |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_expected_breakdown   | Gauge       | 19.32        | %           | integration_id:IN-dd0391f11aba; breakdown:200; breached:true; http_route:/ |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_expected_breakdown   | Gauge       | 19.34        | %           | integration_id:IN-dd0391f11aba; breakdown:201; breached:true; http_route:/ |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_unexpected           | Gauge       | 61.34        | %           | integration_id:IN-dd0391f11aba; breached:true; http_route:/                |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_unexpected_breakdown | Gauge       | 20.46        | %           | integration_id:IN-dd0391f11aba; breakdown:4xx; breached:true; http_route:/ |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_unexpected_breakdown | Gauge       | 40.88        | %           | integration_id:IN-dd0391f11aba; breakdown:5xx; breached:true; http_route:/ |