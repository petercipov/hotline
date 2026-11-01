package smithy_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-tools/aws-openapi/smithy"
)

// functionsModel is a tiny AWS REST and JSON service shaped like Lambda
// and SES: actions told apart by method and path, values in the path,
// query and headers, and JSON bodies.
const functionsModel = `{
  "smithy": "2.0",
  "shapes": {
    "com.example#Functions": {
      "type": "service",
      "version": "2015-03-31",
      "operations": [{"target": "com.example#Invoke"}, {"target": "com.example#SendEmail"}],
      "resources": [{"target": "com.example#Function"}],
      "traits": {
        "aws.protocols#restJson1": {},
        "aws.api#service": {"endpointPrefix": "functions"},
        "smithy.api#title": "Example Functions"
      }
    },
    "com.example#Invoke": {
      "type": "operation",
      "input": {"target": "com.example#InvokeRequest"},
      "output": {"target": "com.example#InvokeResponse"},
      "traits": {"smithy.api#http": {"method": "POST", "uri": "/2015-03-31/functions/{FunctionName}/invocations", "code": 200}}
    },
    "com.example#Function": {
      "type": "resource",
      "identifiers": {"FunctionName": {"target": "smithy.api#String"}},
      "read": {"target": "com.example#GetFunction"}
    },
    "com.example#GetFunction": {
      "type": "operation",
      "input": {"target": "com.example#GetFunctionRequest"},
      "output": {"target": "com.example#GetFunctionResponse"},
      "traits": {"smithy.api#http": {"method": "GET", "uri": "/2015-03-31/functions/{FunctionName}", "code": 200}}
    },
    "com.example#SendEmail": {
      "type": "operation",
      "input": {"target": "com.example#SendEmailRequest"},
      "output": {"target": "com.example#SendEmailResponse"},
      "traits": {"smithy.api#http": {"method": "POST", "uri": "/v2/email/outbound-emails", "code": 200}}
    },
    "com.example#InvokeRequest": {
      "type": "structure",
      "members": {
        "FunctionName": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "InvocationType": {"target": "smithy.api#String", "traits": {"smithy.api#httpHeader": "X-Amz-Invocation-Type"}},
        "Qualifier": {"target": "smithy.api#String", "traits": {"smithy.api#httpQuery": "Qualifier"}},
        "Payload": {"target": "smithy.api#Blob", "traits": {"smithy.api#httpPayload": {}}}
      }
    },
    "com.example#InvokeResponse": {
      "type": "structure",
      "members": {
        "StatusCode": {"target": "smithy.api#Integer", "traits": {"smithy.api#httpResponseCode": {}}},
        "FunctionError": {"target": "smithy.api#String", "traits": {"smithy.api#httpHeader": "X-Amz-Function-Error"}},
        "Payload": {"target": "smithy.api#Blob", "traits": {"smithy.api#httpPayload": {}}}
      }
    },
    "com.example#GetFunctionRequest": {
      "type": "structure",
      "members": {
        "FunctionName": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}}
      }
    },
    "com.example#GetFunctionResponse": {
      "type": "structure",
      "members": {"Runtime": {"target": "smithy.api#String"}, "LastModified": {"target": "smithy.api#Timestamp"}}
    },
    "com.example#SendEmailRequest": {
      "type": "structure",
      "members": {
        "FromEmailAddress": {"target": "smithy.api#String"},
        "Destination": {"target": "com.example#Destination", "traits": {"smithy.api#required": {}}},
        "Tags": {"target": "com.example#TagList"}
      }
    },
    "com.example#Destination": {
      "type": "structure",
      "members": {"ToAddresses": {"target": "com.example#TagList"}}
    },
    "com.example#SendEmailResponse": {
      "type": "structure",
      "members": {"MessageId": {"target": "smithy.api#String", "traits": {"smithy.api#jsonName": "messageId"}}}
    },
    "com.example#TagList": {"type": "list", "member": {"target": "smithy.api#String"}}
  }
}`

