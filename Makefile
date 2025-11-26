GO       ?= go
BIN_DIR  ?= bin
BIN      := $(BIN_DIR)/llformat

.PHONY: all build install unit test clean

all: build

# Build the llformat CLI
build: $(BIN)

$(BIN): $(shell find formatter -name '*.go') $(shell find cmd -name '*.go') go.mod go.sum
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $@ ./cmd/llformat

# Install llformat to GOPATH/bin
install: build
	$(GO) install ./cmd/llformat

# Run unit tests
unit test:
	$(GO) test -v ./...

clean:
	rm -rf $(BIN_DIR)
