#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: tools/find_overflow_blocks.sh --dir DIR [options]

Scans Go files under DIR, formats them with llformat, finds lines that
exceed the column limit, and writes a markdown report with AST-snippets.

Options:
  --dir DIR            Root directory to scan (required).
  --exclude DIR        Exclude directory (repeatable).
  --out FILE           Output report file (default: BUG_REPORTS.md).
  --llformat PATH      llformat binary (default: ./bin/llformat).
  --ast-grep PATH      ast-grep binary (default: ast-grep).
  --col N              Column limit (default: 80).
  --tab N              Tab stop (default: 8).

Example:
  tools/find_overflow_blocks.sh --dir ~/work/llformat --exclude testdata
EOF
}

scan_dir=""
out_file="BUG_REPORTS.md"
llformat_bin="./bin/llformat"
ast_grep_bin="ast-grep"
col_limit=80
tab_stop=8
exclude_dirs=()

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
    --out)
      out_file="$2"
      shift 2
      ;;
    --llformat)
      llformat_bin="$2"
      shift 2
      ;;
    --ast-grep)
      ast_grep_bin="$2"
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

if ! command -v "$ast_grep_bin" >/dev/null 2>&1; then
  echo "ast-grep not found: $ast_grep_bin" >&2
  exit 1
fi

scan_dir="${scan_dir%/}"

tmp_report="$(mktemp)"
trap 'rm -f "$tmp_report"' EXIT

cat > "$tmp_report" <<EOF
# Formatting Overflow Report

Root: \`$scan_dir\`
Column limit: \`$col_limit\`
Tab stop: \`$tab_stop\`

EOF

rg --files -g '*.go' "$scan_dir" | while read -r file; do
  skip=0
  for ex in "${exclude_dirs[@]}"; do
    case "$file" in
      "$scan_dir/$ex"/*|"$ex"/*)
        skip=1
        break
        ;;
    esac
  done
  if [[ $skip -eq 1 ]]; then
    continue
  fi

  formatted_tmp="$(mktemp)"
  ast_json_tmp="$(mktemp)"
  overflow_json_tmp="$(mktemp)"
  trap 'rm -f "$formatted_tmp" "$ast_json_tmp" "$overflow_json_tmp"' RETURN

  "$llformat_bin" --col "$col_limit" --tab "$tab_stop" "$file" > "$formatted_tmp"

  python3 - "$formatted_tmp" "$col_limit" "$tab_stop" > "$overflow_json_tmp" <<'PY'
import json
import sys

path = sys.argv[1]
col = int(sys.argv[2])
tab = int(sys.argv[3])

items = []
with open(path, "r", encoding="utf-8") as f:
    for idx, line in enumerate(f, 1):
        visual = 0
        for ch in line.rstrip("\n"):
            if ch == "\t":
                visual += tab - (visual % tab)
            else:
                visual += 1
        if visual > col:
            items.append({
                "line": idx,
                "width": visual,
                "text": line.rstrip("\n"),
            })

print(json.dumps(items))
PY

  if [[ $(wc -c < "$overflow_json_tmp") -le 2 ]]; then
    continue
  fi

  "$ast_grep_bin" --lang go --pattern '$X' --json "$formatted_tmp" \
    > "$ast_json_tmp" || true

  python3 - \
    "$file" \
    "$formatted_tmp" \
    "$overflow_json_tmp" \
    "$ast_json_tmp" \
    "$tmp_report" <<'PY'
import json
import os
import sys

src_path = sys.argv[1]
formatted_path = sys.argv[2]
overflow_path = sys.argv[3]
ast_json_path = sys.argv[4]
report_path = sys.argv[5]

with open(overflow_path, "r", encoding="utf-8") as f:
    overflows = json.load(f)

if not overflows:
    sys.exit(0)

nodes = []
try:
    with open(ast_json_path, "r", encoding="utf-8") as f:
        ast_matches = json.load(f)
    for m in ast_matches:
        rng = m.get("range", {})
        start = rng.get("start", {})
        end = rng.get("end", {})
        nodes.append({
            "start_line": start.get("line", 0) + 1,
            "start_col": start.get("column", 0),
            "end_line": end.get("line", 0) + 1,
            "end_col": end.get("column", 0),
            "text": m.get("text", ""),
        })
except Exception:
    nodes = []

def best_node(line):
    candidates = []
    for n in nodes:
        if n["start_line"] <= line <= n["end_line"]:
            span_lines = n["end_line"] - n["start_line"]
            span_cols = n["end_col"] - n["start_col"]
            candidates.append((span_lines, span_cols, n))
    if not candidates:
        return None
    candidates.sort(key=lambda x: (x[0], x[1]))
    return candidates[0][2]

with open(formatted_path, "r", encoding="utf-8") as f:
    formatted_lines = f.read().splitlines()

with open(report_path, "a", encoding="utf-8") as out:
    out.write(f"## {src_path}\n\n")
    for item in overflows:
        line_no = item["line"]
        width = item["width"]
        line_text = item["text"]
        snippet = None
        node = best_node(line_no)
        if node and node["text"].strip():
            snippet = node["text"].rstrip("\n")
        if snippet is None:
            start = max(1, line_no - 2)
            end = min(len(formatted_lines), line_no + 2)
            snippet = "\n".join(formatted_lines[start - 1:end])
        out.write(f"- Line {line_no} (width {width}):\n\n")
        out.write("```go\n")
        out.write(snippet)
        out.write("\n```\n\n")
PY
done

mv "$tmp_report" "$out_file"
echo "wrote report to $out_file"
