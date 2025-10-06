Feature: Validations API
  1) list validations
  2) update / delete validations

  Scenario: list of validations is empty for not configured hotline
    Given hotline is running:
      | Feature           | Enabled |
    And validations for integration "INdd0391f11aba" list is:
      """
      { "route-validations": [] }
      """

  Scenario: attach existing schema id:
    Given hotline is running:
      | Feature           | Enabled |
    And schema is created from file "./features/fixtures/product_schema.json"
    And schema is created from file "./features/fixtures/headers_schema.json"
    And schema is created from file "./features/fixtures/query_schema.json"
    And schema list is:
      """
      { "schemas": [
        {
          "schemaID": "SCAZUtiVXQcQGBAQEBAQEBAQ",
          "title": "./features/fixtures/product_schema.json",
          "updatedAt": "2025-02-22T12:02:10.0005Z"
        },
        {
          "schemaID": "SCAZUtiVXRcQGBAQEBAQEBAQ",
          "title": "./features/fixtures/headers_schema.json",
          "updatedAt": "2025-02-22T12:02:10.001Z"
        },
        {
          "schemaID" : "SCAZUtiVXRcQKBAQEBAQEBAQ",
          "title" : "./features/fixtures/query_schema.json",
          "updatedAt" : "2025-02-22T12:02:10.0015Z"
        }
      ] }
      """
    When validation for integration "INdd0391f11aba" is created:
      """
      {
        "route": { "method": "GET", "host": "127.0.0.1", "path": "/products" },
        "requestSchema": {
          "bodySchemaID": "SCAZUtiVXQcQGBAQEBAQEBAQ",
          "headerSchemaID": "SCAZUtiVXRcQGBAQEBAQEBAQ",
          "querySchemaID": "SCAZUtiVXRcQKBAQEBAQEBAQ"
        }
      }
      """
    Then validations for integration "INdd0391f11aba" list is:
      """
      {
        "route-validations" : [{
          "requestSchema" : {
            "bodySchemaID" : "SCAZUtiVXQcQGBAQEBAQEBAQ",
            "headerSchemaID": "SCAZUtiVXRcQGBAQEBAQEBAQ",
            "querySchemaID": "SCAZUtiVXRcQKBAQEBAQEBAQ"
          },
          "route" : {
            "host" : "127.0.0.1",
            "method" : "GET",
            "path" : "/products"
          },
          "routeKey" : "RKbhmdHaevZLs"
        }]
      }
      """
