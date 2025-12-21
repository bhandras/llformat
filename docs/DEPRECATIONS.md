# Deprecations

This document tracks user-facing deprecations for `llformat`.

## Legacy CLI modes

The CLI now defaults to the `next` pipeline (DSL-based, fixpoint iterations on
by default).

The following modes remain available for internal testing and comparison, but
are considered deprecated for interactive use:

- `--mode legacy`
- `--mode dsl-parity`
- `--mode dsl-modern`

These modes may be removed or moved behind a build tag once `next` fully
subsumes their coverage.

