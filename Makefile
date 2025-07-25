.SILENT:

test:
	go test ./src/hotline/... -coverprofile=./cover.out -covermode=atomic -coverpkg=./src/hotline/...
	go test ./src/app/... -coverprofile=./cover.app.out -covermode=atomic -coverpkg=./src/app/...

cover:
	-go tool cover -func cover.out | grep -v "100.0"
	cat cover.app.out | (grep -v "app/main.go" || true ) > cover.app.filtered.out
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

run-infra:
	docker-compose -f ./examples/infra/docker-compose.yml up