# Quickstart: valueFrom-Inferred Deploy Order

**Feature**: `020-infer-valuefrom-deps`
**Audience**: Operators verifying the feature works end-to-end, and
contributors adding a new enrichment rule.

---

## Part A — Operator walkthrough (verify the bug is fixed)

### Setup

Create a fresh specs directory with two manifests under the same team
`ops_bot`.

`specs/ops-bot-db.yaml`:

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: ops-bot-db
  owner: ops_bot
spec:
  type: postgres
  version: "16"
  outputs:
    - name: DB_CONNECTION_URL
      template: "postgres://{{.user}}:{{.password}}@{{.host}}:{{.port}}/{{.db}}"
```

`specs/ops-bot.yaml`:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: ops-bot
  owner: ops_bot
spec:
  image: registry.example/ops-bot:1.0
  env:
    - name: DB_CONNECTION_URL
      valueFrom: resource.ops-bot-db.DB_CONNECTION_URL
```

Note: the `Application` has **no** `spec.dependencies` block.

### Verify ordering (dry-run)

```bash
shrine deploy team ops_bot --dry-run
```

Expected output on stdout begins with:

```
Deploy order:
  1. Resource:ops-bot-db
  2. Application:ops-bot
       depends on:
         - Resource:ops-bot-db (inferred from env DB_CONNECTION_URL)
```

Followed by the usual `[DOCKER] …` lines.

Key assertions to eyeball:
- `Resource:ops-bot-db` is step 1, `Application:ops-bot` is step 2.
- The dependency line carries `(inferred from env DB_CONNECTION_URL)`.

### Verify ordering (real deploy)

```bash
shrine deploy team ops_bot
```

The summary header does NOT print (real deploys are silent on the
summary), but the Docker operations for `ops-bot-db` complete before
`ops-bot` is created. If `ops-bot-db` fails to come up, `ops-bot` is
never created — same gate as if the operator had declared the
dependency explicitly.

### Verify on-disk state is untouched

After running either command above:

```bash
git status specs/
# expected: nothing to commit, working tree clean
```

The planner never writes back to YAML (FR-004, SC-006).

### Verify the cross-team failure

Add a second team's resource that the first team can read:

`specs/team-b/shared.yaml`:

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: shared-cache
  owner: team_b
  access: [ops_bot]
spec:
  type: redis
  version: "7"
  networking:
    exposeToPlatform: true
  outputs:
    - name: HOST
      template: "{{.host}}"
```

Add an env var on `ops-bot` referencing it:

```yaml
    - name: CACHE_HOST
      valueFrom: resource.shared-cache.HOST
```

Run:

```bash
shrine deploy team ops_bot --dry-run
```

The planner now **fails** before any deploy step runs. You will see
on stderr:

```
enrichment: app "ops-bot" env "CACHE_HOST" references resource "shared-cache.HOST" which is not owned by team "ops_bot"; add an explicit spec.dependencies entry (kind: Resource, name: shared-cache) to declare this dependency
```

The exit code is non-zero. No plan summary is printed and no Docker
operations are attempted.

To make the deploy succeed, declare the cross-team coupling explicitly:

```yaml
spec:
  dependencies:
    - kind: Resource
      name: shared-cache
      owner: team_b
```

Now `shrine deploy team ops_bot --dry-run` succeeds. The dry-run
summary shows `Resource:shared-cache` as an explicit dep (no
`(inferred from …)` tag), and the deploy proceeds normally.

> **Why a failure and not a warning?** Cross-team coupling means
> another team owns the lifecycle of `shared-cache`. Inferring the
> ordering silently would hide the coupling; emitting only a warning
> would let an under-specified deploy proceed. Requiring an explicit
> `spec.dependencies` entry forces the coupling to be visible in code
> review.

### Verify the absent-target failure

The same error fires if you reference a manifest that simply does not
exist in the loaded set — typo or otherwise:

```yaml
    - name: BAD_REF
      valueFrom: resource.does-not-exist.HOST
