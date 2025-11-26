# Call Formatting Rules (Packed Multiline)

This document formalizes the packed multiline call formatting rules used by the
left-flow formatter's fallback mode. The design favors deterministic, greedy,
width‑driven behavior with minimal heuristics.

## Scope

- Applies to non-target function/method calls when fallback mode is enabled.
- Targeted calls (logs/printf variants) use the existing left-flow rules.
- Formatting is performed on Go source, then gofmt’d (go/format) for stability.

## Column Model

- Column limit: default 80 (configurable).
- Tabs advance to tab stops (default 8).
- Visual width is computed byte-by-byte with rune width; newlines reset column.

## Indentation

- Continuation indent: `wsIndent + "\t"` (one tab beyond the line’s leading
  whitespace).
- Closing `)`: aligned with the line’s leading whitespace (`wsIndent`).
- Trailing comma: present on the last argument of multiline calls.

## Argument Classification

Each argument is classified for layout decisions:

- String literal (double-quoted; concatenations flattened).
- Composite literal:
  - Map/struct: always multiline when the containing call is multiline.
  - Slice/array: greedily packed (can remain inline if it fits; otherwise
    multiple per line with trailing commas).
- Call expression (possibly nested calls).
- Other expression (identifiers, numbers, selectors, etc.).

## Top-level Decision (Fit vs. Multiline)

For a call `Head(…args…)` at position with current visual column `cur`:

1) Compute the single-line width of the entire call text (unformatted). If it
   fits within the limit (including any separator before it), keep it single 
   line.
2) Otherwise, switch to multiline mode:
   - Emit `Head(` then newline.
   - Format arguments on continuation lines as described below.
   - Emit trailing comma, newline, then aligned `)`.

## Greedy Packing in Multiline Mode

Arguments are emitted left-to-right with greedy packing per continuation line.

- Separator between arguments on the same line: `", "`.
- If adding the next argument would exceed the limit, break the line before it
  (add `,` to end the previous line if needed) and continue on a fresh
  continuation line.

### Strings (Double-Quoted)

- Flatten concatenations of double-quoted literals.
- Split greedily at the last ASCII space that fits. If no space fits, cut by
  width (avoiding mid-rune splits). Continuation segments are emitted as `"…" +`
  on one line; next segment starts on the following continuation line.
- The final segment ends without `+`.

### Composite Literals

- Map/struct literals inside multiline calls are always formatted in block form:
  - Opening `{` on its line, then one `key: value,` per line, then closing `}`
    aligned to the continuation indent.
- Slice/array literals are greedily packed: multiple elements per line up to
  width, with trailing commas; closing `}` aligned to the continuation indent.

### Nested Call Expressions

- If the entire nested call fits on the current line (including `", "` when
  applicable) and contains no “always multiline” composites per the above, keep
  it inline on the current line.
- Otherwise, reformat the nested call in multiline mode recursively and place
  it starting on a fresh continuation line.
- Nested calls formatted in multiline mode do not add a trailing comma inside
  their own parentheses; the enclosing call’s argument separator governs.

## Chained Small Calls

- Short chained calls like `).Limit(100)` are kept inline on the same line when
  they fit, even if the preceding call was multiline; this avoids noisy wrapping
  of short suffixes.

## gofmt Normalization

- After rewriting, the buffer is passed through `go/format` so tooling and
  editors see consistent spacing and import organization.

## Determinism

- All decisions are made via width and structural classification only. No
  language-specific word heuristics are used. Greedy means “take as much as fits
  on the current line” according to the token’s printed form.

## Algorithm Sketch (Multiline Mode)

```
emit Head + "(\n"
line := contIndent
for each arg in args:
  a := classify(arg)
  if a is map/struct: a := blockFormatMapOrStruct(a)
  else if a is slice/array: a := greedyPackSlice(a)
  else if a is string: a := greedySplitString(a)
  else if a is call:
    if fitsOnCurrentLine(a) and !containsAlwaysMultiline(a): place inline
    else: a := multilineFormatCall(a) and start on fresh continuation line
  place a: if fits with ", ": append with ", ", else break line and start new
emit ",\n" + wsIndent + ")"
```

