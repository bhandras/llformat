# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Agent Guidelines

This repository contains golden outputs used by tests under `testdata/**/output_next.go`. These files are **authoritative** fixtures and must not be altered by automation.

- Never modify `testdata/**/output_next.go` or any other golden results when making changes.
- If a change appears to require updating these golden files, stop immediately and ask a human maintainer for direction.
- Do not regenerate or rewrite the goldens as part of formatter changes or test fixes.

When in doubt about expected behavior vs. golden outputs, halt work and escalate to a human before proceeding.

## Project Overview

llformat is a focused Go source formatter that applies custom wrapping rules to log and printf-style statements. It reformats specific function calls while preserving the rest of the file content, using a hybrid approach combining Go's AST parser for targeted calls and a lightweight scanner for positioning.

## Commands

### Build
```bash
make build          # Build the llformat binary to bin/llformat
make all           # Same as build (default target)
```

### Testing
```bash
make test          # Run all unit tests
make unit          # Same as test
go test -v ./...   # Run tests with verbose output
```

### Usage
```bash
# Format and output to stdout
./bin/llformat <file.go>

# Format and write back to file
./bin/llformat -w <file.go>

# Additional options
./bin/llformat --col 100 <file.go>                    # Set column limit to 100
./bin/llformat --tab 4 <file.go>                      # Set tab width to 4
./bin/llformat --wrap-inline-comments <file.go>       # Hoist inline comments above for wrapping
```

### Clean
```bash
make clean         # Remove bin/ directory
```

## Architecture

The project follows a two-stage formatting approach:

### Core Components

1. **Main CLI** (`cmd/llformat/main.go`): Command-line interface that orchestrates the formatting pipeline
2. **CommentFormatter** (`formatter/comment_formatter.go`): Handles standalone comment block reformatting with greedy text reflow
3. **CompactCallFormatter** (`formatter/compact_call_formatter.go`): Handles targeted function call argument reformatting

### Formatting Pipeline

The formatter processes files in two sequential stages:
1. **Comment Formatting**: Reflows standalone comment blocks (preserving indentation and list formatting)
2. **Call Formatting**: Applies compact packing to specific function calls

### Target Functions

The formatter specifically targets these function calls:
- Log functions: `log.Infof`, `log.Debugf`, `log.Tracef`, `log.Errorf`, `log.Warnf`
- Format functions: `fmt.Printf`, `fmt.Sprintf`, `fmt.Errorf`

### Formatting Rules

- **80-column limit** (configurable with `--col`)
- **Compact packing**: Pack arguments from left to right without exceeding column limit
- **Text argument splitting**: String literals can be split across lines with `+` continuation
- **Non-text arguments**: Treated as atomic units, moved to next line if they don't fit

### Test Structure

Tests use golden file approach:
- `testdata/logs/`: Contains input/output pairs for call formatting
- `testdata/comments/`: Contains input/output pairs for comment formatting
- Test files: `input.go` → expected `output_next.go` comparison

### Key Design Decisions

- **Hybrid parsing**: Uses Go AST for call analysis but lightweight scanning for positioning to handle partially invalid files
- **Preserves surrounding code**: Only modifies targeted constructs, leaving rest of file intact
- **Robust text handling**: Properly handles string literal concatenation and preserves whitespace in splits
