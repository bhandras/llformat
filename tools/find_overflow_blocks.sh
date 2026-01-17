#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: tools/find_overflow_blocks.sh --dir DIR [options]

Scans Go files under DIR, formats them with llformat, finds lines that
exceed the column limit, and writes a markdown report.

Implementation note:
- This is a wrapper around `go run ./tools/overflow_report`, which uses Go's
  parser/AST to find the enclosing declaration and emits a standalone repro
  snippet for each overflow.

Options:
  --dir DIR            Root directory to scan (required).
  --exclude DIR        Exclude directory (repeatable).
  --exclude-ext SUF    Exclude file suffix (repeatable, e.g. .pb.go).
  --out PATH           Output markdown file OR directory prefix for per-bug reports (default: BUG_REPORTS.md).
  --llformat PATH      llformat binary (default: ./bin/llformat).
  --ast-grep PATH      Deprecated/ignored (kept for compatibility).
  --col N              Column limit (default: 80).
  --tab N              Tab stop (default: 8).
  --min-excess N       Only report lines that exceed --col by at least N (default: 1).

Example:
  tools/find_overflow_blocks.sh --dir ~/work/llformat --exclude testdata
EOF
}

scan_dir=""
out_file="BUG_REPORTS.md"
llformat_bin="./bin/llformat"
col_limit=80
tab_stop=8
min_excess=1
declare -a exclude_dirs=()
declare -a exclude_suffixes=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      scan_dir="$2"
      shift 2
      ;;
    --exclude)
      exclude_dirs+=("$2")
      shift 2
      ;;
    --exclude-ext)
      exclude_suffixes+=("$2")
      shift 2
      ;;
    --out)
      out_file="$2"
      shift 2
      ;;
    --llformat)
      llformat_bin="$2"
      shift 2
      ;;
    --ast-grep)
      # Deprecated/ignored.
      shift 2
      ;;
    --col)
      col_limit="$2"
      shift 2
      ;;
    --tab)
      tab_stop="$2"
      shift 2
      ;;
    --min-excess)
      min_excess="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown flag: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$scan_dir" ]]; then
  usage
  exit 1
fi

if [[ ! -x "$llformat_bin" ]]; then
  echo "llformat not found or not executable: $llformat_bin" >&2
  exit 1
fi

scan_dir="${scan_dir%/}"

args=(
  go run ./tools/overflow_report
  --dir "$scan_dir"
  --out "$out_file"
  --llformat "$llformat_bin"
  --col "$col_limit"
  --tab "$tab_stop"
  --min-excess "$min_excess"
)
for ex in "${exclude_dirs[@]-}"; do
  args+=(--exclude "$ex")
done

for suf in "${exclude_suffixes[@]-}"; do
  args+=(--exclude-ext "$suf")
done

"${args[@]}"
echo "wrote report to $out_file"