var _ = Describe("Converting an AWS REST and JSON service, such as Lambda or SES, from Smithy to OpenAPI", func() {
	var doc map[string]any
	BeforeEach(func() { doc = convert(functionsModel) })
	op := func(path, method string) any { return at(doc, "paths", path, method) }
	invoke := func() any { return op("/2015-03-31/functions/{FunctionName}/invocations", "post") }

	It("names no action header or field: actions are told apart by method and path, also those grouped in a resource", func() {
		Expect(doc).NotTo(HaveKey("x-hotline-action-header"))
		Expect(doc).NotTo(HaveKey("x-hotline-action-field"))
		Expect(invoke()).To(HaveKeyWithValue("operationId", "Invoke"))
		Expect(op("/2015-03-31/functions/{FunctionName}", "get")).To(HaveKeyWithValue("operationId", "GetFunction"))
		Expect(op("/v2/email/outbound-emails", "post")).To(HaveKeyWithValue("operationId", "SendEmail"))
	})

	It("puts the service's host in the servers address", func() {
		Expect(at(doc, "servers")).To(Equal([]any{map[string]any{
			"url": "https://functions.{region}.amazonaws.com", "variables": map[string]any{"region": map[string]any{"default": "us-east-1"}},
		}}))
	})

	It("lists values in the path, the query and headers as parameters", func() {
		Expect(at(invoke(), "parameters")).To(ConsistOf(
			map[string]any{"name": "FunctionName", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "Qualifier", "in": "query", "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "X-Amz-Invocation-Type", "in": "header", "schema": map[string]any{"type": "string"}},
		))
	})

	It("passes a body sent whole of any type without reading it, for an overlay to describe", func() {
		Expect(at(invoke(), "requestBody")).To(Equal(map[string]any{"content": map[string]any{"*/*": map[string]any{}}}))
		Expect(at(invoke(), "responses", "200", "content")).To(Equal(map[string]any{"*/*": map[string]any{}}))
	})

	It("reads other bodies as JSON, with AWS's JSON names", func() {
		body := at(op("/v2/email/outbound-emails", "post"), "requestBody")
		Expect(at(body, "content", "application/json", "schema")).To(Equal(map[string]any{"$ref": "#/components/schemas/SendEmailRequest"}))
		Expect(at(doc, "components", "schemas", "SendEmailRequest", "required")).To(ConsistOf("Destination"))
		reply := at(op("/v2/email/outbound-emails", "post"), "responses", "200", "content", "application/json", "schema")
		Expect(reply).To(Equal(map[string]any{"$ref": "#/components/schemas/SendEmailResponse"}))
		Expect(at(doc, "components", "schemas", "SendEmailResponse", "properties")).To(HaveKey("messageId"))
	})

	It("writes lists as JSON arrays, and times as seconds", func() {
		Expect(at(doc, "components", "schemas", "TagList")).To(Equal(map[string]any{"type": "array", "items": map[string]any{"type": "string"}}))
		Expect(at(doc, "components", "schemas", "GetFunctionResponse", "properties", "LastModified")).To(Equal(map[string]any{"type": "number"}))
	})

	It("keeps values in the path, query and headers out of the body", func() {
		Expect(op("/2015-03-31/functions/{FunctionName}", "get")).NotTo(HaveKey("requestBody"))
	})

	It("lists reply headers, and keeps the status code out of the body", func() {
		Expect(at(invoke(), "responses", "200", "headers", "X-Amz-Function-Error")).To(Equal(map[string]any{"schema": map[string]any{"type": "string"}}))
	})

	It("keeps AWS's request ID in the record", func() {
		Expect(at(invoke(), "responses", "200", "headers", "x-amzn-RequestId", "x-hotline-redact")).To(BeFalse())
	})

	It("lists AWS's JSON error reply on every action, with the error's name kept in the record", func() {
		for _, status := range []string{"4XX", "5XX"} {
			reply := at(invoke(), "responses", status)
			Expect(at(reply, "content", "application/json", "schema", "$ref")).To(Equal("#/components/schemas/AwsRestJsonError"))
			Expect(at(reply, "headers", "x-amzn-RequestId", "x-hotline-redact")).To(BeFalse())
			Expect(at(reply, "headers", "X-Amzn-ErrorType", "x-hotline-redact")).To(BeFalse())
		}
	})

	It("refuses to list an action header, which a REST service doesn't have", func() {
		_, err := smithy.Convert([]byte(functionsModel), smithy.Options{Action: "rpc.method"})
		Expect(err).To(MatchError(smithy.ErrUnsupported))
	})

	for _, api := range []struct{ model, spec string }{
		{"lambda-2015-03-31.json", "lambda/2015-03-31"},
		{"sesv2-2019-09-27.json", "sesv2/2019-09-27"},
	} {
		It("made the committed spec "+api.spec+" from the committed model", func() {
			m, err := os.ReadFile("../models/" + api.model)
			Expect(err).NotTo(HaveOccurred())
			want, err := os.ReadFile("../../../specs/aws/" + api.spec + ".openapi.yaml")
			Expect(err).NotTo(HaveOccurred())
			got, err := smithy.Convert(m, smithy.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got)).To(Equal(string(want)), "run: cd tools/aws-openapi && go run . models/"+api.model+" > ../../specs/aws/"+api.spec+".openapi.yaml")
		})
	}
})
