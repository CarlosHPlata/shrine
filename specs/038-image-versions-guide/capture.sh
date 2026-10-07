#!/usr/bin/env bash
# Prints every block of the image-versions guide that the shrine binary
# produces without a container runtime (research R1). DOCKER_HOST points at a
# closed port so no Docker daemon is ever contacted; state is seeded in the
# formats internal/state/local reads, with the placeholders of research R5.
#
# Usage: bash specs/038-image-versions-guide/capture.sh [path/to/shrine]
set -u

BIN="$(cd "$(dirname "${1:-./shrine}")" && pwd)/$(basename "${1:-./shrine}")"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

export DOCKER_HOST=tcp://127.0.0.1:1
CFG="$WORK/config"; ST="$WORK/state"; M="$WORK/manifests"
mkdir -p "$CFG" "$ST" "$M/teams"

API1=sha256:3f2a9c1b4d7ebacb024fcc9cc3ba71306a98135816442f1b7d6817ed226ae2e2
API2=sha256:5e8c2b7a1f04e575d034d87b11b2b1464d85f44eb790ab617a8e7e248bef8aef
PG16=sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460
PG17=sha256:9c1b4d7e3f2abafaeca130ff41ae79c7f98018fd87e177d20f90c7d5b32c63f1
API_CID=4c7e19a2b8d0f3e6a5c1d9b7e2f4a8c6d0b3e5f7a9c1d3e5f7b9d1f3a5c7e9b1
CACHE_CID=7a3e5c9b1d2f4a6c8e0b2d4f6a8c0e2b4d6f8a0c2e4b6d8f0a2c4e6b8d0f2a4c
DB_CID=5d0a11c3b2e4f6a8c0d2e4f6b8a0c2e4d6f8b0a2c4e6d8f0b2a4c6e8d0f2b4a6

# The manifest directory reads as the guide's, and table padding is dropped.
tidy() { sed -e "s#$M#/home/me/shop/manifests#g" -e 's/[[:space:]]*$//'; }

run() {
  echo "\$ shrine $*"
  "$BIN" --config-dir "$CFG" --state-dir "$ST" "$@" 2>&1 | tidy
  echo "[exit ${PIPESTATUS[0]}]"
  echo
}

section() { printf '\n######## %s\n\n' "$1"; }

config() { printf 'specsDir: %s\n%s' "$M" "${1:-}" > "$CFG/config.yml"; }

reset_state() {
  rm -rf "$ST"; mkdir -p "$ST/shop"
  "$BIN" --config-dir "$CFG" --state-dir "$ST" apply teams --path "$M/teams" > /dev/null 2>&1
}

pins() { printf '%s\n' "$@" > "$ST/shop/pins.txt"; }

deployments() { printf '%s\n' "$@" > "$ST/shop/deployments.txt"; }

write_team() {
  cat > "$M/teams/shop.yml" <<'EOF'
apiVersion: shrine/v1
kind: Team
metadata:
  name: shop
spec:
  displayName: "Shop"
  contact: shop@example.com
  quotas:
    maxApps: 5
    maxResources: 5
EOF
}

write_api() {
  cat > "$M/api.yml" <<EOF
apiVersion: shrine/v1
kind: Application
metadata:
  name: api
  owner: shop
spec:
  image: ${1:-traefik/whoami}
  imagePullPolicy: Pinned
  port: 80
EOF
}

write_db() {
  cat > "$M/shop-db.yml" <<EOF
apiVersion: shrine/v1
kind: Resource
metadata:
  name: shop-db
  owner: shop
spec:
  type: postgres
${1:-}  imagePullPolicy: Pinned
  env:
    - name: POSTGRES_PASSWORD
      value: change-me
EOF
}

write_cache() {
  cat > "$M/cache.yml" <<EOF
apiVersion: shrine/v1
kind: Resource
metadata:
  name: cache
  owner: shop
spec:
  type: redis
  version: "7.4"
${1:-}
EOF
}

seed_records() {
  deployments \
    "Application api $API_CID 9f2c4e6a8b0d1f3e traefik/whoami Pinned" \
    "Resource cache $CACHE_CID 1e3a5c7e9b0d2f4a redis:7.4 IfNotPresent" \
    "Resource shop-db $DB_CID 6b8d0f2a4c6e8b0d postgres Pinned"
}

write_team; config; reset_state

section "S3a validation: a fixed version under Pinned"
write_api traefik/whoami:v1.10.1; write_db '  version: "16"
'; write_cache
run deploy --dry-run

section "S2 the example manifests"
write_api; write_db
cat "$M/api.yml"; echo; cat "$M/shop-db.yml"; echo; cat "$M/cache.yml"; echo

section "S3b dry run of the first deploy"
run deploy --dry-run

section "S3c dry run after bump resource shop-db -v 16"
pins "Resource shop-db postgres:16 postgres@$PG16 2026-10-01T09:12:44Z"
run deploy --dry-run

section "S5 get and describe after the first deploy"
pins "Application api traefik/whoami traefik/whoami@$API1 2026-10-01T09:14:02Z" \
     "Resource shop-db postgres:16 postgres@$PG16 2026-10-01T09:12:44Z"
seed_records
run get deployed
run get resources --team shop
run describe resource shop-db
run describe app api

section "S6 bump refusals and dry run"
run bump resource shop-db -v 17 --dry-run
run bump resource shop-db -v postgres:18
run bump resource cache -v 8
run bump app web

section "S6b get and describe after bump resource shop-db -v 17, before the deploy"
pins "Application api traefik/whoami traefik/whoami@$API1 2026-10-01T09:14:02Z" \
     "Resource shop-db postgres:17 postgres@$PG17 2026-10-20T08:03:51Z"
run get resources --team shop
run describe resource shop-db

section "S8 bump app api dry run"
run bump app api --dry-run

section "S9 the configuration default"
config 'imagePullPolicy: Pinned
'
reset_state
run deploy --dry-run
write_cache '  imagePullPolicy: IfNotPresent'
run deploy --dry-run
run generate application web --team shop
find "$M" -name 'web.yml' -exec cat {} \;
find "$M" -name 'web.yml' -delete
write_cache; config

section "S10 delete after teardown"
reset_state
pins "Application api traefik/whoami traefik/whoami@$API2 2027-01-15T10:41:27Z" \
     "Resource shop-db postgres@$PG16 postgres@$PG16 2026-10-21T07:55:10Z"
run delete resource shop-db --dry-run
run delete resource shop-db
run delete team shop
