# llformat

`llformat` is a focused Go source formatter for controlled adoption in large
Go repositories. It does not try to become a general-purpose pretty printer.
Instead, it applies a small set of column-limit-aware rewrites that are hard to
get consistently from `gofmt` alone:

- conservative comment reflow
- log/printf-style call formatting
- structured logging key/value pair packing
- multiline non-log call and method-chain formatting
- selected long-expression breaks
- function, function-literal, and interface-method signature formatting
- vertical spacing rules around signatures, returns, cases, and control blocks

The project name comes from the Lightning Labs / LND readability conventions
that originally motivated the formatter:

- https://github.com/lightningnetwork/lnd/blob/master/docs/development_guidelines.md#code-spacing-and-formatting

## Goals

- **Targeted changes only**: touch known formatting targets and preserve the
  rest of the file.
- **Idempotent output**: repeated runs should converge quickly.
- **Parse-safe rewrites**: generated output must remain valid Go and is
  normalized with `gofmt`.
- **Conservative comments**: preserve comments that already fit, preserve
  preformatted blocks, and avoid directives such as `//go:`, cgo pragmas, and
  `//nolint`.
- **Adoptable at repo scale**: provide diagnostics for finding formatter gaps
  before introducing a large baseline commit.

## Non-Goals

- Replacing `gofmt`.
- Formatting generated files by default.
- Guaranteeing every line fits under the column limit.
- Rewriting arbitrary code for subjective style preferences.
- Competing with unrelated whitespace linters that cannot express the same
  policy. In target repos, let one formatter own vertical spacing.

## Quick Start

Build the formatter:

```bash
make build
```

Format one file to stdout:

```bash
./bin/llformat path/to/file.go
```

Write one file in place:

```bash
./bin/llformat -w path/to/file.go
```

Format a repository recursively while skipping common generated files:

```bash
find . -name '*.go' \
  ! -name '*.pb.go' \
  ! -name '*.pb.gw.go' \
  ! -name '*.sql.go' \
  ! -path './db/sqlc/*' \
  -print0 |
  xargs -0 -n 1 /path/to/llformat/bin/llformat -w
```

Most adopting repositories should wrap this in their own `make fmt` target so
the generated-file excludes match the local codebase.

## Default Style

The CLI default is the current "next" profile. Important defaults include:

- column limit `80`, tab stop `8`
- comments mode `overflow`, which only wraps overflowing prose comments
- fitting comment blocks are preserved as-is
- Go example `// Output:` and `// Unordered output:` blocks are preserved
- trailing inline comments are not hoisted unless `--wrap-inline-comments` is
  set
- log/printf calls are formatted with compact string splitting
- structured logging calls keep the message/preamble compact, then pack
  key/value arguments in pairs
- multiline function signatures keep a blank separator before the body
- collapsed single-line signatures do not keep an extra body separator
- single-return control blocks stay compact
- multiline control blocks with multiple statements keep a readability
  separator

These examples show the subset of Lightning Labs-style formatting that
`llformat` owns. They are not a complete Go style guide. The examples use
neutral names and mirror shapes covered by focused tests or golden fixtures.

### Comments

Fitting comments are preserved:

Before:

```go
// LoadStore opens the store and verifies the schema version.
func LoadStore(path string) (*Store, error) {
	return openStore(path)
}
```

After:

```go
// LoadStore opens the store and verifies the schema version.
func LoadStore(path string) (*Store, error) {
	return openStore(path)
}
```

Overflowing prose comments are wrapped:

Before:

```go
// LoadStore opens the store, verifies the schema version, and prepares the background cache that is used by later request handlers.
func LoadStore(path string) (*Store, error) {
	return openStore(path)
}
```

After:

```go
// LoadStore opens the store, verifies the schema version, and prepares the
// background cache that is used by later request handlers.
func LoadStore(path string) (*Store, error) {
	return openStore(path)
}
```

Preformatted output blocks are preserved:

Before:

```go
func ExampleRouter() {
	fmt.Println("ready")

	// Output:
	// ready
}
```

After:

```go
func ExampleRouter() {
	fmt.Println("ready")

	// Output:
	// ready
}
```

### Log and Error Calls

Long printf-style messages are split while keeping the call compact:

Before:

```go
func loadSession(log Logger, sessionID string, attempt int, err error) error {
	return fmt.Errorf("unable to load session %s on attempt %d from the primary store: %w", sessionID, attempt, err)
}
```

After:

```go
func loadSession(log Logger, sessionID string, attempt int, err error) error {
	return fmt.Errorf("unable to load session %s on attempt %d from "+
		"the primary store: %w", sessionID, attempt, err)
}
```

### Structured Logging

Structured log calls keep the message/preamble compact, then pack key/value
pairs:

Before:

```go
func f(log Logger, sessionID string, count int, retry bool, reason string) {
	log.InfoS("processed session with updated state", "session_id", sessionID, "count", count, "retry", retry, "reason", reason)
}
```

