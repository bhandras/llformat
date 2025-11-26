# Formatter Rule Book (Reusable Building Blocks)

This document lists the modular rules and heuristics we need to assemble the desired formatter behavior. Treat each section as a reusable “lego block” when implementing formatting passes.

## Detection & Triggering
- **Call detection**: Identify call expressions by `ident`/selector followed by `(`; skip function/method definitions and keywords.
- **Exclusions**: Skip targeted log/printf calls handled elsewhere; respect user `Excludes`.
- **Wrap trigger**: Compute visual width using flattened (comment-stripped) text; wrap when `indent + call` exceeds `columnLimit`, except short chained tails (e.g., `).Limit(100)` heuristics).

## Layout Skeleton (Packed Multiline)
- Head + `(`, newline; closing `)` aligned to call indent.
- One trailing comma for multiline calls.
- Continuation indent: caller whitespace + single tab.
- Each argument is emitted once; never duplicate args when formatting.

## Arguments: Core Blocks
- **Strings**:
  - If the quoted form fits after `, `, keep inline.
  - Otherwise split at word boundaries (fallback hard cut) using `buildSplitQuoted`; prefer minimal splits (often two-line split suffices).
- **Expressions**:
  - Greedy inline if first-line width fits; otherwise break before the arg and place at continuation indent.
- **Composites**:
  - Maps/structs: one entry per line with trailing commas.
  - Slices/arrays: keep inline unless inline width exceeds limit, then pack greedily with trailing comma.
  - Never emit the same composite twice.
- **Nested calls**:
  - Inline if entire nested call fits and no always-multiline composites/nested calls inside.
  - Otherwise format nested call with packed multiline as a single argument; no extra splitting of siblings.

## Spacing & Grouping
- Arguments separated by `, ` when staying on the same line.
- When breaking, insert `,\n` then continuation indent.
- When an arg spans multiple lines, the next arg starts on a fresh continuation line (unless it fits after the current line and rules allow inline).

## Safety & Normalization
- Skip strings/comments during scanning to avoid mangling literals.
- After rewrite, run `format.Source` (gofmt) to ensure syntactic correctness and standard Go spacing.

## Utilities (assumed available)
- `visualLen`, `advanceCols`, tab-aware width.
- `stripComments` for width estimation.
- `scanBalancedParen`, `splitTopLevelAny` for safe argument splitting.
- Composite helpers: `FormatCompositeLiteralArg` (keyed maps/structs), `findTopLevelBraces`, `callHasAlwaysMultilineComposite`.

Use these rules as the reference when adjusting the formatter; do not modify goldens (`testdata/**/output.go`). Each adjustment should map back to one of these blocks.***
