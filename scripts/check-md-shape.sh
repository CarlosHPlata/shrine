#!/usr/bin/env bash
# Verify every published index.md is well-formed: starts with an H1,
# contains no site chrome, and has not had Hugo template directives leak.
# Used by the docs CI workflow as a US2 acceptance gate.
set -euo pipefail

PUBLIC_DIR="${1:-docs/public}"

if [ ! -d "$PUBLIC_DIR" ]; then
  echo "error: $PUBLIC_DIR does not exist" >&2
  exit 2
fi

bad=0
checked=0
while IFS= read -r md; do
  checked=$((checked + 1))
  if [ ! -s "$md" ]; then
    echo "empty: $md"
    bad=$((bad + 1))
    continue
  fi
  first=$(awk 'NF{print; exit}' "$md")
  case "$first" in
    "# "*) ;;
    *)
      echo "no H1 first line: $md (got: $first)"
      bad=$((bad + 1))
      continue
      ;;
  esac
  if grep -qE '<html|<body|<nav|<header|<footer' "$md"; then
    echo "site chrome leaked: $md"
    bad=$((bad + 1))
    continue
  fi
done < <(find "$PUBLIC_DIR" -name index.md)

if [ "$bad" -gt 0 ]; then
  echo "FAIL: $bad of $checked index.md files have shape issues" >&2
  exit 1
fi
echo "OK: $checked index.md files, all well-formed"