After:

```go
func f(log Logger, sessionID string, count int, retry bool, reason string) {
	log.InfoS("processed session with updated state",
		"session_id", sessionID, "count", count, "retry", retry,
		"reason", reason,
	)
}
```

Error-style structured logs keep the error and message together:

Before:

```go
func f(log Logger, err error, sessionID string, attempt int) {
	log.ErrorS(err, "failed to process session", "session_id", sessionID, "attempt", attempt)
}
```

After:

```go
func f(log Logger, err error, sessionID string, attempt int) {
	log.ErrorS(err, "failed to process session",
		"session_id", sessionID, "attempt", attempt,
	)
}
```

### Function Signatures

Multiline signatures keep a separator:

Before:

```go
func processBundle(store Store, bundle PackageBundle, policy ValidationPolicy, clock Clock) error {
	return store.Save(bundle, policy, clock)
}
```

After:

```go
func processBundle(store Store, bundle PackageBundle, policy ValidationPolicy,
	clock Clock) error {

	return store.Save(bundle, policy, clock)
}
```

Collapsed signatures stay compact:

Before:

```go
func processBundle(
	store Store,
	bundle PackageBundle) error {

	return store.Save(bundle)
}
```

After:

```go
func processBundle(store Store, bundle PackageBundle) error {
	return store.Save(bundle)
}
```

Small return lists stay inline when they fit:

Before:

```go
func loadBundle(store Store, id BundleID) (
	*PackageBundle,
	error) {

	return store.Load(id)
}
```

After:

```go
func loadBundle(store Store, id BundleID) (*PackageBundle, error) {
	return store.Load(id)
}
```

### Function Literals

Multiline function literal signatures also keep a body separator:

Before:

```go
func f() {
	requestParserForInitialization := func(req *OpenRequest) (*InitMessage, error) {
		_ = req
		return nil, nil
	}

	_ = requestParserForInitialization
}
```

After:

```go
func f() {
	requestParserForInitialization := func(req *OpenRequest) (*InitMessage,
		error) {

		_ = req

		return nil, nil
	}

	_ = requestParserForInitialization
}
```

Already-multiline closure signatures keep their separator:

Before:

```go
func f() {
	alreadyFormatted := func(
		first SomeRidiculouslyLongParameterTypeNameThatForcesLineBreakUnder80Columns,
		second AnotherRidiculouslyLongParameterTypeNameThatAlsoForcesLineBreak) {
		_ = first
		_ = second
	}

	_ = alreadyFormatted
}
```

After:

```go
func f() {
	alreadyFormatted := func(
		first SomeRidiculouslyLongParameterTypeNameThatForcesLineBreakUnder80Columns,
		second AnotherRidiculouslyLongParameterTypeNameThatAlsoForcesLineBreak) {

		_ = first
		_ = second
	}

	_ = alreadyFormatted
}
```

### Single-Return Control Blocks

Single-return control blocks stay tight:

Before:

```go
if missingConfig &&
	allowDefault {

	return defaultConfig(), nil
}
```

After:

```go
if missingConfig &&
	allowDefault {
	return defaultConfig(), nil
}
```

Blocks with additional work keep a separator after a multiline header:

Before:

```go
if missingConfig &&
	allowDefault {
	log.Debug("using default config")
	return defaultConfig(), nil
}
```

After:

```go
if missingConfig &&
	allowDefault {

	log.Debug("using default config")

	return defaultConfig(), nil
}
```

### Calls and Chains

Long calls are packed instead of forced into one-argument-per-line layout:

Before:

```go
result := buildPackage(sessionID, requestID, previousState, nextState, retryPolicy, clock)
```

After:

```go
result := buildPackage(
	sessionID, requestID, previousState, nextState, retryPolicy,
	clock,
)
```

Method chains break at selector boundaries:

Before:

```go
return client.Session(sessionID).WithPolicy(policy).WithClock(clock).Load(ctx)
```

After:

```go
return client.
	Session(sessionID).
	WithPolicy(policy).
	WithClock(clock).
	Load(ctx)
```

### Intentional No-Ops

Directive comments and unsafe-to-reflow regions are preserved:

Before:

```go
//go:generate go run ./internal/tool
func generatedHook() {}
```

After:

```go
//go:generate go run ./internal/tool
func generatedHook() {}
```

Comment-heavy expressions may be skipped rather than rewritten:

Before:

```go
value := computeValue(
	input, // keep attached to input
	options,
)
```

After:

```go
value := computeValue(
	input, // keep attached to input
	options,
)
```

## CLI Flags

```text
llformat [-w] [--wrap-inline-comments] [--comments MODE] [--col N] [--tab N] [--multiline-exclude FUNCS] [--logcalls-min-tail-len N] [--logcalls-selector-names NAMES] [--logcalls-selector-prefixes PREFIXES] [--fixpoint-iters N] <path>
llformat --print-plan
llformat --print-logcalls-patterns
```

