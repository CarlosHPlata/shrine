# Phase 0 Research: Secrets Vault Plugin (Infisical)

## Infisical Go SDK

**Decision**: Use the official `github.com/infisical/go-sdk` package.

**Rationale**: Official, maintained by the Infisical team, covers Universal Auth (Machine Identity) natively. Avoids hand-rolling HTTP calls against the API.

**Init pattern**:
```go
client := infisical.NewInfisicalClient(ctx, infisical.Config{
    SiteUrl:          "http://infisical:8080",  // self-hosted URL
    AutoTokenRefresh: true,
})
_, err := client.Auth().UniversalAuthLogin("client-id", "client-secret")
```

**Fetch pattern**:
```go
secret, err := client.Secrets().Retrieve(infisical.RetrieveSecretOptions{
    SecretKey:   "DB_PASSWORD",
    Environment: "production",
    ProjectID:   "my-project",   // slug or UUID — see path format below
    SecretPath:  "/",
})
val := secret.SecretValue
```

`AutoTokenRefresh: true` handles token renewal automatically; the plugin constructs the client once and reuses it across the full resolve pass.

---

## Authentication Method

**Decision**: Universal Auth (Machine Identity) — `client-id` + `client-secret`.

**Rationale**: Service tokens are deprecated upstream as of Infisical v0.19+. Machine Identities are the current recommended approach for non-human access. They support fine-grained project/environment scoping.

**Alternatives considered**:
- Service tokens — rejected; deprecated, narrower API access.
- AWS IAM / GCP ID Token — rejected; unnecessary for homelab (no cloud provider dependency).

---

## Path Format: `vault:<project>/<environment>/<secret-name>`

**Decision**: Three slash-separated components. "Project" is the **project slug** (human-readable identifier set in Infisical UI, e.g. `myapp`). Environment is the Infisical environment slug (e.g. `production`, `staging`). Secret name is the exact key stored in Infisical.

**Rationale**: Slugs are more ergonomic than UUIDs for a homelab operator writing YAML. The Infisical Go SDK's `ProjectID` field accepts both slugs and UUIDs in current versions — slugs are preferred for readability.

**Validation at plan time**: Path MUST have exactly 3 non-empty `/`-separated components. A malformed path (e.g. `vault:foo`, `vault:foo/bar`) is rejected before execution begins.

**Alternatives considered**:
- UUID-only project IDs — rejected; poor ergonomics (operator must copy UUIDs from UI).
- Two-component path (`env/key`) with project scoped to the config block — rejected; prevents a single shrine.yml from referencing secrets across multiple Infisical projects.

---

## Plugin Construction & Lifecycle

**Decision**: The `InfisicalPlugin` is constructed once in `handler/deploy.go` (same location as the Traefik plugin), validated immediately, and passed into `NewLiveResolver`. The Infisical SDK client is initialised once at construction; `AutoTokenRefresh` handles credential expiry internally.

**Failure semantics**: Any fetch error (missing secret, permission denied, token revocation) propagates immediately from `GetSecret()` → `lookupValueFrom()` → `ResolveApplication()` → `ExecuteDeploy()`, aborting the deploy before any container operation. This is consistent with the all-or-nothing contract.

---

## Nil-Safety & Dry-Run

**Decision**: `SecretsPlugin` is an interface. `LiveResolver` holds it as an optional field (nil when no vault is configured). `DryRunResolver` holds a nil vault and returns `[VAULT:<path>]` placeholder without calling the interface. A nil vault with no `vault:` refs in any manifest is a no-op (zero regression).

**Rationale**: Mirrors how nil `RoutingBackend` and `DNSBackend` are handled in the engine — nil backends are silently skipped.

---

## Config Validation at Load Time

**Decision**: If two `plugins.secrets.*` blocks are present in shrine.yml, `config.Load()` returns an error before any handler executes. Implemented as a simple count check over the known secrets plugin fields.

---

## Integration Test Approach

**Decision**: The integration test scenario for vault resolution uses a real Infisical instance started as a Docker Compose side-stack (postgres + redis + infisical) via a test helper. The test:
1. Starts Infisical, provisions a project and secret via the Infisical API.
2. Writes a shrine.yml with `plugins.secrets.infisical` and a manifest with `valueFrom: vault:...`.
3. Runs `shrine apply` as a subprocess.
4. Asserts the container has the correct env var set.

The Infisical test stack is only started when the `integration` build tag is present.
