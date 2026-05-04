# llformat Architecture

This document describes llformat’s architecture, the formatting pipeline, and
the responsibilities of each formatter/stage.

The project is intentionally **next-only**: there is a single pipeline and a
single golden spec (`testdata/*/output_next.go`).

Historically, the name “llformat” comes from Lightning Labs: the formatter is
aimed at making it easy to apply the strict readability rules documented for
Lightning Labs / LND codebases, plus other conventions that emerged across the
ecosystem:

- https://github.com/lightningnetwork/lnd/blob/master/docs/development_guidelines.md#code-spacing-and-formatting

## High-level overview

At a high level, llformat is:

- A **pipeline** of targeted formatters (stages).
- A **DSL engine** used by most stages to apply small, deterministic rewrites to
  Go AST nodes.
- A final normalization step using `gofmt` (`go/format`).

llformat’s core design constraint is **locality**: it does not attempt to
reformat an entire file in a single pretty-print pass. Instead, it rewrites only
regions that are:

- explicitly targeted by the stage’s rule set, and
- safe to rewrite (parse-safe, directive-safe, and comment-safe within the
  limitations of Go AST printing).

## Repository layout

- `cmd/llformat/main.go`: CLI entrypoint.
- `formatter/`: pipeline orchestration + stage implementations + formatting
  helpers.
- `dsl/`: DSL engine (rule evaluation and edit application) and reusable DSL
  rules/conditions/actions.
- `internal/compat/`: compatibility wrappers for legacy-style implementations
  that remain authoritative (currently comment reflow).
- `testdata/*/input.go` / `testdata/*/output_next.go`: golden fixtures used by
  tests.
- `tools/gen_next_goldens/`: helper to generate candidate `output_next.go` files
  into a scratch directory.

## Pipeline

The pipeline is implemented in `formatter/pipeline.go` and is constructed by
`formatter/NewPipeline`.

Stages are created by:

- `formatter/DefaultStagesWithOptions` (`formatter/stage.go`)
- stage builder helpers in `formatter/stage_builders.go`
- rule bundle selection in `formatter/dsl_bundle.go` and
  `formatter/dsl_bundles_next.go`

### Stage order

The default stage order is:

1. `comments`
2. `compact-calls`
3. `expressions`
4. `multiline-calls`
5. `signatures`
6. `blank-lines`
7. final `gofmt`

This ordering is intentional:

- comments run early so later stages see stable indentation and avoid rewriting
  spans that contain directives/comments.
- `compact-calls` runs before generic multiline call formatting so that log/printf
  formatting is handled by the specialized call rules, not the generic call
  packer.
- `expressions` runs before multiline calls to normalize standalone long
  expressions, but must avoid fighting the call-formatting stages.
- `signatures` and `blank-lines` run after call formatting because call stages
  can introduce multiline constructs that then need signature/blank-line hygiene.

### Fixpoint iterations (CLI)

The CLI defaults to a small fixpoint search (`--fixpoint-iters`, default 3).
This matters because:

- Some rewrites become possible only after earlier rewrites introduce stable
  indentation or adjust wrapping decisions.
- Running a small number of full passes is closer to the way users run formatters
  (“format until stable”) without requiring manual re-runs.

The library `NewPipeline` still supports single-pass operation by setting
`MaxPipelineIterations=1`.

## DSL engine

The DSL engine is responsible for:

- parsing the input source to a Go AST when possible,
- walking candidate nodes in a deterministic order,
- evaluating a list of rules (pattern + condition),
- applying edits to the original source as byte-range patches.

Key properties:

- **Deterministic selection**: stable rule ordering plus a stable node walk order.
- **One rewrite per iteration**: a stage can run multiple iterations, but each
  iteration applies at most one transformation. This prevents overlapping edits
  from becoming hard to reason about.
- **Budgets and cycle detection**: stages can set rewrite budgets (max output
  growth, etc.) and detect cycles for safety.

Important limitation:

- Go’s AST printer does not preserve comments inside rewritten subexpressions
  well. As a result, many rewrite rules are conservative and will skip nodes if
  there are inline comments inside the candidate span.

## Ownership boundaries (preventing stage fighting)

Some stages “own” specific spans of the file (notably call spans and argument
lists). Ownership is used to prevent earlier stages from rewriting inside a span
that a later stage will reformat.

Mechanism:

- Stages that can compute owned spans implement an ownership interface and
  provide an `OwnedSpansFunc` over the current source snapshot.
- Earlier stages consult the ownership registry and skip DSL edits that overlap
  owned spans.

This helps avoid oscillations like:

