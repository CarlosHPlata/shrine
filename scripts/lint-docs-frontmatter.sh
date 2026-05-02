#!/usr/bin/env bash
# Lint front-matter on every hand-authored Markdown page under docs/content/.
# Rules:
#   - title: required, non-empty
#   - description: optional, when present <= 160 characters
#   - disallowed fields: date, lastmod, version (build-time concerns, not page-level)
#   - YAML front-matter must parse (delimited by `---` lines)
# Skipped: docs/content/cli/*.md (auto-generated; the drift check covers these).
set -euo pipefail

CONTENT_DIR="${1:-docs/content}"

if [ ! -d "$CONTENT_DIR" ]; then
  echo "error: $CONTENT_DIR does not exist" >&2
  exit 2
fi

bad=0
checked=0
disallowed_fields=("date" "lastmod" "version")

while IFS= read -r md; do
  case "$md" in
    */cli/*.md)
      [ "$(basename "$md")" = "_index.md" ] || continue
      ;;
  esac

  checked=$((checked + 1))

  if ! head -n 1 "$md" | grep -qE '^---$'; then
    echo "no front-matter: $md"
    bad=$((bad + 1))
    continue
  fi

  fm=$(awk '
    /^---[[:space:]]*$/ { delim++; if (delim == 2) exit; next }
    delim == 1 { print }
  ' "$md")

  if [ -z "$fm" ]; then
    echo "empty front-matter: $md"
    bad=$((bad + 1))
    continue
  fi

  title=$(printf '%s\n' "$fm" | awk -F': ' '/^title:/ { sub(/^title:[[:space:]]*/, ""); gsub(/^"|"$/, ""); print; exit }')
  if [ -z "$title" ]; then
    echo "missing or empty title: $md"
    bad=$((bad + 1))
    continue
  fi

  description=$(printf '%s\n' "$fm" | awk -F': ' '/^description:/ { sub(/^description:[[:space:]]*/, ""); gsub(/^"|"$/, ""); print; exit }')
  if [ -n "$description" ] && [ "${#description}" -gt 160 ]; then
    echo "description exceeds 160 chars (${#description}): $md"
    bad=$((bad + 1))
    continue
  fi

  for field in "${disallowed_fields[@]}"; do
    if printf '%s\n' "$fm" | grep -qE "^${field}:"; then
      echo "disallowed field '$field': $md"
      bad=$((bad + 1))
    fi
  done
done < <(find "$CONTENT_DIR" -name '*.md')

if [ "$bad" -gt 0 ]; then
  echo "FAIL: $bad of $checked pages have front-matter issues" >&2
  exit 1
fi
echo "OK: $checked pages, all front-matter valid"
