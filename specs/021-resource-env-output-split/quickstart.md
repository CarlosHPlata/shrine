# Quickstart: Resource `env` / `output` split

Audience: operators writing resource manifests, and contributors implementing
the feature. Assumes a working shrine setup (see the project README).

## 1. The mental model

A Resource now has two distinct blocks:

| Block | Answers | Feeds |
|-------|---------|-------|
| `spec.env` | "What does this container run with?" | the resource container's environment |
| `spec.output` | "What may other manifests read from me?" | the export allowlist consumers see |

`env` is just like an Application's `env` (plus `generated` for secrets).
`output` is a pure list of names (+ optional `template`) — no values.

## 2. Author a resource

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: pg
  owner: data
spec:
  type: postgres
  version: "16"
  port: 5432
  env:
    - name: POSTGRES_DB
      value: app
    - name: POSTGRES_PASSWORD     # auto-minted, stays private
      generated: true
  output:
    - name: POSTGRES_DB           # re-export this env var
    - name: host                  # built-in
    - name: DB_URL                # derived; folds in the private password
      template: "postgres://app:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"
```

## 3. Consume it from an application

```yaml
spec:
  env:
    - name: DATABASE_URL
      valueFrom: resource.pg.DB_URL        # ✅ exported
    - name: DB_NAME
      valueFrom: resource.pg.POSTGRES_DB   # ✅ exported
    # - valueFrom: resource.pg.POSTGRES_PASSWORD  # ❌ not exported → validation error
```

## 4. A resource consuming another resource (new)

Resources are now first-class consumers — same rules as apps (access,
reachability, deploy order):

```yaml
# resource "cache" reads pg's exported host
spec:
  env:
    - name: UPSTREAM_DB_HOST
      valueFrom: resource.pg.host
```

`shrine` orders `pg` before `cache` automatically (same-team references are
inferred; cross-team requires an explicit `spec.dependencies` entry).

## 5. Preview and deploy

```bash
shrine deploy --dry-run --path ./manifests   # shows env (container) vs output (interface), no side effects
shrine deploy --path ./manifests
```

## 6. Migrating an old manifest

Old (rejected):

```yaml
spec:
  outputs:
    - name: POSTGRES_PASSWORD
      generated: true
    - name: DB_URL
      template: "...{{.POSTGRES_PASSWORD}}..."
```

New:

```yaml
spec:
  env:
    - name: POSTGRES_PASSWORD     # same name → same secret, no rotation
      generated: true
  output:
    - name: DB_URL
      template: "...{{.POSTGRES_PASSWORD}}..."
    # add `- name: POSTGRES_PASSWORD` here only if it must stay consumable
```

Running `shrine deploy` against the old shape prints an actionable error naming
the offending output and the fix.

## 7. Verify locally (contributor gate)

```bash
go build -o shrine .
go test ./...                                            # unit gates
go test -tags integration ./tests/integration/...        # Principle V gate (real binary + Docker)
graphify update .                                        # refresh the knowledge graph after code changes
```

Integration coverage to expect (`tests/integration/deploy_resource_env_output_test.go`):
US1 env→container + curated exports; US2 private secret not consumable; US3
old-manifest rejection; resource-as-consumer + ordering (FR-014).
