# llformat Architecture Overhaul Plan

This file tracks the long-term “DSL-first formatter engine” overhaul work as a
living checklist. It is intended to be updated as work lands.

Constraints:
- Do **not** modify golden fixtures under `testdata/**/output.go`.
- Keep default behavior parity (`RuleProfile=parity`, `Mode=""`) unless changes
  are explicitly opt-in.
- Keep `make unit` green on each merged chunk.

Legend:
- **P0/P1/P2**: priority (P0 highest)
- **Safe**: expected not to change default outputs
- **Opt-In**: behavior changes only under explicit mode/profile/policy
- **Risky**: likely to affect outputs if enabled accidentally

---

## 0) Status Snapshot

- [x] DSL rule bundles extracted (`formatter/dsl_bundles.go`)
- [x] `RuleProfile` introduced and threaded through pipeline/stages
- [x] `DSLBundle` stage specs introduced (`formatter/dsl_bundle.go`)
- [x] Stage construction factored into builders (`formatter/stage_builders.go`)
- [x] `StagePlan` introduced (`formatter/stage_plan.go`)
- [x] Pipeline derives `StagePlan` from `Mode`/`DSLCallPolicy` (with tests)

---

## A) Configuration + Taxonomy (StagePlan / Bundle / Profile)

- [x] **P0 Safe** Add `PipelineConfig.StagePlanOverride *StagePlan` (explicit stage selection without per-stage booleans).
- [x] **P0 Safe** Add `RuleProfile -> default StagePlan` mapping (so `RuleProfile` becomes a first-class stage selector).
- [x] **P1 Safe** Add `RuleProfile -> default DSLBundle` factory object (bundle selection as a single testable function).
- [x] **P1 Safe** Move all DSL stage engine defaults into bundles (node order, max iters, shim flags).
- [x] **P1 Safe** Add `--print-plan` output (resolved `RuleProfile`, `StagePlan`, and key style knobs).
- [ ] **P2 Safe** Config validation (unknown profile/style produces clear errors or explicit fallback).

---

## B) Ownership / Non-Interference (Stage Contracts)

- [x] **P0 Safe** Formalize ownership boundaries as a policy object (owned spans per stage).
- [x] **P0 Safe** Add generic “skip edits if overlaps owned spans” support in DSL engine.
- [x] **P1 Safe** Trace reasons when rules are blocked by ownership.
- [x] **P1 Opt-In** Add tests that prove call-arg layout ownership prevents expr-stage fighting.

---

## C) DSL Engine Core (Determinism, Debuggability, Invariants)

- [x] **P0 Safe** Deterministic rewrite ordering tests (preorder vs deepest-first).
- [x] **P0 Safe** Stage-level invariants tests (parseable->parseable under parse-safe, idempotence).
- [x] **P1 Safe** Rewrite budget/guardrails (iteration/bytes/edit limits) for safety in `next`.
- [x] **P1 Safe** Improve trace output formatting (stable stage+rule+node+reason summaries).
- [x] **P2 Safe** Minimal reproduction harness for debugging rule interactions (non-golden).

---

## D) Call Formatting DSL-ization (Calls / Chains / Args)

- [x] **P0 Opt-In** Consolidate call policy as explicit bundle/profile config (packed/layout/all).
- [x] **P1 Opt-In** Expand layout-args coverage (nested calls, composites, generics, index/slice/type assertions).
- [x] **P1 Opt-In** Enforce “no inline-comment arg rewrites” consistently (rule-level guard).
- [x] **P2 Opt-In** Optional arg grouping heuristics as explicit DSL constructs.

---

## E) Expression Formatting (“A” Goal: Tight, Correct Splitting)

- [x] **P0 Opt-In** Build expression layout docs for major expression families (logical/arithmetic/selector/case/string).
- [x] **P0 Safe** Preserve default “no call-arg edits” unless explicitly enabled/allowlisted.
- [x] **P1 Opt-In** Improve call-arg expression formatting only for excluded callees (auto allowlist).
- [x] **P1 Opt-In** Paren-aware breaking improvements and close-paren placement stability.
- [x] **P2 Opt-In** Extend coverage for remaining expression kinds (as needed).

---

## F) Signatures DSL-ization

- [x] **P0 Safe** Ensure legacy fallback remains and is gated (unparseable-only fallback).
- [x] **P1 Opt-In** Expand native signature coverage and reduce fallback reliance.
- [x] **P1 Safe** Replace “max iters = 100” with early-stop and better convergence checks.

---

## G) Blank Lines DSL-ization

- [x] **P0 Safe** Keep shim default for parity; native remains opt-in.
- [x] **P1 Safe** Reduce iterations / improve idempotence for native blank-line rules.
- [x] **P2 Opt-In** Add additional blank-line rules only under explicit profile.

---

## H) Comment Strategy

- [ ] **P0 Safe** Expand directive preservation tests for more directive variants.
- [ ] **P1 Opt-In** Decide long-term comment approach: legacy oracle vs minimal comment DSL.
- [ ] **P2 Risky** Full comment typesetting engine (only if proven necessary).

---

## I) Testing Strategy (No Goldens)

- [ ] **P0 Safe** Expand property tests (idempotence + AST equivalence) across modes/profiles.
- [ ] **P1 Safe** Add more regression snippet tests for tricky AST constructs (non-golden).
- [ ] **P1 Safe** Expand “invalid Go doesn’t panic” coverage.

---

## J) Deprecation + Cleanup

- [ ] **P1 Safe** Remove dead plumbing once StagePlan/profile are authoritative.
- [ ] **P1 Safe** Shrink `StageOptions` by grouping knobs (Legacy/DSL/Style).
