package schemas_test

import (
	"hotline/schemas"
	"hotline/uuid"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Request Validator", Ordered, func() {
	Context("for empty validator", func() {
		sut := validatorSut{}
		It("should validate request", func() {
			sut.forEmptyValidator()
			err := sut.validateRequest()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("for defined header validator", func() {
		sut := validatorSut{}
		It("should validate headers", func() {
			sut.forValidatorWithHeaders()
			err := sut.validateRequest()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail to build validator for invalid json schema", func() {
			err := sut.forValidatorWithInvalidHeaderSchema("invalid schema")
			Expect(err).To(HaveOccurred())
		})

		It("should fail to build validator for valid json but invalid json schema", func() {
			err := sut.forValidatorWithInvalidHeaderSchema(`{ "$schema": 1234 }`)
			Expect(err).To(HaveOccurred())
		})

		It("will fail to build schema with remote refs", func() {
			err := sut.forValidatorWithInvalidHeaderSchema(`{
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
				`jsonschema validation failed with 'https://local-server/api/v1/request-schemas/SCx3zt0ygAcQGBAQEBAQEBAQ/files/request-headers.json#'
- at '/User-Agent': maxItems: got 1, want 0`))
		})
	})

	Context("for defined query validator", func() {
		sut := validatorSut{}
		It("should validate query", func() {
			sut.forValidatorWithQuery()
			err := sut.validateRequest()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail for invalid query", func() {
			sut.forValidatorWithQuerySchema(`{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type": "object",
			"properties": {
				"productID": {
					"type": "array",
					"minItems": 1,
					"maxItems": 1,
					"items": {
						"type": "string",
						"pattern": "^A[0-9]{5}$"
					}
				}
			},
			"required": ["productID"]
		}`)
			err := sut.validateRequest()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`jsonschema validation failed with 'https://local-server/api/v1/request-schemas/SCx3zt0ygAcQGBAQEBAQEBAQ/files/request-query.json#'
- at '/productID/0': 'P12345' does not match pattern '^A[0-9]{5}$'`))
		})

		It("should fail to build validator for invalid json schema", func() {
			err := sut.forValidatorWithInvalidQuerySchema("invalid schema")
			Expect(err).To(HaveOccurred())
		})

		It("should fail to build validator for valid json but invalid json schema", func() {
			err := sut.forValidatorWithInvalidQuerySchema(`{ "$schema": 1234 }`)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("for defined body validator", func() {
		sut := validatorSut{}
		It("should validate body", func() {
			sut.forValidatorWithBody()
			err := sut.validateRequest()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate body schema", func() {
			sut.forValidatorWithBodySchema(`{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type": "object",
				"properties": {
					"productID": {
						"type": "string",
						"pattern": "^P[0-9]{5}$"
					},
					"currency": {
						"type": "string",
						"enum": ["EUR", "GBP"]
					}
				},
				"required": ["productID"]
			}`)
			err := sut.validateRequest()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`jsonschema validation failed with 'https://local-server/api/v1/request-schemas/SCx3zt0ygAcQGBAQEBAQEBAQ/files/request-body.json#'
- at '/currency': value must be one of 'EUR', 'GBP'`))
		})

		It("should fail to build validator for invalid json schema", func() {
			err := sut.forValidatorWithInvalidBodySchema("invalid schema")
			Expect(err).To(HaveOccurred())
		})

		It("should fail to build validator for valid json but invalid json schema", func() {
			err := sut.forValidatorWithInvalidBodySchema(`{ "$schema": 1234 }`)
			Expect(err).To(HaveOccurred())
		})

		It("should validate invalid body", func() {
			sut.forValidatorWithBody()
			err := sut.validateInvalidJSONBodyRequest()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid character 'i' looking for beginning of value"))
		})

		It("should validate missing content", func() {
			sut.forValidatorWithBody()
			err := sut.validateMissingContent()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no content provided"))
		})
	})

	Context("for defined response header validator", func() {
		sut := validatorSut{}
		It("should validate headers", func() {
			sut.forValidatorWithResponseHeaders()
			err := sut.validateResponse()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail to build validator for invalid json schema", func() {
			err := sut.forValidatorWithInvalidHeaderResponseSchema("invalid schema")
			Expect(err).To(HaveOccurred())
		})

		It("should validate headers with errors", func() {
			sut.forValidatorWithHeadersResponseSchema(`
				{
					"$schema": "https://json-schema.org/draft/2020-12/schema",
					"type": "object",
					"properties": {
						"Required-Header": {
							"type": "array",
							"minItems": 0,
							"maxItems": 0,
							"items": {
								"type": "string"
							}
						}
					},
					"required": ["Required-Header"]
				}
			`)
			err := sut.validateResponse()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				`jsonschema validation failed with 'https://local-server/api/v1/request-schemas/SCx3zt0ygAcQGBAQEBAQEBAQ/files/response-headers.json#'
- at '': missing property 'Required-Header'`))
		})

	})

	Context("for defined response body validator", func() {
		sut := validatorSut{}
		It("should validate body", func() {
			sut.forValidatorWithResponseBody()
			err := sut.validateResponse()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate body schema", func() {
			sut.forValidatorWithReponseBodySchema(`{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type": "object",
				"properties": {
					"productID": {
						"type": "string",
						"pattern": "^P[0-9]{5}$"
					},
					"currency": {
						"type": "string",
						"enum": ["EUR", "GBP"]
					}
				},
				"required": ["productID"]
			}`)
			err := sut.validateResponse()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`jsonschema validation failed with 'https://local-server/api/v1/request-schemas/SCx3zt0ygAcQGBAQEBAQEBAQ/files/response-body.json#'
- at '/currency': value must be one of 'EUR', 'GBP'`))
		})

		It("should fail to build validator for invalid json schema", func() {
			err := sut.forValidatorWithInvalidResponseBodySchema("invalid schema")
			Expect(err).To(HaveOccurred())
		})

		It("should fail to build validator for valid json but invalid json schema", func() {
			err := sut.forValidatorWithInvalidResponseBodySchema(`{ "$schema": 1234 }`)
			Expect(err).To(HaveOccurred())
		})

		It("should validate invalid body", func() {
			sut.forValidatorWithResponseBody()
			err := sut.validateInvalidJSONBodyResponse()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid character 'i' looking for beginning of value"))
		})
	})
})

type validatorSut struct {
	validator *schemas.Validator
}

func (s *validatorSut) forEmptyValidator() {
	validator, err := buildValidator(schemas.RequestSchema{})
	Expect(err).ToNot(HaveOccurred())
	s.validator = validator
}

func (s *validatorSut) validateRequest() error {
	return s.validateRequestWithBody(`
	{
		"productID": "P12345",
		"title": "Hotline",
		"price": 59.99,
		"currency": "USD"
	}`)
}

func (s *validatorSut) validateRequestWithBody(body string) error {
	_, err := s.validator.ValidateHeaders(map[string][]string{
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

	_, err = s.validator.ValidateQuery(map[string][]string{
		"productID": {"P12345"},
	})
	if err != nil {
		return err
	}

	_, err = s.validator.ValidateBody(strings.NewReader(body))
	if err != nil {
		return err
	}

	return nil
}

func (s *validatorSut) validateInvalidJSONBodyRequest() error {
	return s.validateRequestWithBody("invalid json body")
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
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		RequestHeaders: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(err).ToNot(HaveOccurred())
	s.validator = validator
}

func (s *validatorSut) forValidatorWithInvalidHeaderSchema(schema string) error {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		RequestHeaders: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(validator).To(BeNil())
	return err
}

func (s *validatorSut) forValidatorWithInvalidQuerySchema(schema string) error {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		RequestQuery: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(validator).To(BeNil())
	return err
}

func (s *validatorSut) forValidatorWithInvalidBodySchema(schema string) error {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		RequestBody: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(validator).To(BeNil())
	return err
}

func buildValidator(schema schemas.RequestSchema) (*schemas.Validator, error) {
	return schemas.NewRequestValidator(schema)
}

func (s *validatorSut) forValidatorWithQuery() {
	s.forValidatorWithQuerySchema(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"productID": {
				"type": "array",
				"minItems": 1,
				"maxItems": 1,
				"items": {
					"type": "string",
					"pattern": "^P[0-9]{5}$"
				}
			}
		},
		"required": ["productID"]
	}`)
}

func (s *validatorSut) forValidatorWithQuerySchema(schema string) {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		RequestQuery: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(err).ToNot(HaveOccurred())
	s.validator = validator
}

func (s *validatorSut) forValidatorWithBody() {
	s.forValidatorWithBodySchema(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"productID": {
				"type": "string",
				"pattern": "^P[0-9]{5}$"
			},
			"title": {
				"type": "string",
				"minLength": 1,
				"maxLength": 100
			},
			"price": {
				"type": "number",
				"minimum": 0
			},
			"currency": {
				"type": "string",
				"enum": ["USD", "EUR", "GBP"]
			}
		},
		"required": ["productID"]
	}`)
}

func (s *validatorSut) forValidatorWithBodySchema(schema string) {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		RequestBody: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(err).ToNot(HaveOccurred())
	s.validator = validator
}

func (s *validatorSut) forValidatorWithResponseHeaders() {
	s.forValidatorWithHeadersResponseSchema(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"Content-Type": {
				"type": "array",
				"minItems": 1,
				"maxItems": 1,
				"items": {
					"type": "string"
				}
			}
		},
		"required": ["Content-Type"]
	}`)
}

func (s *validatorSut) forValidatorWithHeadersResponseSchema(schema string) {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		ResponseHeaders: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(err).ToNot(HaveOccurred())
	s.validator = validator
}

func (s *validatorSut) validateResponse() error {
	_, headerErr := s.validator.ValidateResponseHeaders(map[string][]string{
		"Content-Type": {"application/json"},
	})
	if headerErr != nil {
		return headerErr
	}

	_, respErr := s.validator.ValidateResponseBody(strings.NewReader(`{
		"productID": "P12345",
		"title": "Hotline",
		"price": 59.99,
		"currency": "USD"
	}`))
	if respErr != nil {
		return respErr
	}
	return nil
}

func (s *validatorSut) forValidatorWithInvalidHeaderResponseSchema(schema string) error {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	_, err := buildValidator(schemas.RequestSchema{
		ResponseHeaders: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(err).To(HaveOccurred())
	return err
}

func (s *validatorSut) forValidatorWithResponseBody() {
	s.forValidatorWithReponseBodySchema(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"productID": {
				"type": "string",
				"pattern": "^P[0-9]{5}$"
			},
			"title": {
				"type": "string",
				"minLength": 1,
				"maxLength": 100
			},
			"price": {
				"type": "number",
				"minimum": 0
			},
			"currency": {
				"type": "string",
				"enum": ["USD", "EUR", "GBP"]
			}
		},
		"required": ["productID"]
	}`)
}

func (s *validatorSut) forValidatorWithReponseBodySchema(schema string) {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	validator, err := buildValidator(schemas.RequestSchema{
		ResponseBody: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(err).NotTo(HaveOccurred())
	Expect(validator).NotTo(BeNil())
	s.validator = validator
}

func (s *validatorSut) forValidatorWithInvalidResponseBodySchema(schema string) error {
	idGenerator := schemas.NewIDGenerator(uuid.NewDeterministicV7(&uuid.ConstantRandReader{}))
	id, idErr := idGenerator(time.Time{})
	Expect(idErr).ToNot(HaveOccurred())

	_, err := buildValidator(schemas.RequestSchema{
		ResponseBody: &schemas.SchemaDefinition{
			Content: strings.NewReader(schema),
			ID:      id,
		},
	})

	Expect(err).To(HaveOccurred())
	return err
}

func (s *validatorSut) validateInvalidJSONBodyResponse() error {
	return s.validateResponseWithBody("invalid json body")
}

func (s *validatorSut) validateResponseWithBody(bodyString string) error {
	_, headerErr := s.validator.ValidateResponseHeaders(map[string][]string{
		"Content-Type": {"application/json"},
	})
	if headerErr != nil {
		return headerErr
	}

	_, bodyErr := s.validator.ValidateResponseBody(strings.NewReader(bodyString))
	if bodyErr != nil {
		return bodyErr
	}
	return nil
}

func (s *validatorSut) validateMissingContent() error {
	_, err := s.validator.ValidateBody(nil)
	if err != nil {
		return err
	}
	return nil
}
