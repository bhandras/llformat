#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: tools/plan_diff.sh [--llformat PATH] [--baseline DIR] [--update] <files...>

Runs llformat on each file and compares the formatted output against saved
baseline files. Use --update to write new baselines.

The script always rebuilds llformat before running to ensure tests reflect
the current state of the repository.

Examples:
  tools/plan_diff.sh --baseline .format-baseline ~/work/lnd-origin
  tools/plan_diff.sh --baseline .format-baseline --update ~/work/lnd-origin/*.go
EOF
}

llformat_bin="./bin/llformat"
baseline_dir=".format-baseline"
update=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --llformat)
      llformat_bin="$2"
      shift 2
      ;;
    --baseline)
      baseline_dir="$2"
      shift 2
      ;;
    --update)
      update=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "unknown flag: $1" >&2
      usage
      exit 1
      ;;
    *)
      break
      ;;
  esac
done

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

# Always rebuild llformat to test current repo state
echo "rebuilding llformat..." >&2
go build -o "$llformat_bin" ./cmd/llformat

if [[ ! -x "$llformat_bin" ]]; then
  echo "llformat not found or not executable: $llformat_bin" >&2
  exit 1
fi

mkdir -p "$baseline_dir"

failures=0
files=()
for arg in "$@"; do
  if [[ -d "$arg" ]]; then
    for file in "$arg"/*.go; do
      if [[ -f "$file" ]]; then
        files+=("$file")
      fi
    done
    continue
  fi
  files+=("$arg")
done

for file in "${files[@]}"; do
  if [[ ! -f "$file" ]]; then
    echo "skip (not a file): $file" >&2
    continue
  fi

  # Create baseline path from file's basename to avoid path issues
  basename_file="$(basename "$file")"
  baseline_path="${baseline_dir}/${basename_file}.formatted"

  out_tmp="$(mktemp)"
  "$llformat_bin" "$file" > "$out_tmp" 2>/dev/null || true

  if [[ $update -eq 1 ]]; then
    mv "$out_tmp" "$baseline_path"
    echo "updated: $baseline_path" >&2
    continue
  fi

  if [[ ! -f "$baseline_path" ]]; then
    echo "missing baseline: $baseline_path" >&2
    failures=$((failures + 1))
    rm -f "$out_tmp"
    continue
  fi

  if ! diff -u "$baseline_path" "$out_tmp" >/dev/null; then
    echo "output changed: $file" >&2
    diff -u "$baseline_path" "$out_tmp" || true
    failures=$((failures + 1))
  fi

  rm -f "$out_tmp"
done

if [[ $failures -ne 0 ]]; then
  echo "output diffs found: $failures" >&2
  exit 1
fi

echo "outputs unchanged"
