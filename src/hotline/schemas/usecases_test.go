package schemas_test

import (
	"context"
	"errors"
	"hotline/clock"
	"hotline/http"
	"hotline/schemas"
	"hotline/uuid"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Usecases", func() {
	sut := UsecaseSut{}
	AfterEach(func() {
		err := sut.Close()
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Schema", func() {
		It("creates valid schema", func() {
			sut.forEmptySchemas()
			schemaId := sut.withValidSchema()
			Expect(schemaId).NotTo(BeEmpty())
			Expect(sut.SchemaExists(schemaId)).To(BeTrue())
		})

		It("will NOT create invalid schema", func() {
			sut.forEmptySchemas()
			err := sut.createInvalidSchema()
			var validationErr *schemas.ValidationError
			Expect(errors.As(err, &validationErr)).To(BeTrue())
		})

		It("create multiple schemas", func() {
			sut.forEmptySchemas()
			schemaId := sut.withValidSchema()
			schemaId2 := sut.withValidSchema()
			Expect(schemaId).NotTo(BeEmpty())
			Expect(schemaId2).NotTo(BeEmpty())
			Expect(schemaId).NotTo(Equal(schemaId2))
			Expect(sut.SchemaExists(schemaId)).To(BeTrue())
			Expect(sut.SchemaExists(schemaId2)).To(BeTrue())
		})

		It("can delete schema", func() {
			sut.forEmptySchemas()
			schemaId := sut.withValidSchema()
			sut.deleteSchema(schemaId)
			Expect(schemaId).NotTo(BeEmpty())
			Expect(sut.SchemaExists(schemaId)).To(BeFalse())
		})

		It("modifies valid schema", func() {
			sut.forEmptySchemas()
			schemaId := sut.withValidSchema()
			Expect(schemaId).NotTo(BeEmpty())
			Expect(sut.SchemaExists(schemaId)).To(BeTrue())
			entry := sut.GetSchema(schemaId)
			Expect(entry.Title).To(Equal("test-schema"))

			sut.UpdateSchema(schemaId)
			entry = sut.GetSchema(schemaId)
			Expect(entry.Title).To(Equal("test-schema-updated"))
		})

		It("will NOT modify invalid schema", func() {
			sut.forEmptySchemas()
			schemaID := sut.withValidSchema()
			err := sut.ModifyInvalid(schemaID)
			var validationErr *schemas.ValidationError
			Expect(errors.As(err, &validationErr)).To(BeTrue())
		})

		It("will NOT delete not existing schema", func() {
			sut.forEmptySchemas()
			err := sut.schemaNotDeleted("not-existing")
			Expect(err).To(Equal(schemas.ErrSchemaNotFound))
		})

		It("will NOT modify not existing schema", func() {
			sut.forEmptySchemas()
			err := sut.schemaNotModified("not-existing")
			Expect(err).To(Equal(schemas.ErrSchemaNotFound))
		})

		It("will not create schema when generating id will not pass", func() {
			sut.forSchemasWithFailingGenerator()
			err := sut.createInvalidSchema()
			Expect(err).To(Equal(io.EOF))
		})
	})

	Context("Validation", func() {
		It("should create schema and attach it to route validator", func() {
			sut.forEmptySchemas()
			schemaId := sut.withValidSchema()
			key := sut.AddValidation(http.Route{
				Method:      "GET",
				PathPattern: "/products",
				Host:        "localhost",
				Port:        8080,
			}, schemaId)
			Expect(schemaId).NotTo(BeEmpty())
			Expect(key.String()).NotTo(BeEmpty())
			Expect(sut.SchemaExists(schemaId)).To(BeTrue())
			Expect(sut.ValidatorExists(key)).To(BeTrue())
		})

		It("can delete validation", func() {
			sut.forEmptySchemas()
			schemaId := sut.withValidSchema()
			key := sut.AddValidation(http.Route{
				Method:      "GET",
				PathPattern: "/products",
				Host:        "localhost",
				Port:        8080,
			}, schemaId)
			sut.deleteValidation(key)
			Expect(schemaId).NotTo(BeEmpty())
			Expect(key.String()).NotTo(BeEmpty())
			Expect(sut.ValidatorExists(key)).To(BeFalse())
		})

		It("will NOT update validation for non existing key", func() {
			sut.forEmptySchemas()
			err := sut.AddValidationWithErr(http.Route{
				Method:      "GET",
				PathPattern: "/products",
				Host:        "localhost",
				Port:        8080,
			}, "non existing id")

			Expect(err).To(Equal(schemas.ErrSchemaNotFound))
		})
	})
})

