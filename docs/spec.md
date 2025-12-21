# llformat formatting spec (behavioral)

This document describes llformat’s intended scope and invariants.
It is not a user manual; it is the contract tests and formatter code aim to
uphold.

## Core goals

- **Targeted changes only**: Reformat comments and a narrow set of
  printf/log-style calls, plus selected long expressions and signatures.
  Do not perform broad AST rewrites unrelated to these targets.
- **Idempotent**: Running llformat multiple times produces the same output.
- **Stable / deterministic**: Given the same input and config, output does not
  vary with iteration order or map ordering.
- **Preserve meaning**: Never change semantics; only change whitespace,
  punctuation used solely for formatting (e.g. trailing commas in multiline
  constructs), and string literal segmentation when it is semantics-preserving
  (concatenation of string constants).

## Global constraints

- **Golden fixtures are authoritative**: `testdata/**/output.go` is the source of
  truth for behavior; formatter changes must not “regenerate” these files.
- **gofmt runs once at the end of the pipeline**: llformat normalizes the final
  output with `gofmt` (via `go/format`), but internal passes should avoid
  repeatedly gofmting the whole file, because that can reformat unrelated code.
- **Inline comments are protected**: Many transformations skip when an inline
  `//` comment appears inside the rewritten span, because AST rendering drops
  comments inside expressions/argument lists.

## Pipeline stages (legacy and DSL)

llformat composes multiple focused formatters. The default CLI pipeline is:

1. **Comments**: Reflow and optionally move inline comments above statements.
2. **Compact calls**: Reformat targeted log/printf-style calls.
3. **Expressions**: Break long boolean/arithmetic chains and related constructs.
4. **Multiline calls**: Reformat selected call expressions into multiline form.
5. **Signatures**: Break long function and interface method signatures.
6. **Blank lines**: Insert blank lines before returns, between switch cases, and
   between interface methods.
7. **Final gofmt**: Normalize with `gofmt`.

### `UseDSLExpr` mode

When `UseDSLExpr` is enabled, the pipeline remains the same, but the **expressions**
stage is implemented by the DSL engine.

This is intentional:

- It provides incremental adoption (swap one stage).
- It avoids reformatting calls/signatures in the expression stage, preserving
  behavior parity with the legacy pipeline.

## Expression formatting rules (high level)

The expression stage is responsible for:

- **Keep simple comparisons atomic**: Expressions like `x > 0` must not be split
  across lines.
- **Break long logical chains**: For `&&`/`||` chains exceeding the column limit,
  insert line breaks at operators using a Go-style continuation indent.
- **Break long arithmetic chains**: For `+`, `-`, `*`, `/`, `%` chains exceeding
  the limit, insert line breaks at operators, but:
  - do not break string concatenation expressions; and
  - avoid breaking inside composite literals (to prevent surprising edits to
    struct literal field values).
- **Break long `case` clauses**: Split long `case` labels at commas.
- **Reflow long string concatenation**: If a string concatenation expression
  exceeds the line limit, it may be rewritten by flattening and re-splitting the
  constant string content into multiple quoted segments joined by `+`, using a
  continuation indent.

## DSL engine invariants

The DSL engine applies a list of rules over a parsed Go file:

- **One transforming rule per iteration**: Each iteration finds the first
  matching rule (by priority and node walk order), applies it, and repeats until
  no rules apply or a max-iteration limit is reached.
- **Deterministic rule ordering**: Rules are stably sorted by descending
  priority; ties preserve declaration order.
- **Atomic nodes**: Some rules mark nodes as “atomic” to prevent later breaking
  passes from splitting them.
- **Validated edits**: Prefer expressing changes as a list of non-overlapping
  byte-range edits applied to the original source.

## Configuration

Formatting is parameterized by:

- `--col` (`ColumnLimit`): maximum preferred line width for wrapping decisions.
- `--tab` (`TabStop`): visual width for `\t` when computing line lengths.
- `--wrap-inline-comments`: affects comment stage behavior (moving inline
  comments above).
- `--multiline-exclude`: comma-separated list of function names to exclude from
  multiline call formatting.
- `--logcalls-min-tail-len`: minimum tail length when splitting printf/logcall
  strings (0 => profile default).
- `--fixpoint-iters`: maximum pipeline iterations (0 => default).
- `--print-plan`: prints resolved pipeline plan and exits.
