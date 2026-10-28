Feature: Redacting the vendor address
  The x-hotline-redact marks in our copy of the vendor's spec decide what
  stays in url.full. A redacted value is replaced with the text REDACTED.

  Scenario Outline: The sample calls
    Given the <vendor> spec
    When an app calls "<address>"
    Then url.full is "<redacted>"

    Examples:
      | vendor    | address                                                          | redacted                                                          |
      | Talon.One | https://api.talon.one/v2/customer_sessions/sess-8812?dry=true    | https://api.talon.one/v2/customer_sessions/REDACTED?dry=true      |
      | Stripe    | https://api.stripe.com/v1/payment_intents?limit=10&customer=cus_77 | https://api.stripe.com/v1/payment_intents?limit=10&customer=REDACTED |
      | Stripe    | https://api.stripe.com/v1/payment_intents/pi_3Mtw                | https://api.stripe.com/v1/payment_intents/pi_3Mtw                 |

  Scenario Outline: Query values
    Given the Stripe spec
    When an app calls "https://api.stripe.com/v1/payment_intents?<query>"
    Then the query in url.full is "<recorded>"

    Examples:
      | query                         | recorded                          |
      | limit=10                      | limit=10                          |
      | customer=cus_77               | customer=REDACTED                 |
      | unknown=abc                   | unknown=REDACTED                  |
      | customer=a&limit=5            | customer=REDACTED&limit=5         |
      | customer=a&customer=b         | customer=REDACTED&customer=REDACTED |
      | verbose                       | verbose                           |
      | customer=cus%2077             | customer=REDACTED                 |

  @q11
  Scenario: Empty query values
    Given the Stripe spec
    When an app calls "https://api.stripe.com/v1/payment_intents?customer="
    Then the query in url.full is "customer="

  Scenario: A secret query parameter is always redacted
    Given a spec whose security section names the query parameter key, which is also marked x-hotline-redact false
    When an app calls with "?key=abc"
    Then the query in url.full is "key=REDACTED"

  @q3
  Scenario: A path the spec does not know
    Given the Talon.One spec
    When an app calls "https://api.talon.one/v2/unknown/abc"
    Then url.full is "https://api.talon.one/REDACTED/REDACTED/REDACTED"

  Scenario: User name and password inside the address
    Given the Talon.One spec
    When an app calls "https://user:pass@api.talon.one/v2/customer_sessions/1"
    Then url.full has no user name or password

  Scenario: A fragment is dropped
    Given the Talon.One spec
    When an app calls "https://api.talon.one/v2/customer_sessions/1#top"
    Then url.full has no fragment
