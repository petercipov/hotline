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
	"testing"

	"github.com/cucumber/godog"
	"github.com/stretchr/testify/assert"
)

type SchemaAPISut struct {
	apiURL func() string
	t      *testing.T
}

func NewSchemaAPISut(t *testing.T, apiURL func() string) *SchemaAPISut {
	return &SchemaAPISut{
		apiURL: apiURL,
		t:      t,
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
	if !assert.Equal(a.t, expectedSchemas, schemaList, "schemas do not match") {
		return ctx, errConfigDoNotMatch
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

	if !assert.NotEmpty(a.t, createSchema.JSON201.SchemaID) {
		return ctx, errConfigDoNotMatch
	}
	if !assert.NotEmpty(a.t, createSchema.JSON201.UpdatedAt) {
		return ctx, errConfigDoNotMatch
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

	assert.NotEmpty(a.t, schemaContent.HTTPResponse.Header.Get("Last-Modified"))
	assert.Equal(a.t, "application/octet-stream", schemaContent.HTTPResponse.Header.Get("Content-Type"))

	expectedContent, readExpectedErr := os.ReadFile(filepath.Clean(expectedFilePath))
	if readExpectedErr != nil {
		return ctx, readExpectedErr
	}

	receivedBody := string(schemaContent.Body)
	expectedBody := string(expectedContent)
	if !assert.Equal(a.t, expectedBody, receivedBody) {
		return ctx, errConfigDoNotMatch
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
