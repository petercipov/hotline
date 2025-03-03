test:
	go test ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...

deps:
	go mod download

lint:
	golangci-lint run
	govulncheck ./...
	gocritic check ./...