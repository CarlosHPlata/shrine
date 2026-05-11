# Vault secrets integration test — local setup

This fixture exercises `TestDeploy_VaultSecrets` against a real self-hosted
Infisical instance. The test is skipped by default; to run it locally you have
to bootstrap Infisical once (via its web UI, because the first-admin signup
uses E2EE crypto that can't be done cleanly with `curl`).

## 1. Start the Infisical stack

```bash
docker compose -f tests/testdata/deploy/vault-secrets/docker-compose.yml up -d
```

Wait ~30s for Infisical to migrate its DB. Confirm it's ready:

```bash
curl http://localhost:8080/api/status
```

## 2. Bootstrap Infisical (one-time, via browser)

Open `http://localhost:8080` and:

1. **Sign up** as the first admin (any email + password works — this instance is ephemeral).
2. **Create an organization** (any name).
3. **Create a project** with slug **`shrine-test`**.
4. Inside the project, **create an environment** with slug **`production`** (it may be created by default).
5. **Create two secrets** in the `production` environment:
   - `DB_PASSWORD` → any value (e.g. `ci-db-password-123`)
   - `API_KEY` → any value (e.g. `ci-api-key-xyz`)
6. **Create a Machine Identity**:
   - Access Control → Machine Identities → Create
   - Auth method: **Universal Auth**
   - Note the generated **Client ID** and **Client Secret** (Client Secret is only shown once)
7. **Attach the machine identity to the `shrine-test` project**:
   - Project → Access Control → Machine Identities → Add → select your identity → assign a role with read access to secrets

## 3. Run the integration test

```bash
export INFISICAL_TEST_URL=http://localhost:8080
export INFISICAL_CLIENT_ID=<your-client-id>
export INFISICAL_CLIENT_SECRET=<your-client-secret>

make test-integration
```

The test (`TestDeploy_VaultSecrets`) will:
- Generate a temporary `shrine.yml` with the credentials from the env vars
- Apply the team, then deploy the Resource + Application
- Assert the container has `DB_PASSWORD` and `API_KEY` injected with the vault values

If any of the three env vars is missing, the test skips with a pointer back to this README.

## 4. Tear down

```bash
docker compose -f tests/testdata/deploy/vault-secrets/docker-compose.yml down -v
```

## Why no automated provisioning?

Infisical's first-admin signup is end-to-end-encrypted: the client derives keys
from the password (SRP salt + verifier, X25519 keypair, AES-GCM-encrypted
private key) and posts the encrypted material. Reproducing that in bash isn't
feasible. A future CI integration could either:

- Write a small Go helper that runs the crypto and signup against the API; or
- Pre-seed the Postgres database with admin/org/project/identity rows directly.

Both are non-trivial and out of scope for this PR.
