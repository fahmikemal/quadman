BINARY := quadman
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test vet fmt install clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

install:
	go install -ldflags "-X main.version=$(VERSION)" .

clean:
	rm -f $(BINARY)

e2e:
	cd e2e && go build -o /tmp/qe2e .
	$(MAKE) build
	cp $(BINARY) /tmp/quadman-under-test
	@echo "run /tmp/qe2e from a directory containing the quadman binary"
