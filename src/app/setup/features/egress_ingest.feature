Feature: Hotline should be able to
  1) ingest and proxy egress traffic,
  2) compute slo
  3) report slos to otel

  Scenario: egress traffic is ingested, proxied and slos are computed
    Given service levels reporter is pointing to collector
    And hotline is running:
      | Feature           | Enabled |
      | egress ingestion  | true    |
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
        |||
        {
          "route": { "method": "POST", "host": "127.0.0.1", "path": "/bookings" },
          "latency": {
            "percentiles": [{ "percentile": "99.9%", "breachLatency": "2s"}],
            "windowDuration": "1m0s"
          },
          "status": { "expected": [ "201" ], "breachThreshold": "99.9%", "windowDuration": "1h0m0s" }
        }
        |||
        {
          "route": { "method": "DELETE", "host": "127.0.0.1", "path": "/bookings/{bookingId}" },
          "latency": {
            "percentiles": [{ "percentile": "99.9%", "breachLatency": "2s" }],
            "windowDuration": "1m0s"
          },
          "status": { "expected": [ "204" ], "breachThreshold": "99.9%", "windowDuration": "1h0m0s" }
        }
      """

    When Egress traffic is sent for proxying for integration ID "IN-dd0391f11aba"
    And advance time by 10s

    Then service levels metrics are received in collector:
      | Timestamp UTC        | Name                                              | Type  | Value     | Unit | Attributes                                                                                         |
      | 2025-02-22T12:02:20Z | service_levels_http_route_latency                 | Gauge | 5797.838  | ms   | breached:true; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:p99.9               |
      | 2025-02-22T12:02:20Z | service_levels_http_route_latency                 | Gauge | 5797.838  | ms   | breached:true; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:p99.9                |
      | 2025-02-22T12:02:20Z | service_levels_http_route_latency                 | Gauge | 5797.838  | ms   | breached:true; http_route:RKYKx-JlBkuxQ; integration_id:IN-dd0391f11aba; metric:p99.9 |

      | 2025-02-22T12:02:20Z | service_levels_http_route_status                  | Gauge | 50.737    | %    | breached:true; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:unexpected        |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status                  | Gauge | 100.00    | %    | breached:false; http_route:RKYKx-JlBkuxQ; integration_id:IN-dd0391f11aba; metric:expected |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status                  | Gauge | 50.303    | %    | breached:true; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:unexpected         |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status                  | Gauge | 49.697    | %    | breached:true; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:expected                 |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status                  | Gauge | 49.263    | %    | breached:true; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:expected          |

      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 50.303    | %    | breached:true; breakdown:5xx; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:unexpected               |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 100.00    | %    | breached:false; breakdown:204; http_route:RKYKx-JlBkuxQ; integration_id:IN-dd0391f11aba; metric:expected |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 49.263    | %    | breached:true; breakdown:201; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:expected                |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 49.697    | %    | breached:true; breakdown:200; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:expected                 |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown        | Gauge | 50.737    | %    | breached:true; breakdown:5xx; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:unexpected              |

      | 2025-02-22T12:02:20Z | service_levels_http_route_latency_events          | Sum   | 330       | #    | breached:true; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:p99.9                                     |
      | 2025-02-22T12:02:20Z | service_levels_http_route_latency_events          | Sum   | 331       | #    | breached:true; http_route:RKYKx-JlBkuxQ; integration_id:IN-dd0391f11aba; metric:p99.9                      |
      | 2025-02-22T12:02:20Z | service_levels_http_route_latency_events          | Sum   | 339       | #    | breached:true; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:p99.9                                    |

      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 164       | #    | breached:true; breakdown:200; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:expected                 |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 172       | #    | breached:true; breakdown:5xx; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:unexpected              |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 166       | #    | breached:true; breakdown:5xx; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:unexpected               |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 331       | #    | breached:false; breakdown:204; http_route:RKYKx-JlBkuxQ; integration_id:IN-dd0391f11aba; metric:expected |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_breakdown_events | Sum   | 167       | #    | breached:true; breakdown:201; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:expected                |

      | 2025-02-22T12:02:20Z | service_levels_http_route_status_events           | Sum   | 167       | #    | breached:true; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:expected                               |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_events           | Sum   | 164       | #    | breached:true; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:expected                                |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_events           | Sum   | 166       | #    | breached:true; http_route:RK7k6fbRtiQp4; integration_id:IN-dd0391f11aba; metric:unexpected                              |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_events           | Sum   | 172       | #    | breached:true; http_route:RKT9CfWr9oXRo; integration_id:IN-dd0391f11aba; metric:unexpected                             |
      | 2025-02-22T12:02:20Z | service_levels_http_route_status_events           | Sum   | 331       | #    | breached:false; http_route:RKYKx-JlBkuxQ; integration_id:IN-dd0391f11aba; metric:expected                |

