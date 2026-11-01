package shop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
)

// Refund reads the paid order and starts the credit note workflow for an
// amount of it, giving the workflow run's ARN.
func (a App) Refund(ctx context.Context, orderID, amount, reason string) (string, error) {
	row, err := a.DynamoDB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(a.Names.OrdersTable),
		Key:       map[string]types.AttributeValue{"orderId": &types.AttributeValueMemberS{Value: orderID}},
	})
	if err != nil {
		return "", fmt.Errorf("read the order: %w", err)
	}
	currency, ok := row.Item["currency"].(*types.AttributeValueMemberS)
	if !ok {
		return "", fmt.Errorf("%s: %w", orderID, ErrNoOrder)
	}
	input, err := json.Marshal(map[string]string{"orderId": orderID, "amount": amount, "currency": currency.Value, "reason": reason})
	if err != nil {
		return "", err
	}
	run, err := a.SFN.StartExecution(ctx, &sfn.StartExecutionInput{
		StateMachineArn: aws.String(a.Names.CreditNoteWorkflow),
		Input:           aws.String(string(input)),
	})
	if err != nil {
		return "", fmt.Errorf("start the credit note: %w", err)
	}
	return aws.ToString(run.ExecutionArn), a.note(ctx, "refund for "+orderID+" started", "orderId", orderID, "amount", amount)
}
