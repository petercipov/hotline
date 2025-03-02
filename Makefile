test:
	go test -cover ./servicelevels

deps:
	go mod download

lint:
	golangci-lint run
	govulncheck ./...
	gocritic check ./...