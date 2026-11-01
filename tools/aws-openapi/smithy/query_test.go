package smithy_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-tools/aws-openapi/smithy"
)

// topicsModel is a tiny AWS query service shaped like SNS: every call is
// POST / with a form body naming the action in the field Action, and every
// reply is XML wrapped in the action's name.
const topicsModel = `{
  "smithy": "2.0",
  "shapes": {
    "com.example#Topics": {
      "type": "service",
      "version": "2010-03-31",
      "operations": [{"target": "com.example#Publish"}, {"target": "com.example#DeleteTopic"}, {"target": "com.example#GetAttributes"}],
      "traits": {
        "aws.protocols#awsQuery": {},
        "aws.api#service": {"endpointPrefix": "topics"},
        "smithy.api#title": "Example Topics"
      }
    },
    "com.example#Publish": {
      "type": "operation",
      "input": {"target": "com.example#PublishInput"},
      "output": {"target": "com.example#PublishResponse"}
    },
    "com.example#DeleteTopic": {
      "type": "operation",
      "input": {"target": "com.example#DeleteTopicInput"},
      "output": {"target": "smithy.api#Unit"}
    },
    "com.example#GetAttributes": {
      "type": "operation",
      "input": {"target": "com.example#DeleteTopicInput"},
      "output": {"target": "com.example#GetAttributesResponse"}
    },
    "com.example#PublishInput": {
      "type": "structure",
      "members": {
        "TopicArn": {"target": "smithy.api#String"},
        "Message": {"target": "smithy.api#String", "traits": {"smithy.api#required": {}}},
        "Delay": {"target": "smithy.api#Integer"},
        "Tags": {"target": "com.example#TagList", "traits": {"smithy.api#required": {}}},
        "MessageAttributes": {"target": "com.example#AttributeMap"},
        "Settings": {"target": "com.example#Settings"}
      }
    },
    "com.example#DeleteTopicInput": {
      "type": "structure",
      "members": {"TopicArn": {"target": "smithy.api#String", "traits": {"smithy.api#required": {}}}}
    },
    "com.example#PublishResponse": {
      "type": "structure",
      "members": {
        "MessageId": {"target": "smithy.api#String"},
        "SentAt": {"target": "smithy.api#Timestamp"},
        "Tags": {"target": "com.example#TagList"}
      }
    },
    "com.example#GetAttributesResponse": {
      "type": "structure",
      "members": {"Attributes": {"target": "com.example#AttributeMap"}}
    },
    "com.example#TagList": {"type": "list", "member": {"target": "smithy.api#String"}},
    "com.example#AttributeMap": {
      "type": "map",
      "key": {"target": "smithy.api#String", "traits": {"smithy.api#xmlName": "Name"}},
      "value": {"target": "smithy.api#String", "traits": {"smithy.api#xmlName": "Value"}}
    },
    "com.example#Settings": {"type": "structure", "members": {"Raw": {"target": "smithy.api#Boolean"}}}
  }
}`

