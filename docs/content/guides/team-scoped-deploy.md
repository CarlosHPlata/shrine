---
title: "Team-scoped deploy"
description: "Deploy one team's apps and resources without touching the rest of the specs directory."
weight: 35
---

## What `shrine deploy team` does

`shrine deploy team <name>` reconciles **only** the apps and resources whose
`metadata.owner` matches `<name>`. The bare `shrine deploy` continues to deploy
the entire specs directory — the team subcommand is an additive verb, not a
behavior change.

Use it when:

- Your specs directory holds manifests owned by several teams and you only
  want to touch one team's containers.
- You're iterating on a single team's stack and want faster feedback than a
  full directory deploy.
- You want a safe "redeploy only my team" verb for daily ops while keeping
  bare `shrine deploy` as the platform-wide reconciler.

## How it works

The planner loads the **full** specs directory as resolution context, then
emits deploy steps only for manifests whose owner matches the requested team.
Concretely:

| Phase | Scope |
|---|---|
| Manifest scan | Full directory (so cross-team `valueFrom` and dependency references resolve) |
| Dependency / access / quota resolution | Full directory |
| Routing-collision detection | Full directory (you cannot accidentally introduce a route that collides with another team's existing route) |
| Step emission | **Team-owned manifests only** |
| Engine execution | Steps emitted in the previous phase |

## Walkthrough

Given a specs directory:

```text
specs/
├── marketing/
│   └── blog.yml          # owner: marketing
├── ops/
│   ├── monitor.yml       # owner: ops
│   └── shared-db.yml     # owner: ops, exposeToPlatform: true
└── teams/
    ├── marketing.yml
    └── ops.yml
```

Apply the team manifests once:

```bash
shrine apply teams --path ./specs/teams
```

Deploy only the `marketing` team's stack:

```bash
shrine deploy team marketing --path ./specs
```

```text
[shrine] Planning deployment for team "marketing" from: /home/operator/specs
... (only marketing's containers are reconciled; ops containers are untouched)
```

Bare `shrine deploy` still reconciles everything:

```bash
shrine deploy --path ./specs
# [shrine] Planning deployment from: /home/operator/specs
# (all teams: marketing AND ops)
```

## Cross-team dependencies

When `marketing/blog.yml` depends on `ops/shared-db.yml` (via
`spec.dependencies` or `spec.env[].valueFrom: resource.shared-db.host`), the
team-scoped deploy resolves the dependency from the loaded manifest set but
does **not** redeploy it.

```yaml
# marketing/blog.yml
spec:
  dependencies:
    - kind: Resource
      name: shared-db
      owner: ops
  env:
    - name: DB_HOST
      valueFrom: resource.shared-db.host
```

Requirements that already apply to bare `shrine deploy` apply equally here:

- The dependency's manifest **must be present** in the specs directory.
- The owner declared in `spec.dependencies` **must match** the dependency's
  `metadata.owner`.
- For cross-team Resource access, the dependency must declare
  `networking.exposeToPlatform: true` and (if its `access` list is non-empty)
  list the consumer team.

If the dependency hasn't actually been deployed yet, `shrine deploy team
marketing` will fail at engine-time the same way bare `shrine deploy` would —
the container will be created but won't reach the missing dependency. The
team subcommand intentionally does not add a plan-time deployment-state
lookup; deploy the owning team's stack first, then the dependents.

## Dry-run

`--dry-run` works identically to bare deploy: no Docker, no routing files, no
state writes. The planner still runs validation, dependency resolution, and
routing-collision detection — so dry-run is the safe way to preview a team's
reconciliation.

```bash
shrine deploy team marketing --dry-run --path ./specs
```

If two of `marketing`'s apps declare the same routing domain, the dry-run
exits non-zero with a collision error before any container changes are
considered. The same holds across teams: if `marketing/blog.yml` and
`ops/monitor.yml` both claim `home.lab`, the collision is reported during a
team-scoped dry-run even though only `marketing` is in the deploy scope.

## Error UX

A team name that doesn't match any `metadata.owner` in the specs directory
produces a clear error:

```text
$ shrine deploy team markting --path ./specs
Error: team "markting" not found in specs directory: known teams = [marketing ops]
```

The error names both the typo and the teams that *do* exist, so you can
correct it without grepping manifests.

If the specs directory contains no Application or Resource manifests at all,
the error variant says so explicitly:

```text
$ shrine deploy team anything --path ./empty-dir
Error: team "anything" not found: specs directory contains no Application or Resource manifests
```

## What `shrine deploy team` does **not** do

- It does not change the Team manifest itself — that's `shrine apply teams`.
- It does not redeploy other teams' apps or resources, even when they're
  referenced as cross-team dependencies of the requested team.
- It does not introduce any new manifest fields. Team ownership is the
  pre-existing `metadata.owner` value; the subcommand simply lets you filter
  deploys by that value.

## Related

- [`shrine deploy`](/cli/deploy/) — full-directory reconcile (unchanged).
- [`shrine deploy team`](/cli/deploy_team/) — auto-generated CLI reference.
- [`shrine apply teams`](/cli/apply_teams/) — manage the team registry itself.
