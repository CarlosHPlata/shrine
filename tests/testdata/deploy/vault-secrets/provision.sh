#!/usr/bin/env bash
# provision.sh — seed a fresh self-hosted Infisical instance with the data
# required by TestDeploy_VaultSecrets.
#
# Expects:
#   INFISICAL_URL  — base URL of the Infisical instance (e.g. http://localhost:8080)
#
# Writes:
#   tests/testdata/deploy/vault-secrets/shrine.yml   — updated with real credentials
#
# Exit codes:
#   0  success
#   1  any provisioning step failed

set -euo pipefail

INFISICAL_URL="${INFISICAL_URL:-http://localhost:8080}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHRINE_YML="${SCRIPT_DIR}/shrine.yml"

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------
post() {
  local path="$1"; shift
  curl -sf -X POST \
    -H "Content-Type: application/json" \
    "$@" \
    "${INFISICAL_URL}${path}"
}

get() {
  local path="$1"; shift
  curl -sf -X GET \
    -H "Content-Type: application/json" \
    "$@" \
    "${INFISICAL_URL}${path}"
}

# ---------------------------------------------------------------------------
# 1. Create admin account (first-time setup)
# ---------------------------------------------------------------------------
echo "==> Creating admin account..."
SIGNUP_RESP=$(post "/api/v1/auth/email/signup" \
  -d '{"email":"admin@shrine-ci.local","password":"Shrine-CI-P@ss1","firstName":"CI","lastName":"Admin"}' \
  2>/dev/null || true)

# If signup returns a token directly, grab it; otherwise login.
ADMIN_TOKEN=$(echo "${SIGNUP_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('token',''))" 2>/dev/null || true)

if [ -z "${ADMIN_TOKEN}" ]; then
  echo "==> Signup complete or already exists — logging in..."
  LOGIN_RESP=$(post "/api/v1/auth/email/login" \
    -d '{"email":"admin@shrine-ci.local","password":"Shrine-CI-P@ss1"}')
  ADMIN_TOKEN=$(echo "${LOGIN_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['token'])")
fi

AUTH_HEADER="-H \"Authorization: Bearer ${ADMIN_TOKEN}\""

# ---------------------------------------------------------------------------
# 2. Create / retrieve organization
# ---------------------------------------------------------------------------
echo "==> Creating organization..."
ORG_RESP=$(post "/api/v2/organizations" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -d '{"name":"shrine-ci"}' 2>/dev/null || true)

ORG_ID=$(echo "${ORG_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('organization',{}).get('id','') or d.get('id',''))" 2>/dev/null || true)

if [ -z "${ORG_ID}" ]; then
  echo "==> Fetching existing org..."
  ORG_ID=$(get "/api/v1/organization" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    | python3 -c "import sys,json; orgs=json.load(sys.stdin)['organizations']; print(orgs[0]['id'])")
fi
echo "    org_id=${ORG_ID}"

# ---------------------------------------------------------------------------
# 3. Create project (workspace)
# ---------------------------------------------------------------------------
echo "==> Creating project..."
PROJ_RESP=$(post "/api/v2/workspaces" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -d "{\"organizationId\":\"${ORG_ID}\",\"projectName\":\"shrine-test\",\"projectSlug\":\"shrine-test\"}" \
  2>/dev/null || true)

PROJ_ID=$(echo "${PROJ_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('workspace',{}).get('id','') or d.get('id',''))" 2>/dev/null || true)

if [ -z "${PROJ_ID}" ]; then
  echo "==> Fetching existing project..."
  PROJ_ID=$(get "/api/v1/organization/${ORG_ID}/workspaces" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    | python3 -c "import sys,json; ws=json.load(sys.stdin)['workspaces']; print(next(w['id'] for w in ws if w['slug']=='shrine-test'))")
fi
echo "    project_id=${PROJ_ID}"

# ---------------------------------------------------------------------------
# 4. Create Machine Identity with Universal Auth
# ---------------------------------------------------------------------------
echo "==> Creating machine identity..."
IDENTITY_RESP=$(post "/api/v1/identities" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -d "{\"organizationId\":\"${ORG_ID}\",\"name\":\"shrine-ci-identity\",\"role\":\"admin\"}")

IDENTITY_ID=$(echo "${IDENTITY_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['identity']['id'])")
echo "    identity_id=${IDENTITY_ID}"

UA_RESP=$(post "/api/v1/auth/universal-auth/identities/${IDENTITY_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -d '{"accessTokenMaxTTL":2592000,"accessTokenNumUsesLimit":0}')

CLIENT_ID=$(echo "${UA_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['identityUniversalAuth']['clientId'])")

CRED_RESP=$(post "/api/v1/auth/universal-auth/identities/${IDENTITY_ID}/client-secrets" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -d '{"description":"ci","ttl":2592000}')

CLIENT_SECRET=$(echo "${CRED_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['clientSecret'])")
echo "    client_id=${CLIENT_ID}"

# Attach identity to project
post "/api/v2/workspace/${PROJ_ID}/identity-memberships/${IDENTITY_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -d '{"role":"admin"}' > /dev/null

# ---------------------------------------------------------------------------
# 5. Create test secrets in the production environment
# ---------------------------------------------------------------------------
echo "==> Creating test secrets..."

# Get an identity token for creating secrets
ID_TOKEN_RESP=$(post "/api/v1/auth/universal-auth/login" \
  -d "{\"clientId\":\"${CLIENT_ID}\",\"clientSecret\":\"${CLIENT_SECRET}\"}")
ID_TOKEN=$(echo "${ID_TOKEN_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['accessToken'])")

post "/api/v3/secrets/raw/DB_PASSWORD" \
  -H "Authorization: Bearer ${ID_TOKEN}" \
  -d "{\"workspaceId\":\"${PROJ_ID}\",\"environment\":\"prod\",\"secretValue\":\"ci-db-password-123\"}" > /dev/null

post "/api/v3/secrets/raw/API_KEY" \
  -H "Authorization: Bearer ${ID_TOKEN}" \
  -d "{\"workspaceId\":\"${PROJ_ID}\",\"environment\":\"prod\",\"secretValue\":\"ci-api-key-xyz\"}" > /dev/null

echo "==> Secrets created."

# ---------------------------------------------------------------------------
# 6. Update shrine.yml fixture with real credentials
# ---------------------------------------------------------------------------
echo "==> Writing shrine.yml..."
cat > "${SHRINE_YML}" <<EOF
plugins:
  secrets:
    infisical:
      url: ${INFISICAL_URL}
      client-id: "${CLIENT_ID}"
      client-secret: "${CLIENT_SECRET}"
EOF

echo "==> Provisioning complete."
echo "    INFISICAL_URL=${INFISICAL_URL}"
echo "    CLIENT_ID=${CLIENT_ID}"