1. expression stage breaks inside call args
2. call stage repacks args
3. expression stage breaks again, etc.

## Stage-by-stage details

### 1) Comments stage (`comments`)

Goal:

- Reflow standalone comment blocks while preserving meaning. The CLI default is
  conservative `overflow` mode: preserve comment blocks that already fit the
  column limit, and preserve blocks that look preformatted.
- Preserve tool directives embedded in comments.
- Optionally hoist inline trailing comments above statements so they can be
  wrapped safely (`--wrap-inline-comments`).

Implementation:

- The rule selection is DSL-driven, but the reflow algorithm is delegated to the
  compatibility comment formatter (`internal/compat`) because it remains the
  authoritative implementation for directive preservation.

Key safety rules:

- Do not reflow directive-like comments such as:
  - `//go:build`, `//go:generate`, `//go:embed`, `// +build`
  - `//nolint`, `//lint:ignore`, `//staticcheck:ignore`, etc.
  - cgo pragmas (`/* #cgo ... */`, `/* #include ... */`)
- In `overflow` mode, do not reflow already-fitting blocks, numbered lists,
  fenced examples, table-like comments, URL-heavy comments, or comments with
  alignment spacing.

### 2) Compact calls stage (`compact-calls`)

Goal:

- Format log/printf-style calls (including custom loggers where only the method
  suffix is meaningful, e.g. `rpcsLog.Infof`).
- Pack arguments greedily within the column limit.
- Split long format strings safely with a configurable minimum “tail” length
  to avoid ugly 1–2 character remnants.
  - Selection can be restricted to an allowlist of selector receiver prefixes
    via `--logcalls-selector-prefixes` (useful when a repo has many `Infof`-like
    methods that should not be rewritten).
  - The set of recognized `*f` selector names can be overridden via
    `--logcalls-selector-names`.

Implementation:

- DSL rules match log/printf/error constructors and call into call-formatting
  helpers in `formatter/compact_call_formatter.go`.

Key behaviors:

- Prefer staying single-line if it fits.
- If multiline is needed:
  - `FuncName(` on the first line
  - args packed across subsequent lines
  - `)` aligned to call indentation
- When splitting string literals:
  - maintain semantics (string concatenation of constants)
  - keep `+` placement gofmt-friendly
  - avoid leaving tiny tails via `--logcalls-min-tail-len`

### 3) Expressions stage (`expressions`)

Goal:

- Apply targeted long-expression splitting for readability while preserving
  semantics and minimizing collateral formatting.

Typical targets:

- long logical chains (`&&`/`||`)
- long arithmetic chains
- long selector chains (often using “leading dot” layout)
- long `case A, B, C:` lists
- safe string literal splitting in certain contexts

Safety constraints:

- By default, expression rules avoid editing inside call arguments unless
  explicitly allowed, to avoid interference with call stages.
- Rules typically skip spans containing inline comments.

### 4) Multiline calls stage (`multiline-calls`)

Goal:

- Format non-log calls and method chains when they exceed the column limit:
  - keep single-line calls as-is when they fit
  - otherwise, emit a predictable multiline packed layout

Behavior:

- First line: callee + `(`
- Middle lines: args packed tightly across lines (greedy)
- Last line: `)` aligned with call indent

It also supports “layout” styles for method chains and/or args, depending on
configured stage options.

### 5) Signatures stage (`signatures`)

Goal:

- Format function signatures consistently across:
  - function declarations
  - function literals (`func(...) ... {}`)
  - interface method declarations

Behavioral targets:

- If it fits on one line, keep it on one line.
- If not:
  - break parameters across lines, preserving grouping where possible
  - pack return types where possible
  - avoid placing `)` and the first return token on different lines unless the
    return list itself is multiline

### 6) Blank lines stage (`blank-lines`)

Goal:

- Apply a small set of readability rules that insert blank lines in places that
  improve scanning, without turning llformat into a stylistic whitespace tool.

Examples:

- Insert a blank line after a multiline control statement header (e.g. multiline
  `if (...)` condition) before the first statement in the body.
- Insert a blank line above comments that immediately precede `case` or `return`
  so comments stay attached to the following statement.

## Golden spec and tests

Golden fixtures are authoritative:

- `testdata/*/input.go` → `testdata/*/output_next.go`

Tests compare pipeline output exactly to these fixtures.

For non-golden coverage (to avoid over-constraining formatting), the test suite
also includes property/regression tests that check:

- parseability of the output
- idempotence (or convergence within a small number of passes)
- AST equivalence for valid Go inputs (ignoring positions and comments)
