# Deprecations

This document tracks user-facing deprecations for `llformat`.

## Legacy CLI modes (removed)

The CLI now runs the `next` pipeline only (DSL-based, fixpoint iterations on by
default).

The following flags were removed from the user-facing CLI:

- `--mode` (including `legacy`, `dsl-parity`, `dsl-modern`, and `next`)
- `--legacy`
- `--legacy-hardening`
- `--trace-dsl`
- `--trace-dsl-reasons`
- `--dsl-*` stage/style knobs
