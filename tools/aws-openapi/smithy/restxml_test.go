package smithy_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-tools/aws-openapi/smithy"
)

// filesModel is a tiny AWS REST and XML service shaped like S3: the bucket
// in the host name, file names that take the rest of the path, actions
// told apart by query words, and XML bodies with AWS's XML tags.
const filesModel = `{
  "smithy": "2.0",
  "shapes": {
    "com.example#Files": {
      "type": "service",
      "version": "2006-03-01",
      "operations": [
        {"target": "com.example#ListFiles"}, {"target": "com.example#ListBuckets"},
        {"target": "com.example#GetFile"}, {"target": "com.example#PutFile"},
        {"target": "com.example#PutPart"}, {"target": "com.example#GetTags"},
        {"target": "com.example#DeleteFiles"}, {"target": "com.example#HeadFile"}
      ],
      "traits": {
        "aws.protocols#restXml": {"noErrorWrapping": true},
        "aws.api#service": {"endpointPrefix": "files"},
        "smithy.api#title": "Example Files"
      }
    },
    "com.example#ListFiles": {
      "type": "operation",
      "input": {"target": "com.example#ListFilesRequest"},
      "output": {"target": "com.example#ListFilesOutput"},
      "traits": {"smithy.api#http": {"method": "GET", "uri": "/{Bucket}?list-type=2", "code": 200}}
    },
    "com.example#ListBuckets": {
      "type": "operation",
      "output": {"target": "com.example#ListFilesOutput"},
      "traits": {"smithy.api#http": {"method": "GET", "uri": "/?x-id=ListBuckets", "code": 200}}
    },
    "com.example#GetFile": {
      "type": "operation",
      "input": {"target": "com.example#GetFileRequest"},
      "output": {"target": "com.example#GetFileOutput"},
      "traits": {"smithy.api#http": {"method": "GET", "uri": "/{Bucket}/{Key+}?x-id=GetFile", "code": 200}}
    },
    "com.example#PutFile": {
      "type": "operation",
      "input": {"target": "com.example#PutFileRequest"},
      "traits": {"smithy.api#http": {"method": "PUT", "uri": "/{Bucket}/{Key+}?x-id=PutFile", "code": 200}}
    },
    "com.example#PutPart": {
      "type": "operation",
      "input": {"target": "com.example#PutPartRequest"},
      "traits": {"smithy.api#http": {"method": "PUT", "uri": "/{Bucket}/{Key+}?x-id=PutPart", "code": 200}}
    },
    "com.example#GetTags": {
      "type": "operation",
      "input": {"target": "com.example#GetFileRequest"},
      "output": {"target": "com.example#GetTagsOutput"},
      "traits": {"smithy.api#http": {"method": "GET", "uri": "/{Bucket}/{Key+}?tagging", "code": 200}}
    },
    "com.example#DeleteFiles": {
      "type": "operation",
      "input": {"target": "com.example#DeleteFilesRequest"},
      "output": {"target": "com.example#ListFilesOutput"},
      "traits": {"smithy.api#http": {"method": "POST", "uri": "/{Bucket}?delete", "code": 200}}
    },
    "com.example#HeadFile": {
      "type": "operation",
      "input": {"target": "com.example#GetFileRequest"},
      "traits": {"smithy.api#http": {"method": "HEAD", "uri": "/{Bucket}/{Key+}", "code": 200}}
    },
    "com.example#ListFilesRequest": {
      "type": "structure",
      "members": {
        "Bucket": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "Prefix": {"target": "smithy.api#String", "traits": {"smithy.api#httpQuery": "prefix"}},
        "MaxKeys": {"target": "smithy.api#Integer", "traits": {"smithy.api#httpQuery": "max-keys"}},
        "Owner": {"target": "smithy.api#String", "traits": {"smithy.api#httpHeader": "x-amz-expected-bucket-owner"}}
      }
    },
    "com.example#ListFilesOutput": {
      "type": "structure",
      "members": {
        "Name": {"target": "smithy.api#String"},
        "Truncated": {"target": "smithy.api#Boolean", "traits": {"smithy.api#xmlName": "IsTruncated"}},
        "Contents": {"target": "com.example#FileList", "traits": {"smithy.api#xmlFlattened": {}}},
        "Charged": {"target": "smithy.api#String", "traits": {"smithy.api#httpHeader": "x-amz-request-charged"}}
      },
      "traits": {"smithy.api#xmlName": "ListBucketResult"}
    },
    "com.example#FileList": {"type": "list", "member": {"target": "com.example#File"}},
    "com.example#File": {
      "type": "structure",
      "members": {
        "Key": {"target": "smithy.api#String"},
        "Size": {"target": "smithy.api#Long"},
        "LastModified": {"target": "smithy.api#Timestamp"},
        "Owner": {"target": "com.example#Owner"}
      }
    },
    "com.example#Owner": {
      "type": "structure",
      "members": {"ID": {"target": "smithy.api#String"}, "Type": {"target": "smithy.api#String", "traits": {"smithy.api#xmlAttribute": {}, "smithy.api#xmlName": "xsi:type"}}},
      "traits": {"smithy.api#xmlName": "Person"}
    },
    "com.example#GetFileRequest": {
      "type": "structure",
      "members": {
        "Bucket": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "Key": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "Since": {"target": "smithy.api#Timestamp", "traits": {"smithy.api#httpHeader": "If-Modified-Since"}}
      }
    },
    "com.example#GetFileOutput": {
      "type": "structure",
      "members": {
        "Body": {"target": "com.example#Stream", "traits": {"smithy.api#httpPayload": {}}},
        "Length": {"target": "smithy.api#Long", "traits": {"smithy.api#httpHeader": "Content-Length"}}
      }
    },
    "com.example#Stream": {"type": "blob", "traits": {"smithy.api#streaming": {}}},
    "com.example#PutFileRequest": {
      "type": "structure",
      "members": {
        "Bucket": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "Key": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "Body": {"target": "com.example#Stream", "traits": {"smithy.api#httpPayload": {}}}
      }
    },
    "com.example#PutPartRequest": {
      "type": "structure",
      "members": {
        "Bucket": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "Key": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "PartNumber": {"target": "smithy.api#Integer", "traits": {"smithy.api#httpQuery": "partNumber", "smithy.api#required": {}}},
        "Body": {"target": "com.example#Stream", "traits": {"smithy.api#httpPayload": {}}}
      }
    },
    "com.example#GetTagsOutput": {
      "type": "structure",
      "members": {"TagSet": {"target": "com.example#TagSet", "traits": {"smithy.api#required": {}}}},
      "traits": {"smithy.api#xmlName": "Tagging"}
    },
    "com.example#TagSet": {"type": "list", "member": {"target": "com.example#Tag", "traits": {"smithy.api#xmlName": "Tag"}}},
    "com.example#Tag": {"type": "structure", "members": {"Key": {"target": "smithy.api#String"}, "Values": {"target": "com.example#Names"}}},
    "com.example#DeleteFilesRequest": {
      "type": "structure",
      "members": {
        "Bucket": {"target": "smithy.api#String", "traits": {"smithy.api#httpLabel": {}, "smithy.api#required": {}}},
        "Delete": {"target": "com.example#Delete", "traits": {"smithy.api#httpPayload": {}, "smithy.api#required": {}, "smithy.api#xmlName": "Delete"}}
      }
    },
    "com.example#Delete": {
      "type": "structure",
      "members": {
        "Objects": {"target": "com.example#Names", "traits": {"smithy.api#xmlFlattened": {}, "smithy.api#xmlName": "Object", "smithy.api#required": {}}},
        "Quiet": {"target": "smithy.api#Boolean"}
      }
    },
    "com.example#Names": {"type": "list", "member": {"target": "smithy.api#String"}, "traits": {"smithy.api#length": {"max": 1000}}}
  }
}`

