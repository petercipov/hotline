"""The shop's payment-qr function, for AWS Lambda (Python 3.13).

It gets an order and gives the payment text a bank app reads from the QR
code on the invoice, in the Czech and Slovak "short payment descriptor"
form. A wrong input raises an error, so Lambda answers with
X-Amz-Function-Error, which Hotline reports.
"""


def handler(event, context):
    order_id = event.get("orderId")
    amount = event.get("amount")
    currency = event.get("currency")
    if not order_id or not amount or not currency:
        raise ValueError("orderId, amount and currency are needed")
    return {"qrText": f"SPD*1.0*AM:{amount}*CC:{currency}*MSG:{order_id}"}
