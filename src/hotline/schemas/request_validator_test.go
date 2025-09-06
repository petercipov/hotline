package schemas_test

import (
	"hotline/schemas"
	"hotline/uuid"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Request Validator", func() {
	sut := validatorSut{}
	Context("for empty validator", func() {
		It("should validate request", func() {
			sut.forEmptyValidator()
			err := sut.validateRequest()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("for defined validator", func() {
		It("should validate headers", func() {
			sut.forValidatorWithHeaders()
			err := sut.validateRequest()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail to build validator for invalid json schema", func() {
			err := sut.forValidatorWithInvalidSchema("invalid schema")
			Expect(err).To(HaveOccurred())
		})

		It("should fail to build validator for valid json but invalid json schema", func() {
			err := sut.forValidatorWithInvalidSchema(`{ "$schema": 1234 }`)
			Expect(err).To(HaveOccurred())
		})

		It("will fail to build schema with remote refs", func() {
			err := sut.forValidatorWithInvalidSchema(`{
			  "$schema": "https://json-schema.org/draft/2020-12/schema#",
			  "type": "object",
			  "properties": {
				"address": {
				  "$ref": "https://json-schema.org/custom/remote/schems.json"
				}
			  }
			}`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`failing loading "https://json-schema.org/custom/remote/schems.json": do not support loading schemas from remote sources`))
		})

		It("should validate headers with errors", func() {
			sut.forValidatorWithHeadersSchema(`
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"type": "object",
					"properties": {
						"User-Agent": {
							"type": "array",
							"minItems": 0,
							"maxItems": 0,
							"items": {
								"type": "string"
							}
						}
					},
					"required": ["User-Agent"]
				}
			`)
			err := sut.validateRequest()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				`jsonschema validation failed with 'https://local-server/config-api/request-schemas/SCx3zt0ygAcQGBAQEBAQEBAQ/files/request-headers.json#'
- at '/User-Agent': maxItems: got 1, want 0`))
		})
	})
})

type validatorSut struct {
	validator   *schemas.RequestValidator
	idGenerator schemas.IDGenerator
}

func (s *validatorSut) forEmptyValidator() {

	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	s.idGenerator = idGenerator
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := schemas.NewRequestValidator(schemas.RequestSchema{
		ID: id,
	})
	Expect(err).ToNot(HaveOccurred())
	s.validator = validator
}

func (s *validatorSut) validateRequest() error {
	// GET https://github.com/petercipov/hotline/deferred-metadata/main/go.work
	err := s.validator.ValidateHeaders(map[string][]string{
		"User-Agent":              {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:142.0) Gecko/20100101 Firefox/142.0"},
		"Accept":                  {"application/json"},
		"Accept-Language":         {"en-US,en;q=0.5"},
		"Accept-Encoding":         {"gzip, deflate, br, zstd"},
		"Referer":                 {"https://github.com/petercipov/hotline/blob/main/go.work"},
		"Content-Type":            {"application/json"},
		"GitHub-Verified-Fetch":   {"true"},
		"X-Requested-With":        {"XMLHttpRequest"},
		"X-Fetch-Nonce":           {"v2:0000000-ac68-9fdc-ba63-000000000"},
		"X-GitHub-Client-Version": {"d69917a34df00fb39f728c4d820c9583d0e19a64"},
		"Connection":              {"keep-alive"},
		"Cookie":                  {"logged_in=yes; _device_id=00000; GHCC=Required:1-Analytics:1-SocialMedia:1-Advertising:1; MicrosoftApplicationsTelemetryDeviceId=0000; MSFPC=GUID=00000&HASH=e401&LV=202505&V=4&LU=1747936598773; saved_user_sessions=session; _octo=GH1.1.692467499.1754420398; user_session=session; __Host-user_session_same_site=session; dotcom_user=user; color_mode=%7B%22color_mode%22%3A%22auto%22%2C%22light_theme%22%3A%7B%22name%22%3A%22light%22%2C%22color_mode%22%3A%22light%22%7D%2C%22dark_theme%22%3A%7B%22name%22%3A%22dark%22%2C%22color_mode%22%3A%22dark%22%7D%7D; _gh_sess=token; cpu_bucket=sm; preferred_color_mode=dark; tz=Europe%2FPrague; fileTreeExpanded=true"},
		"Sec-Fetch-Dest":          {"empty"},
		"Sec-Fetch-Mode":          {"cors"},
		"Sec-Fetch-Site":          {"same-origin"},
		"Priority":                {"u=4"},
		"TE":                      {"trailers"},
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *validatorSut) forValidatorWithHeaders() {
	s.forValidatorWithHeadersSchema(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",

			"type": "object",
			"properties": {
			"User-Agent": {
				"type": "array",
					"minItems": 1,
					"maxItems": 1,
					"items": {
					"type": "string"
				}
			}
		},
		"required": ["User-Agent"]
	}`)
}

func (s *validatorSut) forValidatorWithHeadersSchema(schema string) {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	s.idGenerator = idGenerator
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := schemas.NewRequestValidator(schemas.RequestSchema{
		ID:      id,
		Headers: strings.NewReader(schema),
	})
	Expect(err).ToNot(HaveOccurred())
	s.validator = validator
}

func (s *validatorSut) forValidatorWithInvalidSchema(schema string) error {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	s.idGenerator = idGenerator
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := schemas.NewRequestValidator(schemas.RequestSchema{
		ID:      id,
		Headers: strings.NewReader(schema),
	})
	Expect(validator).To(BeNil())
	return err
}
