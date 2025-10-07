package setup_test

import (
	"app/setup/config"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

type SchemaAPISut struct {
	apiURL func() string
}

func NewSchemaAPISut(apiURL func() string) *SchemaAPISut {
	return &SchemaAPISut{
		apiURL: apiURL,
	}
}

func (a *SchemaAPISut) AddSteps(sctx *godog.ScenarioContext) {
	sctx.Step(`schema list is`, a.checkSchemaList)
	sctx.Step(`schema is created from file "([^"]*)"`, a.createSchema)
	sctx.Step(`schema content for "([^"]*)" is same as in file "([^"]*)"`, a.compareSchemaContent)
	sctx.Step(`schema "([^"]*)" is deleted`, a.deleteSchema)
	sctx.Step(`schema "([^"]*)" is upserted from file "([^"]*)"`, a.schemaIsUpsertedFromFile)
}

func (a *SchemaAPISut) checkSchemaList(ctx context.Context, configRaw string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	listResponse, listErr := configClient.ListRequestSchemasWithResponse(ctx)
	if listErr != nil {
		return ctx, listErr
	}

	if listResponse.StatusCode() != 200 && listResponse.StatusCode() != 404 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, listResponse.StatusCode())
	}

	schemaList := *listResponse.JSON200

	var expectedSchemas config.ListRequestSchemas
	jsonErr := json.Unmarshal([]byte(configRaw), &expectedSchemas)
	if jsonErr != nil {
		return ctx, jsonErr
	}

	eqErr := ObjectsAreEqual(expectedSchemas, schemaList, "schemas do not match")
	if eqErr != nil {
		return ctx, eqErr
	}
	return ctx, nil
}

func (a *SchemaAPISut) createSchema(ctx context.Context, filePath string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}
	schemaFile, openErr := os.Open(filepath.Clean(filePath))
	if openErr != nil {
		return ctx, openErr
	}
	defer func() {
		_ = schemaFile.Close()
	}()

	buff, _ := io.ReadAll(schemaFile)
	createSchema, createErr := configClient.CreateRequestSchemaWithBodyWithResponse(
		ctx,
		&config.CreateRequestSchemaParams{Title: &filePath},
		"application/octet-stream",
		bytes.NewReader(buff),
	)
	if createErr != nil {
		return ctx, createErr
	}

	if createSchema.StatusCode() != 201 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, createSchema.StatusCode())
	}

	if err := ObjectNotEmpty(createSchema.JSON201.SchemaID, "schema_id is empty"); err != nil {
		return ctx, err
	}
	if err := ObjectNotEmpty(createSchema.JSON201.UpdatedAt, "updated_at is empty"); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func (a *SchemaAPISut) compareSchemaContent(ctx context.Context, schemaID string, expectedFilePath string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	schemaContent, getErr := configClient.GetRequestSchemaWithResponse(
		ctx,
		schemaID,
	)
	if getErr != nil {
		return ctx, getErr
	}

	if schemaContent.StatusCode() != 200 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, schemaContent.StatusCode())
	}

	if err := ObjectNotEmpty(schemaContent.HTTPResponse.Header.Get("Last-Modified")); err != nil {
		return ctx, err
	}
	if err := ObjectsAreEqual("application/octet-stream", schemaContent.HTTPResponse.Header.Get("Content-Type")); err != nil {
		return ctx, err
	}

	expectedContent, readExpectedErr := os.ReadFile(filepath.Clean(expectedFilePath))
	if readExpectedErr != nil {
		return ctx, readExpectedErr
	}

	receivedBody := string(schemaContent.Body)
	expectedBody := string(expectedContent)

	if err := ObjectsAreEqual(expectedBody, receivedBody, "body do not match"); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func (a *SchemaAPISut) schemaIsUpsertedFromFile(ctx context.Context, schemaID string, schemaFilePath string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	schemaFile, openErr := os.Open(filepath.Clean(schemaFilePath))
	if openErr != nil {
		return ctx, openErr
	}
	defer func() {
		_ = schemaFile.Close()
	}()

	buff, _ := io.ReadAll(schemaFile)

	resp, respErr := configClient.PutRequestSchemaWithBodyWithResponse(
		ctx,
		schemaID,
		&config.PutRequestSchemaParams{Title: &schemaFilePath},
		"application/octet-stream",
		bytes.NewReader(buff),
	)
	if respErr != nil {
		return ctx, respErr
	}
	if resp.StatusCode() != http.StatusCreated {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, resp.StatusCode())
	}
	return ctx, nil
}

func (a *SchemaAPISut) deleteSchema(ctx context.Context, schemaID string) (context.Context, error) {
	configClient, createClientErr := config.NewClientWithResponses(a.apiURL())
	if createClientErr != nil {
		return ctx, createClientErr
	}

	response, deleteErr := configClient.DeleteRequestSchemaWithResponse(ctx, schemaID)
	if deleteErr != nil {
		return ctx, deleteErr
	}

	if response.StatusCode() != 204 {
		return ctx, fmt.Errorf("%w status code: %d", errUnexpectedResponse, response.StatusCode())
	}
	return ctx, nil
}