Useful flags:

- `-w`, `--write`: write the formatted result back to the source file
- `--col N`: column limit, default `80`
- `--tab N`: tab stop width, default `8`
- `--comments MODE`: `overflow`, `prose`, or `off`, default `overflow`
- `--wrap-inline-comments`: hoist trailing inline comments above statements so
  they can be wrapped safely
- `--multiline-exclude a,b,c`: exclude function-name substrings from generic
  multiline call formatting
- `--logcalls-min-tail-len N`: avoid leaving tiny tails when splitting long
  printf/log strings
- `--logcalls-selector-names n1,n2`: override selector or identifier names to
  treat as printf/log calls
- `--logcalls-selector-prefixes p1,p2`: restrict compact log/printf formatting
  to matching receiver expression prefixes
- `--fixpoint-iters N`: repeat the full pipeline until stable, default `3`
- `--print-plan`: print the resolved stage plan and exit
- `--print-logcalls-patterns`: print the active log/printf matching patterns
  and exit
- `--trace-dsl`: trace applied DSL edits to stderr
- `--trace-dsl-reasons`: trace DSL skip/apply reasons to stderr

## Adoption Workflow

For a large repository, avoid starting with a blind formatting commit. A safer
loop is:

1. Build `llformat`.
2. Run corpus diagnostics against the target repo.
3. Inspect overflows, AST diffs, non-idempotence, and changed-line clusters.
4. Convert clear failures into small formatter rules with neutral regression
   tests.
5. Re-run diagnostics and compare reports.
6. Once the remaining output is acceptable, introduce one mechanical baseline
   commit in the target repo.
7. Put formatter invocation and generated-file excludes behind the target
   repo's normal `make fmt` / `make fmt-check` workflow.

This loop is intentionally designed to keep target-repo source out of
formatter tests and reports. Regression tests in this repo should use neutral,
synthetic examples.

## Corpus Diagnostics

The corpus checker formats one or more repositories and writes redacted
Markdown/JSON reports:

```bash
go run ./tools/corpus_check -repo /path/to/repo -out .corpus_reports/latest
```

The default `adoption` profile keeps normal safety excludes and skips common
generated-file suffixes so reports focus on hand-maintained source. Use
`-profile all` only when you intentionally want a broader scan.

The diagnostics are useful for:

- finding new or moved long lines
- identifying formatter-introduced overflows
- spotting AST changes
- finding non-idempotent formatting loops
- ranking formatter bugs by frequency and review impact

## How It Works

The formatter runs a pipeline of targeted stages, then runs `gofmt`:

1. Comments
2. Compact log/printf and string calls
3. Selected expression rewrites
4. Multiline non-log calls and method chains
5. Function signatures
6. Blank-line rules
7. `gofmt` normalization

Most stages are implemented through an internal formatting DSL engine. The DSL
applies deterministic AST-selected rewrites, tracks stage ownership boundaries,
uses rewrite budgets, and detects cycles to avoid stage fighting.

Rules are conservative around comments. If rewriting a span would risk dropping
or moving comments incorrectly, the formatter usually skips the edit.

For a deeper walkthrough, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Tests

Run unit tests:

```bash
make unit
```

Run the full local check used before formatter commits:

```bash
make self-check
make lint
```

### Golden Fixtures

Golden fixtures are authoritative and live at:

- `testdata/*/input.go` -> `testdata/*/output_next.go`

Do not rewrite these fixtures as part of ordinary formatter work. If a behavior
change appears to require golden updates, treat that as a spec change and get
explicit maintainer direction first.

### Candidate Goldens

For local experiments, generate candidate next outputs into a scratch
directory:

```bash
make gen-next-goldens
```

This writes to `.next_goldens/` and is not a substitute for reviewing or
maintaining the authoritative golden fixtures.

## Code Layout

- `cmd/llformat/main.go`: CLI
- `formatter/`: pipeline and formatting stages
- `dsl/`: DSL engine, AST conditions, and rewrite actions
- `tools/corpus_check/`: adoption diagnostics
- `tools/overflow_report/`: focused overflow reporting
- `testdata/`: authoritative golden fixtures

## Status

The repository is next-only: legacy modes and legacy goldens have been removed.
Behavior is specified through focused unit tests, corpus-derived regression
tests, and golden fixtures.

The formatter is intended for controlled rollout. Run it in CI, inspect the
diffs, and keep target-repo lint rules aligned with the formatter's ownership
of spacing.

## Disclaimer

This project was built with substantial AI assistance. It is provided as-is,
without warranty of any kind, express or implied. The author assumes no
responsibility for issues, bugs, or unintended behavior that may arise from
using this software. Use at your own risk.

See [LICENSE](LICENSE) for the full MIT license terms.