var _ = Describe("Converting an AWS REST and XML service, such as S3, from Smithy to OpenAPI", func() {
	var doc map[string]any
	BeforeEach(func() { doc = convert(filesModel) })
	op := func(path, method string) any { return at(doc, "paths", path, method) }

	It("names no action header: actions are told apart by path and query words", func() {
		Expect(doc).NotTo(HaveKey("x-hotline-action-header"))
	})

	It("puts the bucket in the host name, as AWS's SDKs do", func() {
		Expect(at(doc, "servers")).To(Equal([]any{map[string]any{
			"url":       "https://{bucket}.files.{region}.amazonaws.com",
			"variables": map[string]any{"bucket": map[string]any{"default": "bucket"}, "region": map[string]any{"default": "us-east-1"}},
		}}))
	})

	It("writes each action's route without the bucket, with its query words after #", func() {
		Expect(op("/#?list-type=2", "get")).To(HaveKeyWithValue("operationId", "ListFiles"))
		Expect(op("/#?delete", "post")).To(HaveKeyWithValue("operationId", "DeleteFiles"))
		Expect(op("/{Key+}#?tagging", "get")).To(HaveKeyWithValue("operationId", "GetTags"))
		Expect(op("/{Key+}", "head")).To(HaveKeyWithValue("operationId", "HeadFile"))
	})

	It("leaves out actions that don't work on a bucket", func() {
		Expect(at(doc, "paths")).NotTo(HaveKey("/"))
		Expect(at(doc, "paths")).NotTo(HaveKey("/#?x-id=ListBuckets"))
	})

	It("drops the x-id word where the route is clear without it, and lists it as an optional query value", func() {
		Expect(op("/{Key+}", "get")).To(HaveKeyWithValue("operationId", "GetFile"))
		Expect(at(op("/{Key+}", "get"), "parameters")).To(ContainElement(map[string]any{
			"name": "x-id", "in": "query", "schema": map[string]any{"type": "string", "const": "GetFile"}, "x-hotline-redact": false,
		}))
	})

	It("keeps the x-id word where actions would share a route without it", func() {
		Expect(op("/{Key+}#?x-id=PutFile", "put")).To(HaveKeyWithValue("operationId", "PutFile"))
		Expect(op("/{Key+}#?x-id=PutPart", "put")).To(HaveKeyWithValue("operationId", "PutPart"))
	})

	It("lists the file name, query values and headers as parameters", func() {
		Expect(at(op("/{Key+}", "get"), "parameters")).To(ContainElements(
			map[string]any{"name": "Key", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "If-Modified-Since", "in": "header", "schema": map[string]any{"type": "string"}},
		))
		Expect(at(op("/#?list-type=2", "get"), "parameters")).To(ContainElements(
			map[string]any{"name": "prefix", "in": "query", "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "max-keys", "in": "query", "schema": map[string]any{"type": "integer", "format": "int32"}},
			map[string]any{"name": "x-amz-expected-bucket-owner", "in": "header", "schema": map[string]any{"type": "string"}},
		))
		Expect(at(op("/{Key+}#?x-id=PutPart", "put"), "parameters")).To(ContainElement(
			map[string]any{"name": "partNumber", "in": "query", "required": true, "schema": map[string]any{"type": "integer", "format": "int32"}},
		))
	})

	It("reads XML replies, marked to be read as XML, with the root element AWS names", func() {
		content := at(op("/#?list-type=2", "get"), "responses", "200", "content", "application/xml")
		Expect(at(content, "x-hotline-body")).To(Equal("xml"))
		Expect(at(content, "schema", "$ref")).To(Equal("#/components/schemas/ListFilesOutput"))
		Expect(at(doc, "components", "schemas", "ListFilesOutput", "xml")).To(Equal(map[string]any{"name": "ListBucketResult"}))
	})

	It("keeps headers out of the XML body, and lists them on the reply", func() {
		Expect(at(doc, "components", "schemas", "ListFilesOutput", "properties")).NotTo(HaveKey("Charged"))
		Expect(at(op("/#?list-type=2", "get"), "responses", "200", "headers", "x-amz-request-charged")).To(Equal(map[string]any{"schema": map[string]any{"type": "string"}}))
	})

	It("keeps AWS's request IDs in the record", func() {
		for _, h := range []string{"x-amz-request-id", "x-amz-id-2"} {
			Expect(at(op("/#?list-type=2", "get"), "responses", "200", "headers", h, "x-hotline-redact")).To(BeFalse())
		}
	})

	It("writes AWS's XML names, attributes and times", func() {
		output := at(doc, "components", "schemas", "ListFilesOutput", "properties")
		Expect(at(output, "Truncated")).To(Equal(map[string]any{"type": "boolean", "xml": map[string]any{"name": "IsTruncated"}}))
		file := at(doc, "components", "schemas", "File", "properties")
		Expect(at(file, "LastModified")).To(Equal(map[string]any{"type": "string", "format": "date-time"}))
		Expect(at(file, "Owner")).To(Equal(map[string]any{"$ref": "#/components/schemas/Owner", "xml": map[string]any{"name": "Owner"}}))
		Expect(at(doc, "components", "schemas", "Owner", "properties", "Type")).To(Equal(map[string]any{"type": "string", "xml": map[string]any{"name": "type", "attribute": true}}))
	})

	It("writes a flattened list as its items repeated, named like the member", func() {
		Expect(at(doc, "components", "schemas", "ListFilesOutput", "properties", "Contents")).To(Equal(map[string]any{
			"type": "array", "items": map[string]any{"$ref": "#/components/schemas/File", "xml": map[string]any{"name": "Contents"}},
		}))
		Expect(at(doc, "components", "schemas", "Delete", "properties", "Objects")).To(Equal(map[string]any{
			"type": "array", "maxItems": 1000, "items": map[string]any{"type": "string", "xml": map[string]any{"name": "Object"}},
		}))
	})

	It("writes any other list with an outer element, its items named member or as AWS names them", func() {
		Expect(at(doc, "components", "schemas", "GetTagsOutput", "properties", "TagSet")).To(Equal(map[string]any{
			"$ref": "#/components/schemas/TagSet", "xml": map[string]any{"wrapped": true},
		}))
		Expect(at(doc, "components", "schemas", "TagSet", "items", "xml")).To(Equal(map[string]any{"name": "Tag"}))
		Expect(at(doc, "components", "schemas", "Names", "items", "xml")).To(Equal(map[string]any{"name": "member"}))
	})

	It("reads an XML body sent as one member, with the member's element name as the root", func() {
		body := at(op("/#?delete", "post"), "requestBody")
		Expect(at(body, "required")).To(BeTrue())
		Expect(at(body, "content", "application/xml")).To(Equal(map[string]any{
			"schema":         map[string]any{"$ref": "#/components/schemas/Delete", "xml": map[string]any{"name": "Delete"}},
			"x-hotline-body": "xml",
		}))
	})

	It("passes file bodies of any type without reading them", func() {
		Expect(at(op("/{Key+}#?x-id=PutFile", "put"), "requestBody")).To(Equal(map[string]any{"content": map[string]any{"*/*": map[string]any{}}}))
		Expect(at(op("/{Key+}", "get"), "responses", "200", "content")).To(Equal(map[string]any{"*/*": map[string]any{}}))
	})

	It("sends no body where an action has none", func() {
		Expect(op("/{Key+}", "head")).NotTo(HaveKey("requestBody"))
		Expect(at(op("/{Key+}", "head"), "responses", "200")).NotTo(HaveKey("content"))
	})

	It("lists AWS's XML error reply on every action", func() {
		for _, status := range []string{"4XX", "5XX"} {
			reply := at(op("/#?list-type=2", "get"), "responses", status)
			Expect(at(reply, "content", "application/xml", "schema", "$ref")).To(Equal("#/components/schemas/AwsXmlError"))
			Expect(at(reply, "content", "application/xml", "x-hotline-body")).To(Equal("xml"))
			Expect(at(reply, "headers", "x-amz-request-id", "x-hotline-redact")).To(BeFalse())
		}
		Expect(at(doc, "components", "schemas", "AwsXmlError", "xml")).To(Equal(map[string]any{"name": "Error"}))
		Expect(at(doc, "components", "schemas", "AwsXmlError", "required")).To(ConsistOf("Code"))
	})

	It("refuses to list an action header, which a REST service doesn't have", func() {
		_, err := smithy.Convert([]byte(filesModel), smithy.Options{Action: "messaging.operation.name"})
		Expect(err).To(MatchError(smithy.ErrUnsupported))
	})

	It("made the committed S3 spec from the committed S3 model", func() {
		m, err := os.ReadFile("../models/s3-2006-03-01.json")
		Expect(err).NotTo(HaveOccurred())
		want, err := os.ReadFile("../../../specs/aws/s3/2006-03-01.openapi.yaml")
		Expect(err).NotTo(HaveOccurred())
		got, err := smithy.Convert(m, smithy.Options{})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(Equal(string(want)), "run: cd tools/aws-openapi && go run . models/s3-2006-03-01.json > ../../specs/aws/s3/2006-03-01.openapi.yaml")
	})
})
