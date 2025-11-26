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
  - [ ] Run `go test ./...` and diff `testdata/multiline/output.go` vs. formatted output to verify slice/map placement and string splits.
  - [ ] Keep goldens untouched (`testdata/**/output.go`).
