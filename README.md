# llformat

`llformat` is a focused Go source formatter that reflows comments and applies
targeted, column-limit-aware formatting to:

- log/printf-style calls (including custom loggers like `rpcsLog.Infof(...)`)
- multiline (non-log) calls and method chains
- selected long expressions
- function signatures (including function literals and interface methods)
- blank-line hygiene rules that improve readability

It is intentionally **not** a general-purpose “pretty printer” that rewrites
the whole file.

Historically, it’s named `llformat` because it aims to implement the strict
readability rules described in Lightning Labs / LND development documentation
and other formatting conventions used across Lightning Labs projects:

- https://github.com/lightningnetwork/lnd/blob/master/docs/development_guidelines.md#code-spacing-and-formatting

## Goals and Non-Goals

Goals:

- **Targeted changes only**: only touch known formatting targets; preserve
  everything else.
- **Idempotent**: running `llformat` repeatedly should converge quickly.
- **Parse-safe**: output must remain valid Go (final output is normalized by
  `gofmt`).
- **Directive-safe comments**: never break `//go:` directives, `//nolint`, cgo
  pragmas, etc.

Non-goals:

- Replacing `gofmt`.
- Reflowing arbitrary code for style preferences (this repo prefers explicit
  golden fixtures for spec).

## How it Works (One Document)

The formatter runs a **pipeline of stages** over the file, then runs `gofmt`:

1. Comments (directive-safe reflow; optionally hoist inline comments)
2. Compact calls (log/printf/error string packing + string splitting)
3. Expressions (selected long-expression splits)
4. Multiline calls (non-log calls: pack args + layout selector chains)
5. Signatures (func decls, func literals, interface methods)
6. Blank lines (minimal readability rules)
7. `gofmt` normalization

Most of the pipeline is implemented via an internal **formatting DSL engine**
that:

- Applies one targeted rewrite at a time (deterministic ordering).
- Avoids rewriting spans that are “owned” by later stages when ownership
  boundaries are enabled (prevents stage fighting).
- Uses rewrite budgets and cycle detection for safety.

Where the AST printer would drop comments inside rewritten regions, rules are
conservative and often skip edits if inline comments are present.

For a detailed walkthrough of the pipeline and formatters, see `ARCHITECTURE.md`.

## CLI Usage

Build:

```bash
make build
```

Format to stdout:

```bash
./bin/llformat path/to/file.go
```

Write in-place:

```bash
./bin/llformat -w path/to/file.go
```

Helpful flags:

- `--col N`: column limit (default `80`)
- `--tab N`: tab stop width (default `8`)
- `--wrap-inline-comments`: hoist trailing inline comments above statements so
  they can be wrapped safely
- `--multiline-exclude a,b,c`: exclude function names from generic multiline
  call formatting
- `--logcalls-min-tail-len N`: avoid leaving tiny tails when splitting long
  format strings (0 means default)
- `--logcalls-selector-names n1,n2`: override the set of `*f` selector names to
  treat as printf/log calls for compact formatting (default includes `Infof`,
  `Errorf`, `Sprintf`, etc.)
- `--logcalls-selector-prefixes p1,p2`: restrict compact log/printf call
  formatting to specific selector receiver expression prefixes (e.g.
  `rpcSLog,zap.L().Sugar()`)
- `--fixpoint-iters N`: run the full pipeline repeatedly until stable (default
  is `3` in the CLI)
- `--print-plan`: print resolved stage plan and exit
- `--print-logcalls-patterns`: print the currently recognized log/printf call
  patterns and exit

## Tests

Unit tests:

```bash
make unit
```

### Golden fixtures (spec)

Golden fixtures are authoritative and live at:

- `testdata/*/input.go` → `testdata/*/output_next.go`

These fixtures define the intended behavior and are compared by tests. If a
formatter change appears to require golden updates, treat that as a spec change
and do it explicitly (ideally in a dedicated commit).

### Generating next goldens (for local experimentation)

The repository includes a helper for generating candidate `output_next.go`
files into a scratch directory (not committed):

```bash
go run ./tools/gen_next_goldens --out .next_goldens
```

## Code Layout

- `cmd/llformat/main.go`: CLI
- `formatter/`: pipeline + formatting stages
- `dsl/`: DSL engine and rules
- `testdata/`: golden fixtures

## Status

The repository is **next-only**: legacy modes and legacy goldens have been
removed.

## Disclaimer

This project was entirely **vibe coded** with the assistance of AI. While it
works well for its intended purpose, it is provided **as-is**, without warranty
of any kind, express or implied. The author assumes no responsibility for any
issues, bugs, or unintended behavior that may arise from using this software.
Use at your own risk.

See [LICENSE](LICENSE) for the full MIT license terms.
