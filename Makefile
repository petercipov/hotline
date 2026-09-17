.SILENT:

gen_collector:
	~/go/bin/ocb --verbose --config ./src/otelcol-dev/builder-config.yaml

test:
	rm -f cover.out
	go clean -testcache
	go test  ./src/hotline/... -coverprofile=./cover.out -covermode=atomic -coverpkg=./src/hotline/...
	go test  ./src/otel-hotline/...

clean-cache:
	go clean -testcache
	go clean -cache

cover:
	test -f cover.out || { echo "cover.out not found, run 'make test' first"; exit 1; }
	go tool cover -func cover.out > cover.func.txt
	-grep -v "100.0" cover.func.txt
	! grep -q -v "100.0" cover.func.txt || exit 1

deps:
	go mod download

lint:
	golangci-lint run ./src/hotline/...
	golangci-lint run ./src/otel-hotline/...
