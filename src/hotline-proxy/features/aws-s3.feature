# Requirements and decisions: docs/proxy-xml-bodies.md.
Feature: AWS S3 through the proxy, with the invoices example
  The example app (examples/aws-sqs-invoices) stores each invoice PDF in
  the S3 bucket shop-invoices, and the shop fetches, lists and cleans up the
  PDFs there, with the AWS SDK for Go. S3 drives the proxy's XML checks:
  its lists come back as XML, and deleting several files sends XML (Peter,
  2026-10-31).

  The app runs as its own program and talks to AWS's real host names; the
  proxy, with testdata/aws/hotline-proxy.yaml (only its addresses
  changed), reaches a fake SQS and a fake S3 by them. The bucket is in the
  host name, shop-invoices.s3.eu-west-1.amazonaws.com, as the SDK sends it.

  Background:
    Given a fake SQS and a fake S3 with the bucket shop-invoices
    And the proxy runs with the invoices hotline-proxy.yaml

  Scenario: The worker stores the PDF and the shop fetches it
    When the example app requests the invoice for order A-1001
    And the example app works through the queue once, into the bucket
    And the example app fetches the invoice for A-1001
    Then the app saved the same PDF the fake S3 holds as A-1001.pdf
    And the S3 records are PutObject with url.template /{Key+}#?x-id=PutObject and GetObject with url.template /{Key+}#?x-id=GetObject, with no results

  Scenario: An upload in AWS's chunks reaches S3 unchanged
    When the example app requests the invoice for order A-1001
    And the example app works through the queue once, into the bucket
    Then the fake S3 got the PDF in AWS's chunks with a matching checksum, signed by the SDK, over https, without Proxy-Authorization

  Scenario: A missing PDF is a normal error reply
    When the example app fetches the invoice for A-9999
    Then the app said there is no invoice for A-9999
    And the S3 record for GetObject has status 404 and no results

  Scenario: The shop lists the invoices, and the XML reply is checked
    Given the bucket holds A-1001.pdf and notes.txt
    When the example app lists the invoices
    Then the S3 record for ListObjectsV2 has url.template /#?list-type=2 and the reply results body $.Contents[1].Key pattern ^[A-Z]-[0-9]+\.pdf$

  Scenario: The shop deletes old invoices in one call, and the XML call is checked
    Given the bucket holds A-1001.pdf and A-1002.pdf, stored on 2026-10-01
    When the example app cleans up the invoices stored before 2026-11-01
    Then the fake S3 holds no files
    And the S3 record for DeleteObjects has url.template /#?delete and no results

  Scenario: Deleting a file that isn't an invoice is reported
    Given the bucket holds notes.txt, stored on 2026-10-01
    When the example app cleans up the invoices stored before 2026-11-01
    Then the S3 record for DeleteObjects has url.template /#?delete and the request results body $.Objects[0].Key pattern ^[A-Z]-[0-9]+\.pdf$

  Scenario: The PDF itself is never read or recorded
    When the example app requests the invoice for order A-1001
    And the example app works through the queue once, into the bucket
    Then no record holds the text of the PDF
