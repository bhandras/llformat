# Comment Strategy (Directive-Safe)

This document captures the current and intended long-term strategy for comment
formatting in `llformat`, with a special focus on directive preservation.

## Goals

- Reflow standalone comment blocks in a predictable way.
- Preserve the meaning of source code and keep output Go parseable.
- Never break tool directives that are encoded as comments (Go toolchain, cgo,
  linters, code generators).
- Keep default behavior parity unless explicitly opt-in.

## Current Approach (Default / Parity)

In the default configuration (`RuleProfile=parity`, `Mode=""`), comment
formatting remains **legacy-driven**:

- The `CommentFormatter` is treated as the **authoritative “oracle”** for comment
  reflow and comment safety rules.
- The DSL engine does not attempt to typeset comments; it focuses on AST-backed
  rewrites (calls/expressions/signatures/blank lines) with explicit ownership and
  non-interference.

This keeps the pipeline stable while the DSL engine matures, and it avoids
accidentally relying on AST printing for comment formatting (which can drop or
misplace comments embedded inside rewritten spans).

## Directive Preservation Policy

Some comments are not prose and must not be reflowed, normalized, or wrapped.
These comments are preserved verbatim.

### Line Directives (`//...`)

Line comments that look like directives (examples):

- `//go:build ...`, `// +build ...`
- `//go:generate ...`, `//go:embed ...`, `//go:linkname ...`
- `//go:noinline`, `//go:nosplit`
- Linter / tool directives such as `//nolint:...`, `//lint:ignore ...`,
  `//staticcheck:ignore ...`, `//gosec:ignore ...`, `//revive:disable:...`

These lines are preserved exactly, even if they exceed the column limit, because
wrapping can change semantics or break downstream tooling.

### Block Directives (`/* ... */`)

Standalone block comment blocks that appear to contain cgo/tool directives are
preserved verbatim, because reflowing can break cgo parsing.

Conservatively, we treat blocks containing any of the following prefixes as
directive-like and skip reflow:

- `#cgo`
- `#include`
- `#pragma`

## Long-Term Strategy

### P1 (Opt-In): Minimal Comment DSL

If we move comment logic into the DSL, the preferred approach is **minimal and
constraint-focused**:

- Express “do no harm” policies (directive preservation, no reflow in blocks
  that match directive patterns, no transformation when inline comments are
  embedded in a rewritten span).
- Keep the actual reflow algorithm delegated to the legacy comment formatter,
  at least initially.

This allows comment behavior to be configured and staged (like other DSL stages)
without requiring a full typesetting engine.

### P2 (Risky): Full Comment Typesetting Engine

A full comment layout/typesetting engine is explicitly considered risky and is
only justified if there is a proven need that cannot be met by:

- directive preservation + conservative reflow, and/or
- targeted opt-in rules for narrow comment families.

