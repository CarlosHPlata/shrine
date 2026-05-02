#!/usr/bin/env bash
# Verify every published HTML page has a sibling index.md companion.
# Used by the docs CI workflow as a US2 acceptance gate.
set -euo pipefail

PUBLIC_DIR="${1:-docs/public}"

if [ ! -d "$PUBLIC_DIR" ]; then
  echo "error: $PUBLIC_DIR does not exist (run hugo first)" >&2
  exit 2
fi

missing=0
checked=0
while IFS= read -r html; do
  checked=$((checked + 1))
  dir=$(dirname "$html")
  if [ ! -f "$dir/index.md" ]; then
    echo "missing: $dir/index.md (next to $html)"
    missing=$((missing + 1))
  fi
done < <(find "$PUBLIC_DIR" -name index.html)

if [ "$missing" -gt 0 ]; then
  echo "FAIL: $missing of $checked pages are missing their index.md companion" >&2
  exit 1
fi
echo "OK: $checked pages, every one has an index.md companion"
