// Command aws-openapi writes the OpenAPI spec of an AWS JSON service from
// its Smithy model, for the specs folder:
//
//	go run . -record QueueUrl=messaging.destination.name models/sqs-2012-11-05.json > ../../specs/aws/sqs/2012-11-05.openapi.yaml
//
// Each -record FIELD=NAME marks that field of every action's input, so the
// proxy copies it into records as NAME.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"hotline-tools/aws-openapi/smithy"
)

var errRecord = errors.New("not FIELD=NAME")

func main() {
	record := map[string]string{}
	flag.Func("record", "FIELD=NAME: copy FIELD of every action's input into records as NAME", func(v string) error {
		field, name, ok := strings.Cut(v, "=")
		if !ok || field == "" || name == "" {
			return fmt.Errorf("%w: %q", errRecord, v)
		}
		record[field] = name
		return nil
	})
	flag.Parse()
	if flag.NArg() != 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: aws-openapi [-record FIELD=NAME]... MODEL.json > SPEC.openapi.yaml")
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0)) //nolint:gosec // reads the model the user names
	if err == nil {
		data, err = smithy.Convert(data, record)
	}
	if err == nil {
		_, err = os.Stdout.Write(data)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
