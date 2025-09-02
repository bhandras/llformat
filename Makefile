GO       ?= go
BIN_DIR  ?= bin

.PHONY: all build unit test clean

# Default: build all commands under cmd/*
all: build

# Discover command directories and map to binaries in bin/
CMD_DIRS   := $(wildcard cmd/*)
BINARIES   := $(patsubst cmd/%,$(BIN_DIR)/%,$(CMD_DIRS))

# Rebuild when sources change
SRC := $(shell find formatter -name '*.go') $(shell find cmd -name '*.go') go.mod

build: $(BINARIES)

$(BIN_DIR)/%: $(SRC)
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $@ ./cmd/$*

# Run unit tests
unit test:
	$(GO) test -v ./...

clean:
	rm -rf $(BIN_DIR)
