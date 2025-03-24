test:
	go test ./src/hotline/... -coverprofile=./cover.out -covermode=atomic -coverpkg=./src/hotline/...

deps:
	go mod download

lint:
	golangci-lint run ./src/hotline/...
	govulncheck
	gocritic check ./src/hotline/...

run-infra:
	docker-compose -f ./examples/infra/docker-compose.yml up