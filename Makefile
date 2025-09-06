.SILENT:

generate:
	cd ./src/app/setup/config && oapi-codegen -config=codegen.config.yaml config.openapi.yaml

test:
	go clean -testcache
	go test -p 1 ./src/hotline/... -coverprofile=./cover.out -covermode=atomic -coverpkg=./src/hotline/...
	go test -p 1 ./src/app/... -coverprofile=./cover.app.out -covermode=atomic -coverpkg=./src/app/...

cover:
	-go tool cover -func cover.out | grep -v "100.0"
	cat cover.app.out | (grep -v "app/main.go" || true ) | (grep -v "gen.go" || true ) > cover.app.filtered.out
	-go tool cover -func cover.app.filtered.out | grep -v "100.0"
	! go tool cover -func cover.out | grep -v "100.0" || exit 1
	#! go tool cover -func cover.app.out | grep -v "100.0" || exit 1

deps:
	go mod download

lint:
	govulncheck
	golangci-lint run ./src/hotline/...
	gocritic check ./src/hotline/...
	golangci-lint run ./src/app/...
	gocritic check ./src/app/...
	vacuum lint -d --no-banner -r ./.vacuum.rules.yaml ./src/app/setup/config/config.openapi.yaml -z

run-infra:
	docker-compose -f ./examples/infra/docker-compose.yml up