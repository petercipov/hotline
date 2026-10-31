# Requirements and decisions: docs/proxy-xml-bodies.md.
Feature: Routes picked by a query word, and values that take the rest of the path
  Some vendors, S3 first, tell their actions apart by a word in the query:
  GET /A-1001.pdf fetches a file, GET /A-1001.pdf?tagging fetches its tags.
  OpenAPI allows no ? in a path, so the spec writes the word after #, where
  SQS's action header goes: /{Key+}#?tagging. A word may need a value
  (#?list-type=2), and a route may need several (#?select&select-type=2)
  (Peter, 2026-10-31).

  S3 file names hold slashes, such as 2026/10/A-1001.pdf, so a value written
  {Key+} takes the rest of the path, as in AWS's own model. Only the last
  part of a path may take the rest (Peter, 2026-10-31).

  Background:
    Given the S3-like routes spec

  Rule: A query word picks the route

    Scenario Outline: Each action is its own route
      When a call is <call>
      Then the route template is <template>
      And the record has no request results

      Examples:
        | call                                      | template                       |
        | GET /                                     | /                              |
        | GET /?list-type=2&prefix=2026             | /#?list-type=2                 |
        | GET /?list-type=1                         | /                              |
        | POST /?delete                             | /#?delete                      |
        | POST /?delete=                            | /#?delete                      |
        | GET /A-1001.pdf                           | /{Key+}                        |
        | GET /A-1001.pdf?tagging                   | /{Key+}#?tagging               |
        | PUT /A-1001.pdf?partNumber=1&uploadId=u-1 | /{Key+}#?uploadId              |
        | POST /a.csv?select&select-type=2          | /{Key+}#?select&select-type=2  |

    Scenario Outline: A call without its word has no route
      When a call is <call>
      Then there is no route

      Examples:
        | call                  |
        | POST /                |
        | POST /?Delete         |
        | POST /a.csv?select    |

    Scenario: A word the plain route doesn't know is reported there
      When a call is GET /A-1001.pdf?acl
      Then the route template is /{Key+}
      And the request results are query acl not_in_spec

    Scenario: A word's fixed value stays in url.full, other values are redacted
      When a call is PUT /A-1001.pdf?partNumber=1&uploadId=u-1&list-type=2
      Then url.full is "https://api.x.com/REDACTED?partNumber=REDACTED&uploadId=REDACTED&list-type=REDACTED"
      When a call is GET /?list-type=2&prefix=2026
      Then url.full is "https://api.x.com/?list-type=2&prefix=REDACTED"

  Rule: A value written {Key+} takes the rest of the path

    Scenario Outline: The rest of the path is one value
      When a call is <call>
      Then the route template is <template>

      Examples:
        | call                              | template          |
        | GET /2026/10/A-1001.pdf           | /{Key+}           |
        | GET /2026/10/A-1001.pdf?tagging   | /{Key+}#?tagging  |
        | GET /reports/r.csv                | /reports/{Key+}   |
        | GET /reports/2026/r.csv           | /reports/{Key+}   |

    Scenario: The rest of the path must hold something
      When a call is GET /reports/
      Then the route template is /{Key+}

    Scenario: The whole rest is checked as one value
      When a call is GET /reports/2026/10/r.csv
      Then the record has no request results

    Scenario: A rest that breaks its pattern is reported
      When a call is GET /reports/r.csv
      Then the request results are path Key pattern ^[0-9]{4}/

    Scenario: The whole rest is redacted as one value
      When a call is GET /2026/10/A-1001.pdf
      Then url.full is "https://api.x.com/REDACTED"

  Rule: A spec that writes these wrong is refused

    Scenario Outline: A broken route is refused
      Given a spec with the route <route>
      When the proxy loads the config
      Then the config is refused with a message naming "<route>"

      Examples:
        | route              |
        | /{Key+}/meta       |
        | /#?                |
        | /files#?a&&b       |
