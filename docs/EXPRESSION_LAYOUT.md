# Expression Layout (Opt-In)

This document describes llformat’s **opt-in** expression formatting behavior in
the `next` pipeline. The goal is to provide **predictable, parse-safe
splitting** of selected long expressions while preserving llformat’s core rule:

- **Only touch targeted regions**; avoid reformatting unrelated code.

Important constraints:

- Golden fixtures under `testdata/**/output.go` are authoritative. Default
  behavior must remain stable.
- Expression formatting is intentionally conservative around:
  - call argument lists (unless explicitly enabled)
  - composite literals / func literals
  - inline comments inside expressions

## Design Goals

1. **Correctness first**: formatted output must parse, and should be idempotent.
2. **Locality**: rewrite only the smallest span needed to achieve a readable
   wrap; avoid whole-file rewrites.
3. **Determinism**: repeated runs should converge; rule application order should
   be stable.
4. **Non-interference**: expression rules should not fight call formatting.

## Call Argument Policy

By default, the DSL expression stage does **not** rewrite inside call argument
expressions.

This is controlled by the `CallArgsPolicy` used by `dsl.ExprEditSafeCond`:

- `CallArgsPolicyOff` (default): never edit inside call args.
- `CallArgsPolicyAuto`: allow edits only inside arguments of calls whose callee
  is on an allowlist (intended to match “calls excluded from multiline call
  formatting” so stages do not fight).
- `CallArgsPolicyForce`: allow edits inside call args (highest risk; intended
  only for controlled experiments).

Pipeline knobs:

- `PipelineConfig.AutoDSLCallArgs`: enables `CallArgsPolicyAuto`.
- `PipelineConfig.AllowDSLCallArgs`: enables `CallArgsPolicyForce`.

## Major Expression Families

This section documents the current layout intent for the most important
expression kinds. These behaviors are controlled via the style toggles:

- `DSLExprLogicalStyle`
- `DSLExprArithmeticStyle`
- `DSLExprSelectorChainStyle`
- `DSLExprCaseClauseStyle`

### Logical chains (`&&` / `||`)

Goal: split long logical chains at operators into a readable vertical form,
preferring stable indentation and avoiding “close paren on its own line” hazards
in parenthesized contexts.

Typical shape:

- break after operator tokens
- indent continuation lines under the expression’s indentation

### Arithmetic chains (`+` / `-` / `*` / `/` / `%`)

Goal: split long arithmetic chains without breaking precedence or removing
explicit parentheses. Chains are treated as sequences of terms joined by the
same operator.

Typical shape:

- keep explicit `ParenExpr` boundaries intact
- break between terms

### Selector / method chains

Goal: for long selector chains and method chains, prefer a “leading dot” style
to avoid semicolon insertion hazards:

```
client.
    WithTimeout(...).
    Execute(...)
```

In call-argument contexts, method chain segments may also format their argument
lists using the layout engine (still conservatively: comments and existing
multiline spans are skipped).

### `switch` / `case` clause lists

Goal: split long `case A, B, C:` lists into a consistent multiline list, while
preserving parseability and avoiding comment loss.

### String concatenation

Goal: when safe, reflow long `"a" + "b" + ...` concatenations into a stable
multiline form that stays within the configured column limit.

Note: raw string literals and comment-containing spans are intentionally skipped.

## Known Limitations (Current)

- Expression layout is intentionally conservative in the presence of comments
  inside the span to be rewritten (AST printing does not preserve those comments).
- Composite literals and func literals are treated as “owned elsewhere” and are
  skipped by expression rewrites.
- Call argument rewriting is opt-in and must be coordinated with call formatting
  to avoid oscillation; the ownership registry and/or call-arg allowlists are the
  recommended mechanisms.
