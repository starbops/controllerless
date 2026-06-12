.PHONY: build test test-integration lint install-crd run example-task clean

BINARY := bin/controllerless
CRD_MANIFEST := internal/crd/scheduledtask/crd.yaml
EXAMPLE_MANIFEST := examples/nightly-backup.yaml

build:
	go build -o $(BINARY) ./cmd/controllerless

test:
	go test ./...

test-integration:
	go test -tags integration ./...

lint:
	golangci-lint run

install-crd:
	kubectl apply -f $(CRD_MANIFEST)

run: build
	./$(BINARY)

example-task:
	kubectl apply -f $(EXAMPLE_MANIFEST)

clean:
	rm -rf bin/
