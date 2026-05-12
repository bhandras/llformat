# TODO

## Retire Legacy Formatter Paths

Track and migrate the remaining legacy formatter paths now that logical chains
use the packed logical-chain formatter by default.

- Rename the logical-chain `"legacy"` style internally/docs to `"packed"` while
  keeping `"legacy"` accepted as a compatibility alias.
- Consider defaulting case clauses to `BreakCaseClauseLayoutAction`, with
  `BreakCaseClauseAction` retained only as fallback if needed.
- Consider defaulting arithmetic chains to the layout action for safe same-op
  chains, keeping `BreakAtOpAction` as fallback for mixed or unsupported binary
  expressions.
- Measure and remove or narrow `LegacyFuncSigFallbackRules` once native
  signature rules cover the remaining fallback cases.
- Remove unused historical rule bundles where they are not part of the public
  API, especially the `expressionRules` legacy-parity bundle.
- Keep comment formatting as a specialized text/prose formatter for now. It is
  the least likely piece to become normal AST DSL rules because it owns prose
  wrapping, directives, raw-string avoidance, and comment-block semantics rather
  than expression layout.
