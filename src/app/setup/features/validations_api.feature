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

  Scenario: attach existing schema id
    Given hotline is running:
      | Feature           | Enabled |
    And advance time by 1s
    And schema is created from file "./features/fixtures/product_schema.json"
    And advance time by 1s
    And schema is created from file "./features/fixtures/headers_schema.json"
    And advance time by 1s
    And schema is created from file "./features/fixtures/query_schema.json"
    And schema list is:
      """
      { "schemas": [
        {
          "schemaID" : "SCAZUtiVm5cQGBAQEBAQEBAQ",
          "title" : "./features/fixtures/product_schema.json",
          "updatedAt" : "2025-02-22T12:02:11.001Z"
        }, {
          "schemaID" : "SCAZUtiV2icQGBAQEBAQEBAQ",
          "title" : "./features/fixtures/headers_schema.json",
          "updatedAt" : "2025-02-22T12:02:12.002Z"
        }, {
          "schemaID" : "SCAZUtiWGLcQGBAQEBAQEBAQ",
          "title" : "./features/fixtures/query_schema.json",
          "updatedAt" : "2025-02-22T12:02:13.003Z"
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
  Scenario: delete existing validation
    Given hotline is running:
      | Feature           | Enabled |
    And advance time by 1s
    And schema is created from file "./features/fixtures/product_schema.json"
    And schema list is:
      """
      { "schemas": [
        {
          "schemaID" : "SCAZUtiVm5cQGBAQEBAQEBAQ",
          "title" : "./features/fixtures/product_schema.json",
          "updatedAt" : "2025-02-22T12:02:11.001Z"
        }
      ] }
      """
    And validation for integration "INdd0391f11aba" is created:
      """
      {
        "route": { "method": "GET", "host": "127.0.0.1", "path": "/products" },
        "requestSchema": {
          "bodySchemaID": "SCAZUtiVXQcQGBAQEBAQEBAQ"
        }
      }
      """
    And validations for integration "INdd0391f11aba" list is:
      """
      {
        "route-validations" : [{
          "requestSchema" : {
            "bodySchemaID" : "SCAZUtiVXQcQGBAQEBAQEBAQ"
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
    When validation for integration "INdd0391f11aba" with routeKey "RKbhmdHaevZLs" is deleted
    Then validations for integration "INdd0391f11aba" list is:
      """
      { "route-validations": [] }
      """