var _ = Describe("Converting an AWS query service, such as SNS or STS, from Smithy to OpenAPI", func() {
	var doc map[string]any
	BeforeEach(func() { doc = convert(topicsModel) })
	op := func(path string) any { return at(doc, "paths", path, "post") }
	form := func(path string) any {
		return at(op(path), "requestBody", "content", "application/x-www-form-urlencoded", "schema")
	}

	It("names the body field Action as the one that picks the route", func() {
		Expect(doc).To(HaveKeyWithValue("x-hotline-action-field", "Action"))
		Expect(doc).NotTo(HaveKey("x-hotline-action-header"))
	})

	It("writes every action as POST /, named after #", func() {
		Expect(op("/#Action=Publish")).To(HaveKeyWithValue("operationId", "Publish"))
		Expect(op("/#Action=DeleteTopic")).To(HaveKeyWithValue("operationId", "DeleteTopic"))
	})

	It("puts the service's host in the servers address", func() {
		Expect(at(doc, "servers")).To(Equal([]any{map[string]any{
			"url": "https://topics.{region}.amazonaws.com", "variables": map[string]any{"region": map[string]any{"default": "us-east-1"}},
		}}))
	})

	It("reads the call as a form body, with Action and Version fixed", func() {
		Expect(at(op("/#Action=Publish"), "requestBody", "required")).To(BeTrue())
		Expect(at(form("/#Action=Publish"), "properties", "Action")).To(Equal(map[string]any{"type": "string", "const": "Publish"}))
		Expect(at(form("/#Action=Publish"), "properties", "Version")).To(Equal(map[string]any{"type": "string", "const": "2010-03-31"}))
		Expect(at(form("/#Action=Publish"), "required")).To(ConsistOf("Action", "Version", "Message"))
	})

	It("checks plain fields with their type", func() {
		props := at(form("/#Action=Publish"), "properties")
		Expect(at(props, "Message")).To(Equal(map[string]any{"type": "string"}))
		Expect(at(props, "Delay")).To(Equal(map[string]any{"type": "integer", "format": "int32"}))
	})

	It("lets lists, maps and structures through unchecked, since AWS writes them with dots (Tags.member.1)", func() {
		props := at(form("/#Action=Publish"), "properties")
		Expect(props).NotTo(HaveKey("Tags"))
		Expect(props).NotTo(HaveKey("MessageAttributes"))
		Expect(props).NotTo(HaveKey("Settings"))
		Expect(form("/#Action=Publish")).NotTo(HaveKey("additionalProperties"))
	})

	It("reads the reply as XML wrapped in the action's name, with the result inside", func() {
		reply := at(op("/#Action=Publish"), "responses", "200", "content", "text/xml", "schema")
		Expect(at(reply, "xml")).To(Equal(map[string]any{"name": "PublishResponse"}))
		Expect(at(reply, "properties", "PublishResult")).To(Equal(map[string]any{"$ref": "#/components/schemas/PublishResponse"}))
		Expect(at(reply, "properties", "ResponseMetadata", "properties", "RequestId")).To(Equal(map[string]any{"type": "string"}))
		Expect(at(doc, "components", "schemas", "PublishResponse", "properties", "SentAt")).To(Equal(map[string]any{"type": "string", "format": "date-time"}))
	})

	It("writes a list in the reply with an outer element and items named member", func() {
		Expect(at(doc, "components", "schemas", "PublishResponse", "properties", "Tags")).To(Equal(map[string]any{
			"$ref": "#/components/schemas/TagList", "xml": map[string]any{"wrapped": true},
		}))
		Expect(at(doc, "components", "schemas", "TagList", "items", "xml")).To(Equal(map[string]any{"name": "member"}))
	})

	It("writes a map in the reply as entry elements, each with its key and value", func() {
		Expect(at(doc, "components", "schemas", "GetAttributesResponse", "properties", "Attributes")).To(Equal(map[string]any{
			"$ref": "#/components/schemas/AttributeMap", "xml": map[string]any{"wrapped": true},
		}))
		Expect(at(doc, "components", "schemas", "AttributeMap")).To(Equal(map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"xml":  map[string]any{"name": "entry"},
				"properties": map[string]any{
					"Name":  map[string]any{"type": "string"},
					"Value": map[string]any{"type": "string"},
				},
			},
		}))
	})

	It("has only the request ID in the reply of an action with no result", func() {
		reply := at(op("/#Action=DeleteTopic"), "responses", "200", "content", "text/xml", "schema")
		Expect(at(reply, "xml")).To(Equal(map[string]any{"name": "DeleteTopicResponse"}))
		Expect(at(reply, "properties")).To(HaveLen(1))
		Expect(at(reply, "properties")).To(HaveKey("ResponseMetadata"))
	})

	It("lists AWS's query error reply on every action, and keeps the request ID in the record", func() {
		for _, status := range []string{"4XX", "5XX"} {
			reply := at(op("/#Action=Publish"), "responses", status)
			Expect(at(reply, "content", "text/xml", "schema", "$ref")).To(Equal("#/components/schemas/AwsQueryError"))
			Expect(at(reply, "headers", "x-amzn-RequestId", "x-hotline-redact")).To(BeFalse())
		}
		Expect(at(doc, "components", "schemas", "AwsQueryError", "xml")).To(Equal(map[string]any{"name": "ErrorResponse"}))
		Expect(at(doc, "components", "schemas", "AwsQueryError", "properties", "Error", "required")).To(ConsistOf("Code"))
	})

	It("marks input fields and the Action field to be copied into records", func() {
		doc := convertRecording(topicsModel, smithy.Options{Record: map[string]string{"TopicArn": "messaging.destination.name"}, Action: "messaging.operation.name"})
		props := at(doc, "paths", "/#Action=Publish", "post", "requestBody", "content", "application/x-www-form-urlencoded", "schema", "properties")
		Expect(at(props, "TopicArn", "x-hotline-record")).To(Equal("messaging.destination.name"))
		Expect(at(props, "Action", "x-hotline-record")).To(Equal("messaging.operation.name"))
	})

	for _, api := range []struct{ model, spec, flags string }{
		{"sns-2010-03-31.json", "sns/2010-03-31", "-record TopicArn=messaging.destination.name -record-action messaging.operation.name"},
		{"sts-2011-06-15.json", "sts/2011-06-15", "-record-action rpc.method"},
	} {
		It("made the committed spec "+api.spec+" from the committed model", func() {
			m, err := os.ReadFile("../models/" + api.model)
			Expect(err).NotTo(HaveOccurred())
			want, err := os.ReadFile("../../../specs/aws/" + api.spec + ".openapi.yaml")
			Expect(err).NotTo(HaveOccurred())
			got, err := smithy.Convert(m, options(api.flags))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got)).To(Equal(string(want)), "run: cd tools/aws-openapi && go run . "+api.flags+" models/"+api.model+" > ../../specs/aws/"+api.spec+".openapi.yaml")
		})
	}
})
