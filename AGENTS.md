## Agent Guidelines

This repository contains golden outputs used by tests under `testdata/**/output_next.go`. These files are **authoritative** fixtures and must not be altered by automation.

- Never modify `testdata/**/output_next.go` or any other golden results when making changes.
- If a change appears to require updating these golden files, stop immediately and ask a human maintainer for direction.
- Do not regenerate or rewrite the goldens as part of formatter changes or test fixes.

When in doubt about expected behavior vs. golden outputs, halt work and escalate to a human before proceeding.

## Project Context

- Purpose: `llformat` is a Go formatter that reflows comments and reformats targeted log/printf-style calls without touching other code.
- Build/test commands: `make build` (or `make all`) builds `bin/llformat`; `make test` / `make unit` run `go test -v ./...`; `make clean` removes `bin/`.
- CLI usage: `./bin/llformat <file.go>` (or `-w` to write in place); key flags include `--col`, `--tab`, and `--wrap-inline-comments`.
- Architecture highlights: CLI in `cmd/llformat/main.go`; comment formatting in `formatter/comment_formatter.go`; call formatting in `formatter/left_flow_call_formatter.go`; tests compare `input.go` → `output_next.go` goldens under `testdata/`.

## Workflow Expectations

- Before committing changes, run `make self-check` and `make lint`.
- Prefer `make fmt` / `make fmt-check` for formatting; the repo formatter tooling intentionally excludes `testdata/**` so golden fixtures stay untouched.
- CI runs the same checks via `.github/workflows/ci.yml`.
