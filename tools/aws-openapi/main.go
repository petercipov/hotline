// Command aws-openapi writes the OpenAPI spec of an AWS JSON service from
// its Smithy model, for the specs folder:
//
//	go run . models/sqs-2012-11-05.json > ../../specs/aws/sqs/2012-11-05.openapi.yaml
package main

import (
	"fmt"
	"os"

	"hotline-tools/aws-openapi/smithy"
)

func main() {
	if len(os.Args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: aws-openapi MODEL.json > SPEC.openapi.yaml")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1]) //nolint:gosec // reads the model the user names
	if err == nil {
		data, err = smithy.Convert(data)
	}
	if err == nil {
		_, err = os.Stdout.Write(data)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
