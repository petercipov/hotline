package schemas_test

import (
	"context"
	"hotline/clock"
	"hotline/concurrency"
	"hotline/http"
	"hotline/schemas"
	"hotline/uuid"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Request Validator", func() {
	sut := validationPipelineSut{}

	AfterEach(func() {
		sut.Close()
	})

	Context("Header Validation", func() {
		It("can validate request without validation definition", func() {
			sut.forPipelineWithoutDefinition()
			result := sut.validateValidRequest()
			Expect(result.Errors).To(BeEmpty())
			Expect(result.Success).To(BeEmpty())
			Expect(string(result.RequestID)).To(Equal("request-id"))
			Expect(string(result.IntegrationID)).To(Equal("integration-id"))
			Expect(result.Timestamp).To(Equal(clock.ParseTime("2025-02-22T12:04:55Z")))
		})

		It("can validate request with defined schema validation", func() {
			sut.forPipelineWithoutDefinition()
			sut.withHeaderSchemaValidation()
			result := sut.validateValidRequest()
			Expect(result.Errors).To(BeEmpty())
			Expect(result.Success).NotTo(BeEmpty())
		})

		It("can validate invalid request with missing headers", func() {
			sut.forPipelineWithoutDefinition()
			sut.withHeaderSchemaValidation()
			result := sut.validateInvalidRequest()
			Expect(result.Success).To(BeEmpty())
			Expect(result.Errors).To(HaveLen(1))
		})

		It("will skip unknown query", func() {
			sut.forPipelineWithoutDefinition()
			sut.withHeaderSchemaValidation()
			result := sut.validateUnknownRequest()
			Expect(result.Success).To(BeEmpty())
			Expect(result.Errors).To(BeEmpty())
		})

		It("will skip invalid request when validator schemas are invalid", func() {
			sut.forPipelineWithoutDefinition()
			sut.withInvalidHeaderSchemaValidation()
			result := sut.validateInvalidRequest()
			Expect(result.Success).To(BeEmpty())
			Expect(result.Errors).To(BeEmpty())
		})
	})

	Context("Query Validation", func() {
		It("can validate request with query definition", func() {
			sut.forPipelineWithoutDefinition()
			sut.withQuerySchemaValidation()
			result := sut.validateValidRequest()
			Expect(result.Errors).To(BeEmpty())
			Expect(result.Success).To(HaveLen(1))
		})

		It("can validate invalid request with query definition", func() {
			sut.forPipelineWithoutDefinition()
			sut.withQuerySchemaValidation()
			result := sut.validateInvalidRequest()
			Expect(result.Errors).To(HaveLen(1))
			Expect(result.Success).To(BeEmpty())
		})
	})

	Context("Body Validation", func() {
		It("can validate request with body definition", func() {
			sut.forPipelineWithoutDefinition()
			sut.withBodySchemaValidation()
			result := sut.validateValidRequest()
			Expect(result.Errors).To(BeEmpty())
			Expect(result.Success).To(HaveLen(1))
		})

		It("can validate invalid request with body definition", func() {
			sut.forPipelineWithoutDefinition()
			sut.withBodySchemaValidation()
			result := sut.validateInvalidRequest()
			Expect(result.Success).To(BeEmpty())
			Expect(result.Errors).To(HaveLen(1))
		})
	})
})

type validationPipelineSut struct {
	pipeline       *schemas.ValidatorPipeline
	schemaRepo     *schemas.InMemorySchemaRepository
	validationRepo *schemas.InMemoryValidationRepository
	reporter       *schemas.InMemoryValidationReporter
}

func (s *validationPipelineSut) Close() {
	schemaList := s.schemaRepo.ListSchemas(context.Background())
	for _, schema := range schemaList {
		deleteErr := s.schemaRepo.DeleteSchema(context.Background(), schema.ID)
		Expect(deleteErr).NotTo(HaveOccurred())
	}

	validationsList := s.validationRepo.GetConfig(
		context.Background(), "integration-id")
	if validationsList != nil {
		for _, route := range validationsList.Routes {
			deleteErr := s.validationRepo.DeleteRouteByKey(context.Background(), "integration-id", route.RouteKey)
			Expect(deleteErr).NotTo(HaveOccurred())
		}
	}

	s.pipeline = nil
	s.schemaRepo = nil
	s.validationRepo = nil
	s.reporter = nil
}

func (s *validationPipelineSut) forPipelineWithoutDefinition() {
	s.schemaRepo = schemas.NewInMemorySchemaRepository(
		uuid.NewV7(&uuid.ConstantRandReader{}),
	)
	s.validationRepo = schemas.NewInMemoryValidationRepository()
	s.reporter = &schemas.InMemoryValidationReporter{}

	scopes := concurrency.NewScopes(
		concurrency.GenerateScopeIds("validators", 8),
		func() *schemas.ValidatorScope {
			return schemas.NewEmptyValidatorScope(
				s.schemaRepo,
				s.validationRepo,
				s.reporter,
			)
		},
	)
	s.pipeline = schemas.NewValidatorPipeline(scopes)

	Expect(s.schemaRepo.ListSchemas(context.Background())).To(BeEmpty())
}

