Feature: Hotline should be able to
  1) list schema
  2) upload schema
  3) list schema
  4) get schema content

  Scenario: list of schemas is empty for not configured hotline
    Given hotline is running
    Then schema list is:
      """
      { "schemas": [] }
      """

  Scenario: schema can be listed, created and deleted
    Given hotline is running
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