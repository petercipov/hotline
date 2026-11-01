#!/usr/bin/env bash
# run-shop.sh runs the shop's whole story in one go, through Hotline, on
# the AWS resources that already exist (see README.md). It never creates or
# removes anything.
#
#   1. checkout: a customer pays an order; it is saved and announced
#   2. invoices: the PDF is made, signed, stored, saved and announced
#   3. mailer:   the invoice is mailed (written to ./mail, for now)
#   4. refunds:  a refund starts the credit note workflow
#   5. two mistakes on purpose, which still reach AWS and which Hotline
#      reports: a broken invoice job, and a write to a table that is not
#      checkout's own
#
# It expects Hotline to be running already, and reads the usual AWS
# settings (AWS_PROFILE, AWS_REGION, ...) and HOTLINE_PROXY. It stops at
# the first step that goes wrong, and says which.
#
#   HOTLINE_PROXY=http://localhost:8080 ./run-shop.sh
#
# Settings:
#   ORDER        the order to pay (default A-1001)
#   SEND_TO      where the invoice is mailed (default customer@example.com)
#   WRONG_TABLE  the table for the second mistake (default orders-archive,
#                a table that doesn't exist, so nothing is written there)
set -euo pipefail

cd "$(dirname "$0")"
export HOTLINE_PROXY="${HOTLINE_PROXY:-http://localhost:8080}"
order="${ORDER:-A-1001}"
send_to="${SEND_TO:-customer@example.com}"
wrong_table="${WRONG_TABLE:-orders-archive}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p mail
summary=()

echo "Building aws-shop"
go build -o "$work/aws-shop" .
shop() { "$work/aws-shop" "$@"; }

# step runs one command and stops the script when it fails.
step() {
  local name="$1"
  shift
  echo
  echo "== $name"
  if ! "$@"; then
    echo
    echo "Stopped: the step \"$name\" failed. Is Hotline running at $HOTLINE_PROXY?"
    exit 1
  fi
  summary+=("done      $name")
}

# mistake runs one command that is wrong on purpose. It may fail, as AWS
# may refuse it; Hotline reports it either way.
mistake() {
  local name="$1"
  shift
  echo
  echo "== $name (a mistake on purpose)"
  if "$@"; then
    summary+=("sent      $name")
  else
    summary+=("refused   $name (AWS said no; Hotline still reports it)")
  fi
}

# mailed waits for the shop's EventBridge rule to put InvoiceReady on the
# mail queue, then mails it.
mailed() {
  for _ in 1 2 3 4 5; do
    shop mailer work --once --out mail
    if [[ -f "mail/$order.eml" ]]; then
      echo "mailed mail/$order.eml"
      return 0
    fi
    sleep 2
  done
  echo "no InvoiceReady event for $order reached the mail queue"
  return 1
}

step "checkout pays $order" shop checkout pay --order "$order" --language en --send-to "$send_to"
step "invoices makes the invoice for $order" shop invoices work --once
step "invoices shows the invoice of $order" shop invoices status --order "$order"
step "mailer mails the invoice of $order" mailed
step "refunds starts a credit note for $order" shop refunds start --order "$order" --amount 10.00 --reason returned

mistake "checkout sends a broken invoice job" \
  shop checkout send --body "{\"schemaVersion\":1,\"orderId\":\"$order\"}"
mistake "checkout writes an order to the table $wrong_table" \
  env ORDERS_TABLE="$wrong_table" "$work/aws-shop" checkout pay --order "$order" --language en

echo
echo "== Summary"
printf '  %s\n' "${summary[@]}"
cat <<EOF

Each app is its own integration in Hotline: look for the integration IDs
checkout, invoices, mailer and refunds in the records. The steps above
should have no results; the two mistakes should each have one:
  - the broken job:   body \$.MessageBody.eventId required, and more
  - the wrong table:  body \$.TableName const orders
The broken job stays on the invoices queue, where the worker refuses it
until SQS moves it to the dead letter queue.
EOF