```

`shrine deploy team ops_bot --dry-run` produces:

```
enrichment: app "ops-bot" env "BAD_REF" references resource "does-not-exist.HOST" which is not owned by team "ops_bot"; add an explicit spec.dependencies entry (kind: Resource, name: does-not-exist) to declare this dependency
```

Because `shrine deploy team ops_bot` loads all manifests owned by
`ops_bot`, an absent target is by construction not same-team. The
remediation is to fix the typo (or remove the env var, or declare an
explicit dep if the target lives in another team).

### Multiple bad references — fail-fast

If multiple env vars on the same Application — or across multiple
Applications in the team — would each individually trigger the
failure, only the **first** one (in sorted Application name +
declaration order on each Application's env list) is reported per run.
Fix that one and re-run to surface the next.

---

## Part B — Contributor walkthrough (add a new enrichment rule)

This walkthrough shows how to add a hypothetical "implicit ordering
from `valueFrom: vault:…` references that resolve to a manifest-owned
secret" rule. The exact rule is illustrative; the steps generalize.

### Step 1 — create the rule file

`internal/planner/enrich_vault.go`:

```go
package planner

import "github.com/CarlosHPlata/shrine/internal/manifest"

type enrichValueFromVault struct{}

func (enrichValueFromVault) Enrich(set *ManifestSet) (*ManifestSet, error) {
    // Use applyEnrichmentRule with your own parseFor and lookupOwner.
    // Return new set + error (use *EnrichmentError for FR-010-style
    // failures). NEVER mutate `set`.
    return set, nil // (skeleton — implement parse + gate + dedup here)
}
```

### Step 2 — register it in the default chain

`internal/planner/enrich.go`:

```go
func DefaultEnrichers() []Enricher {
    return []Enricher{
        enrichValueFromResource{},
        enrichValueFromApplication{},
        enrichValueFromVault{},  // ← new rule, appended
    }
}
```

That is the **only** edit to an existing file. The previous two rule
files are untouched (FR-007).

### Step 3 — add unit tests

`internal/planner/enrich_vault_test.go`:

```go
func TestEnrichValueFromVault_AddsEdgeForSameOwnerSecret(t *testing.T) { … }
func TestEnrichValueFromVault_FailsOnCrossTeamWithoutExplicitDep(t *testing.T) { … }
func TestEnrichValueFromVault_SucceedsOnCrossTeamWithExplicitDep(t *testing.T) { … }
func TestEnrichValueFromVault_FailsOnAbsentTargetWithoutExplicitDep(t *testing.T) { … }
func TestEnrichValueFromVault_DedupsAgainstExplicit(t *testing.T) { … }
func TestEnrichValueFromVault_DoesNotMutateInputOnSuccessOrFailure(t *testing.T) { … }
```

Use the test fixtures already in `enrich_valuefrom_test.go` as a model
— same shape, different `valueFrom` strings.

### Step 4 — extend the integration test if the rule changes
operator-visible output

If the new rule produces a new dry-run provenance tag (e.g., `(inferred
from vault env <NAME>)`), update `tests/integration/deploy_team_infer_test.go`
to assert the new tag for the new scenario.

### Step 5 — run the gates

```bash
go test ./internal/planner/...                              # unit gate
go test -tags integration ./tests/integration/...           # integration gate (Principle V)
make docs-gen-cli && git diff --exit-code docs/content/cli/ # docs freshness
```

The constitution requires the integration gate before the phase is
declared complete. For this feature, the gating test is
`tests/integration/deploy_team_infer_test.go`.

---

## Part C — Troubleshooting

| Symptom | Likely cause |
|---|---|
| `enrichment: app "<X>" env "<Y>" references … which is not owned by team …` | The `valueFrom` references a manifest owned by another team, or a manifest that is not in the loaded set. Either fix the typo, or declare an explicit `spec.dependencies` entry for the cross-team target (US3 acceptance scenario 2). |
| Planner fails on what looks like a same-team reference | Confirm the target manifest is actually loaded — `shrine deploy team <T>` only loads manifests with `metadata.owner == <T>`. Confirm both consumer and target have the same `metadata.owner` value (typos in the owner field count as cross-team). |
| Inferred dep is missing in dry-run | The env var uses literal `value:` (not `valueFrom:`), or its `valueFrom` uses `vault:…` (not currently inferred). Note: a `valueFrom: resource.<X>.<Y>` whose target is absent from the loaded set no longer silently skips — it FAILS the planner (see first row). |
| Duplicate edge in dry-run summary | This is a bug — file an issue. Dedup is keyed on `(Kind, Name)` and should suppress duplicates. |
| Multiple cross-team failures reported in one run | Enrichment is fail-fast — only the first offending reference is reported per run. Fix it and re-run to surface the next one. |
| YAML on disk shows changes after running `shrine deploy` | This is a bug — file an issue. Enrichment must never write to disk (FR-004, SC-006). |

For deeper debugging, run with `--dry-run` first; the plan summary makes
provenance explicit and removes a layer of guesswork.
