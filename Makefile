.PHONY: build test lint smoke clean

build:
	go build -o bin/lens ./cmd/lens

test:
	go test ./...

lint:
	golangci-lint run

smoke: build
	./bin/lens run examples/customer-support/experiment-smoke.yaml

clean:
	rm -rf bin/
