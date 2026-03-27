VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

BINARY  := memoria
BINDIR  := bin
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: build test lint clean

build:
	go build $(LDFLAGS) -o $(BINDIR)/$(BINARY) ./cmd/memoria/

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf $(BINDIR)
