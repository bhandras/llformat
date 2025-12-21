# TODO: Formatter Alignment Checklist

- [ ] Guard composites against duplication
  - [ ] Ensure `FormatCompositeLiteralArg` expansion is applied once per arg and never re-emitted downstream.
  - [ ] Keep slices/arrays inline unless inline width exceeds `columnLimit`; only expand when necessary.
- [ ] Harmonize string splitting
  - [ ] Prefer minimal splits (e.g., two-line split for long literals like “line exceed” → “limit”) while respecting width.
  - [ ] Keep following args on clean continuation lines after a split.
- [ ] Nested call handling
  - [ ] Inline nested call when it fits and lacks always-multiline composites/nested calls.
  - [ ] Otherwise treat the nested call as a single multiline arg; no sibling duplication or extra splitting.
- [ ] Width calculation & wrapping
  - [ ] Use comment-stripped, flattened text for width; keep chained-tail heuristic to avoid over-wrapping `).Foo()` tails.
  - [ ] Trigger wrap only when `indent + call` exceeds `columnLimit`.
- [ ] Layout and spacing
  - [ ] Head + `(` newline; args separated by `, ` or `,\n<indent>`; closing `)` aligned to call indent; trailing comma for multiline.
- [ ] Validation
  - [ ] Run `go test ./...` and diff `testdata/multiline/output_next.go` vs. formatted output to verify slice/map placement and string splits.
  - [ ] Keep goldens untouched (`testdata/**/output_next.go`).

---

# TODO: DSL Formatter Roadmap (Grand List)

## Principles / guardrails
- [x] Never modify golden fixtures under `testdata/**/output_next.go`.
- [x] Next-only pipeline: avoid reintroducing legacy/mode/profile selectors.
- [ ] Reduce long-term reliance on scan/string heuristics by converging on a single layout engine.

## Pipeline: DSLize all stages (without changing goldens)
- [x] DSL stage for comments (delegates to legacy), with directive preservation.
- [x] DSL stage for log/printf-style calls (delegates to legacy formatting).
- [x] DSL stage for multiline calls with selectable styles (`legacy|packed|packed-chain`).
- [x] DSL native blank-lines rules (with fallback for unparsable sources).
- [x] DSL native signatures rules with a style switch (`legacy|dsl`) and fallback.
- [x] Expression stage ownership boundaries (legacy hardening):
  - [x] Define a clear division of responsibility between “call formatting” and “expression formatting” via stage-owned spans.
  - [x] Ensure staged execution cannot fight/oscillate (strong idempotence guarantees) with parse-safe + ownership registry tests.

## Signature formatting (pure DSL style improvements)
- [x] Keep parse-safe: never insert a newline between `)` and the first return token.
- [x] Expand inline `struct{...}` / `interface{...}` in multiline signatures (opt-in).
- [x] Format multiline result lists and pack where possible (opt-in).
- [x] Format multiline param lists and pack where possible (opt-in).
- [x] Support generic type parameter lists in function declarations (opt-in).
- [ ] Add receiver-aware + generic-aware support for interface methods if needed (native DSL method stage).
- [ ] Add more coverage for edge cases: function-typed params, nested generics, constraints with `interface{}` and unions.

## Expression formatting (tight, stable, semantic)
- [ ] Strengthen precedence/associativity handling for line breaks (avoid any semantic ambiguity).
- [ ] Make call-argument formatting safe-by-design:
  - [x] `AutoDSLCallArgs` (allow only for known excluded calls).
  - [x] `layout-args` auto-enables DSL expr stage to avoid stage interference.
  - [x] Add a principled “never fight later call stages” mechanism for legacy stages (ownership registry, opt-in).
  - [ ] Extend the same ownership mechanism to DSL stages as needed (beyond `layout-args`).
- [ ] Improve formatting for mixed chains (selectors, indexing, slicing, calls) under a unified model.
  - [x] `layout-args` supports key/value, composites, indexing/slicing, generics, unary, parens, and type assertions in call-arg context.

## Testing and hardening (without touching goldens)
- [x] Add AST-equivalence property tests (ignore positions/scopes/Objs) for valid sources:
  - [x] Verify formatted output parses and is structurally equivalent to the original AST.
  - [x] Keep idempotence tests for DSL pipeline.
- [x] Add a large table-driven regression suite (~100 snippets) for `layout-args` that checks parseability + idempotence (no goldens).
- [ ] Expand crash/parse-failure coverage:
  - [x] DSL engine can still apply file rules even if `go/parser` fails.
  - [x] Add targeted tests for common “invalid go” fixtures patterns (multiple `package` blocks, etc.).