type UsecaseSut struct {
	schema    *schemas.SchemaUseCase
	validator *schemas.ValidationUseCase
}

func (sut *UsecaseSut) Close() error {
	return nil
}

func (sut *UsecaseSut) build(generator uuid.V7StringGenerator) {
	manualClock := clock.NewDefaultManualClock()
	schemaRepo := schemas.NewInMemorySchemaRepository()
	validatorRepo := schemas.NewInMemoryValidationRepository()
	sut.schema = schemas.NewSchemaUseCase(
		schemaRepo,
		manualClock.Now,
		generator,
	)
	sut.validator = schemas.NewValidationUseCase(validatorRepo, schemaRepo)
}

func (sut *UsecaseSut) forEmptySchemas() {
	sut.build(uuid.NewV7(&uuid.ConstantRandReader{}))
}

func (sut *UsecaseSut) forSchemasWithFailingGenerator() {
	sut.build(uuid.NewV7(&uuid.ErrorRandReader{}))
}

func (sut *UsecaseSut) AddValidation(route http.Route, id schemas.ID) http.RouteKey {
	routeKey, upsertErr := sut.validator.UpsertValidation(
		context.Background(),
		"integration-id",
		route,
		schemas.RouteValidators{
			Request: &schemas.RequestValidators{
				BodySchemaID: &id,
			},
		})
	Expect(upsertErr).NotTo(HaveOccurred())
	return routeKey
}

func (sut *UsecaseSut) AddValidationWithErr(route http.Route, id schemas.ID) error {
	_, upsertErr := sut.validator.UpsertValidation(
		context.Background(),
		"integration-id",
		route,
		schemas.RouteValidators{
			Request: &schemas.RequestValidators{
				BodySchemaID: &id,
			},
		})
	Expect(upsertErr).To(HaveOccurred())
	return upsertErr
}

func (sut *UsecaseSut) withValidSchema() schemas.ID {
	entry, createErr := sut.schema.CreateSchema(
		context.Background(),
		`{
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
		}`,
		`test-schema`,
	)
	Expect(createErr).ToNot(HaveOccurred())
	return entry.ID
}

func (sut *UsecaseSut) SchemaExists(id schemas.ID) bool {
	list, listErr := sut.schema.ListSchemas(context.Background())
	Expect(listErr).ToNot(HaveOccurred())
	for _, schema := range list {
		if schema.ID == id {
			return true
		}
	}
	return false
}

func (sut *UsecaseSut) ValidatorExists(key http.RouteKey) bool {
	validations, getErr := sut.validator.GetValidations(context.Background(), "integration-id")
	Expect(getErr).ToNot(HaveOccurred())
	for _, validation := range validations {
		if validation.RouteKey == key {
			return true
		}
	}
	return false
}

func (sut *UsecaseSut) deleteSchema(id schemas.ID) {
	deleteErr := sut.schema.DeleteSchema(context.Background(), id)
	Expect(deleteErr).ToNot(HaveOccurred())
}

func (sut *UsecaseSut) deleteValidation(key http.RouteKey) {
	deleteErr := sut.validator.DeleteValidation(context.Background(), "integration-id", key)
	Expect(deleteErr).ToNot(HaveOccurred())
}

func (sut *UsecaseSut) UpdateSchema(id schemas.ID) {
	modifyErr := sut.schema.ModifySchema(
		context.Background(),
		id,
		`{
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
		  "required": ["productID", "title"]
		}`,
		`test-schema-updated`,
	)
	Expect(modifyErr).ToNot(HaveOccurred())
}

func (sut *UsecaseSut) createInvalidSchema() error {
	_, modifyErr := sut.schema.CreateSchema(
		context.Background(),
		`invalid schema`,
		`test-schema-invalid`,
	)
	Expect(modifyErr).To(HaveOccurred())
	return modifyErr
}

func (sut *UsecaseSut) GetSchema(id schemas.ID) schemas.SchemaEntry {
	entry, getErr := sut.schema.GetSchema(context.Background(), id)
	Expect(getErr).ToNot(HaveOccurred())
	return entry
}

func (sut *UsecaseSut) schemaNotDeleted(id schemas.ID) error {
	err := sut.schema.DeleteSchema(context.Background(), id)
	Expect(err).To(HaveOccurred())
	return err
}

func (sut *UsecaseSut) schemaNotModified(id schemas.ID) error {
	return sut.schema.ModifySchema(context.Background(), id, `{
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
		}`, "not-existing-id")
}

func (sut *UsecaseSut) ModifyInvalid(id schemas.ID) error {
	return sut.schema.ModifySchema(context.Background(), id,
		`invalid schema content`, "modify invalid schema")
}
