GO       ?= go
BIN_DIR  ?= bin
BIN      := $(BIN_DIR)/llformat
VERSION  ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
LDFLAGS  ?= -X main.buildVersion=$(VERSION) -X main.buildCommit=$(COMMIT)

.PHONY: all build install unit test clean
.PHONY: fmt fmt-check lint self-check

all: build

# Build the llformat CLI
build: $(BIN)

$(BIN): FORCE $(shell find formatter -name '*.go') $(shell find cmd -name '*.go') go.mod go.sum Makefile
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $@ ./cmd/llformat

.PHONY: FORCE
FORCE:

# Install llformat to GOPATH/bin
install: build
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/llformat

# Run unit tests
unit test:
	$(GO) test -v ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt: build
	@$(GO) run ./tools/fmt_repo --llformat ./$(BIN) --write

.PHONY: fmt-check
fmt-check: build
	@$(GO) run ./tools/fmt_repo --llformat ./$(BIN)

.PHONY: self-binary-check
self-binary-check: build
	@tmp=$$(mktemp -t llformat_bin.XXXXXX); \
	cp $(BIN) $$tmp; \
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/llformat; \
	cmp -s $$tmp $(BIN); \
	rm -f $$tmp; \
	echo "Self-binary-check ok: reproducible build."

.PHONY: self-check
self-check: build fmt-check unit self-binary-check
	@echo "Self-check ok: fmt-check, unit, and self-binary-check passed."

.PHONY: gen-next-goldens
gen-next-goldens: build
	@echo "Generating next outputs into .next_goldens/ (not committed)..."
	@$(GO) run ./tools/gen_next_goldens

clean:
	rm -rf $(BIN_DIR)