func (s *validationPipelineSut) validateInvalidRequest() schemas.ValidationResult {
	return s.validateRequest(&schemas.ValidateRequestMessage{
		ID:            "request-id",
		IntegrationID: "integration-id",
		Now:           clock.ParseTime("2025-02-22T12:04:55Z"),
		Request: schemas.RequestContent{
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/test",
				Host:   "example.com",
				Port:   80,
			},
		},
	})
}

func (s *validationPipelineSut) validateUnknownRequest() schemas.ValidationResult {
	return s.validateRequest(&schemas.ValidateRequestMessage{
		ID:            "request-id",
		IntegrationID: "integration-id",
		Now:           clock.ParseTime("2025-02-22T12:04:55Z"),
		Request: schemas.RequestContent{
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/unknown",
				Host:   "example.com",
				Port:   80,
			},
			Headers: map[string][]string{},
		},
	})
}

func (s *validationPipelineSut) validateValidRequest() schemas.ValidationResult {
	return s.validateRequest(&schemas.ValidateRequestMessage{
		ID:            "request-id",
		IntegrationID: "integration-id",
		Now:           clock.ParseTime("2025-02-22T12:04:55Z"),
		Request: schemas.RequestContent{
			Locator: http.RequestLocator{
				Method: "GET",
				Path:   "/test",
				Host:   "example.com",
				Port:   80,
			},
			Headers: map[string][]string{
				"User-Agent": {"nginex"},
			},
			Query: map[string][]string{
				"productID": {"P12345"},
			},
			Body: strings.NewReader(`{
				"productID": "P12345",
				"title": "Hotline",
				"price": 59.99,
				"currency": "USD"
			}`),
		},
	})
}

func (s *validationPipelineSut) validateRequest(message *schemas.ValidateRequestMessage) schemas.ValidationResult {
	s.pipeline.IngestHttpRequest(message)

	count := 0
	for {
		time.Sleep(time.Millisecond * 1)
		result := s.reporter.GetResults()
		if len(result) != 0 {
			return result[0]
		}
		count++
		if count > 1000 {
			break
		}
	}
	return schemas.ValidationResult{}
}

func (s *validationPipelineSut) withHeaderSchemaValidation() {
	headersSchemaID, _ := s.schemaRepo.GenerateID(time.UnixMicro(0))
	now := time.Now()
	schemaErr := s.schemaRepo.SetSchema(context.Background(), headersSchemaID, `{
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
	}`, now, "header.schema.json")
	Expect(schemaErr).NotTo(HaveOccurred())

	schemaById, getErr := s.schemaRepo.GetSchemaByID(context.Background(), headersSchemaID)
	Expect(getErr).NotTo(HaveOccurred())
	Expect(schemaById.ID).To(Equal(headersSchemaID))
	Expect(schemaById.UpdatedAt).To(Equal(now))
	Expect(schemaById.Content).NotTo(BeEmpty())
	Expect(s.schemaRepo.ListSchemas(context.Background())).NotTo(BeEmpty())

	_, _ = s.validationRepo.SetForRoute(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteSchemaDefinition{
			Request: &schemas.RequestSchemaDefinition{
				HeaderSchemaID: &headersSchemaID,
			},
		})
}

func (s *validationPipelineSut) withInvalidHeaderSchemaValidation() {
	headersSchemaID, _ := s.schemaRepo.GenerateID(time.UnixMicro(0))
	schemaErr := s.schemaRepo.SetSchema(context.Background(), headersSchemaID, `invalid string`, time.Now(), "header.schema.json")
	Expect(schemaErr).To(HaveOccurred())
	Expect(s.schemaRepo.ListSchemas(context.Background())).To(BeEmpty())

	_, _ = s.validationRepo.SetForRoute(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteSchemaDefinition{
			Request: &schemas.RequestSchemaDefinition{
				HeaderSchemaID: &headersSchemaID,
			},
		})
}

func (s *validationPipelineSut) withQuerySchemaValidation() {
	querySchemaID, _ := s.schemaRepo.GenerateID(time.UnixMicro(1))

	schemaErr := s.schemaRepo.SetSchema(context.Background(), querySchemaID, `{
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
	}`, time.Now(), "query.schema.json")
	Expect(schemaErr).NotTo(HaveOccurred())
	Expect(s.schemaRepo.ListSchemas(context.Background())).NotTo(BeEmpty())

	_, _ = s.validationRepo.SetForRoute(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteSchemaDefinition{
			Request: &schemas.RequestSchemaDefinition{
				QuerySchemaID: &querySchemaID,
			},
		})
}

func (s *validationPipelineSut) withBodySchemaValidation() {
	bodySchemaID, _ := s.schemaRepo.GenerateID(time.UnixMicro(3))
	schemaErr := s.schemaRepo.SetSchema(context.Background(), bodySchemaID, `{
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
	}`, time.Now(), "body.schema.json")
	Expect(schemaErr).NotTo(HaveOccurred())
	Expect(s.schemaRepo.ListSchemas(context.Background())).NotTo(BeEmpty())

	_, _ = s.validationRepo.SetForRoute(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteSchemaDefinition{
			Request: &schemas.RequestSchemaDefinition{
				BodySchemaID: &bodySchemaID,
			},
		})

	_, _ = s.validationRepo.SetForRoute(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/alias",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteSchemaDefinition{
			Request: &schemas.RequestSchemaDefinition{
				BodySchemaID: &bodySchemaID,
			},
		})
}
