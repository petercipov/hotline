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
	pipeline          *schemas.ValidatorPipeline
	schemaRepo        *schemas.InMemorySchemaRepository
	schemaUseCase     *schemas.SchemaUseCase
	validationRepo    *schemas.InMemoryValidationRepository
	validationUseCase *schemas.ValidationUseCase
	reporter          *schemas.InMemoryValidationReporter
}

func (s *validationPipelineSut) Close() {
	schemaList, listErr := s.schemaUseCase.ListSchemas(context.Background())
	Expect(listErr).NotTo(HaveOccurred())
	for _, schema := range schemaList {
		deleteErr := s.schemaUseCase.DeleteSchema(context.Background(), schema.ID)
		Expect(deleteErr).NotTo(HaveOccurred())
	}

	validationsList, listErr := s.validationRepo.GetValidations(
		context.Background(), "integration-id")
	Expect(listErr).NotTo(HaveOccurred())
	for _, route := range validationsList {
		deleteErr := s.validationRepo.DeleteRouteByKey(context.Background(), "integration-id", route.RouteKey)
		Expect(deleteErr).NotTo(HaveOccurred())
	}

	s.pipeline = nil
	s.schemaUseCase = nil
	s.schemaRepo = nil
	s.validationRepo = nil
	s.validationUseCase = nil
	s.reporter = nil
}

func (s *validationPipelineSut) forPipelineWithoutDefinition() {
	manualTime := clock.NewDefaultManualClock()
	s.schemaRepo = schemas.NewInMemorySchemaRepository()
	s.validationRepo = schemas.NewInMemoryValidationRepository()
	s.validationUseCase = schemas.NewValidationUseCase(s.validationRepo, s.schemaRepo)
	s.reporter = &schemas.InMemoryValidationReporter{}
	s.schemaUseCase = schemas.NewSchemaUseCase(
		s.schemaRepo,
		manualTime.Now,
		uuid.NewV7(&uuid.ConstantRandReader{}),
	)

	scopes := concurrency.NewScopes(
		concurrency.GenerateScopeIds("validators", 8),
		func() *schemas.ValidatorScope {
			return schemas.NewEmptyValidatorScope(
				s.schemaUseCase,
				s.validationRepo,
				s.reporter,
			)
		},
	)

	fanOut := concurrency.NewFanoutWithMessagesConsumer(scopes)
	publisher := concurrency.NewFanoutPublisher(fanOut)

	s.pipeline = schemas.NewValidatorPipeline(publisher)
	Expect(s.schemaUseCase.ListSchemas(context.Background())).To(BeEmpty())
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
	s.pipeline.IngestHttpRequest(context.Background(), message)

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
	schemaById, createErr := s.schemaUseCase.CreateSchema(context.Background(), `{
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
	}`, "header.schema.json")

	Expect(createErr).NotTo(HaveOccurred())
	Expect(schemaById.ID.String()).To(Equal("SCAZUtiVXQcQGBAQEBAQEBAQ"))
	Expect(schemaById.Title).To(Equal("header.schema.json"))
	Expect(schemaById.UpdatedAt.String()).To(Equal("2025-02-22 12:02:10 +0000 UTC"))
	Expect(s.schemaUseCase.ListSchemas(context.Background())).NotTo(BeEmpty())

	_, _ = s.validationUseCase.UpsertValidation(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteValidators{
			Request: &schemas.RequestValidators{
				HeaderSchemaID: &schemaById.ID,
			},
		})
}

func (s *validationPipelineSut) withInvalidHeaderSchemaValidation() {
	headersSchemaID := schemas.ID("some")
	schemaErr := s.schemaRepo.SetSchema(context.Background(), headersSchemaID, `invalid string`, time.Now(), "header.schema.json")
	Expect(schemaErr).NotTo(HaveOccurred())
	Expect(s.schemaRepo.ListSchemas(context.Background())).NotTo(BeEmpty())

	_, _ = s.validationUseCase.UpsertValidation(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteValidators{
			Request: &schemas.RequestValidators{
				HeaderSchemaID: &headersSchemaID,
			},
		})
}

func (s *validationPipelineSut) withQuerySchemaValidation() {
	entry, schemaErr := s.schemaUseCase.CreateSchema(context.Background(), `{
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
	}`, "query.schema.json")
	Expect(schemaErr).NotTo(HaveOccurred())
	Expect(s.schemaUseCase.ListSchemas(context.Background())).NotTo(BeEmpty())

	_, _ = s.validationUseCase.UpsertValidation(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteValidators{
			Request: &schemas.RequestValidators{
				QuerySchemaID: &entry.ID,
			},
		})
}

func (s *validationPipelineSut) withBodySchemaValidation() {
	entry, schemaErr := s.schemaUseCase.CreateSchema(context.Background(), `{
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
	}`, "body.schema.json")
	Expect(schemaErr).NotTo(HaveOccurred())
	Expect(s.schemaUseCase.ListSchemas(context.Background())).NotTo(BeEmpty())

	_, _ = s.validationUseCase.UpsertValidation(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/test",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteValidators{
			Request: &schemas.RequestValidators{
				BodySchemaID: &entry.ID,
			},
		})

	_, _ = s.validationUseCase.UpsertValidation(context.Background(), "integration-id",
		http.Route{
			Method:      "GET",
			PathPattern: "/alias",
			Host:        "example.com",
			Port:        80,
		},
		schemas.RouteValidators{
			Request: &schemas.RequestValidators{
				BodySchemaID: &entry.ID,
			},
		})
}
