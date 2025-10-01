Feature: Schema API
  1) list schema
  2) upload schema
  3) list schema
  4) get schema content

  Scenario: list of schemas is empty for not configured hotline
    Given hotline is running:
      | Feature           | Enabled |
    Then schema list is:
      """
      { "schemas": [] }
      """

  Scenario: schema can be listed, created and deleted
    Given hotline is running:
      | Feature           | Enabled |
    When schema is created from file "./features/fixtures/product_schema.json"
    Then schema list is:
      """
      { "schemas": [
        {
          "schemaID": "SCAZUtiVXQcQGBAQEBAQEBAQ",
          "updatedAt": "2025-02-22T12:02:10.0005Z"
        }
      ] }
      """
    Then schema content for "SCAZUtiVXQcQGBAQEBAQEBAQ" is same as in file "./features/fixtures/product_schema.json"
    When schema "SCAZUtiVXQcQGBAQEBAQEBAQ" is deleted
    Then schema list is:
      """
      { "schemas": [] }
      """
  Scenario: schema can be upserted
    Given hotline is running:
      | Feature           | Enabled |
    And schema is created from file "./features/fixtures/product_schema.json"
    And schema list is:
      """
      { "schemas": [
        {
          "schemaID": "SCAZUtiVXQcQGBAQEBAQEBAQ",
          "updatedAt": "2025-02-22T12:02:10.0005Z"
        }
      ] }
      """

    When advance time by 10s
    And schema "SCAZUtiVXQcQGBAQEBAQEBAQ" is upserted from file "./features/fixtures/product_schema.v2.json"

    Then schema list is:
      """
      { "schemas": [
        {
          "schemaID": "SCAZUtiVXQcQGBAQEBAQEBAQ",
          "updatedAt": "2025-02-22T12:02:20.00150Z"
        }
      ] }
      """