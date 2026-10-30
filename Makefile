.SILENT:

gen_collector:
	~/go/bin/ocb --verbose --config ./src/otelcol-dev/builder-config.yaml

test:
	rm -f cover.out
	go clean -testcache
	go test  ./src/hotline/... ./src/hotline-proxy/... -coverprofile=./cover.out -covermode=atomic -coverpkg=./src/hotline/...,./src/hotline-proxy/...
	go test  ./src/otel-hotline/...
	$(MAKE) e2e
	tail -n +2 e2e-cover.out >> cover.out

# e2e runs the @e2e scenarios against the built hotline-proxy program. The
# program is built to record coverage, which lands in e2e-cover.out.
e2e:
	rm -rf e2e-cover e2e-cover.out
	mkdir -p e2e-cover/data
	cd src/hotline-proxy && go build -cover -covermode=atomic -coverpkg=./... -o ../../e2e-cover/hotline-proxy ./cmd
	HOTLINE_PROXY=$(CURDIR)/e2e-cover/hotline-proxy GOCOVERDIR=$(CURDIR)/e2e-cover/data \
		go test -tags e2e -count=1 -run TestEndToEnd ./src/hotline-proxy/
	go tool covdata textfmt -i=e2e-cover/data -o e2e-cover.out

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
	golangci-lint run ./src/hotline-proxy/...
	golangci-lint run ./src/otel-hotline/...
