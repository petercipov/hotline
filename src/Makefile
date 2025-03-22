test:
	go test ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...

deps:
	go mod download

lint:
	golangci-lint run
	govulncheck ./...
	gocritic check ./...

run-infra:
	docker-compose -f ./examples/infra/docker-compose.yml up