# Contract: operator-visible output

**Feature**: 023-publish-host-ports

## Dry-run container line (`dryrun.DryRunContainerBackend.CreateContainer`)

Appended detail lines under the existing `[DOCKER] ContainerCreate:` entry:

```text
[DOCKER] ContainerCreate: name=ops.dashboard image=registry.local/dashboard:1.2.0
  attach to platform network=shrine.platform
  publish=127.0.0.1:8080->3000/tcp
```

Rules:
- Explicit port, or an automatic port already persisted for this app →
  `publish=127.0.0.1:<hostPort>-><containerPort>/tcp`
- Automatic port not yet allocated → `publish=127.0.0.1:(auto)-><containerPort>/tcp`
- The `attach to platform network` line appears whenever the **derived**
  attachment is true — including publish-only manifests — so the implied
  attachment is visible (US4/AS-5).
- Dry-run performs no allocation and no writes; repeated dry-runs are
  byte-identical given identical inputs and state (SC-005).

## Deploy output (FR-014)

The observer/event stream gains a `hostport.published` event (emitted by the
Docker backend after the container starts, on both the fresh-create and
already-running paths) so the deploy states the resulting mapping:

```text
    📡 Published ops/dashboard on 127.0.0.1:30000 -> 3000/tcp
```

This is where an operator discovers an automatically assigned port without
consulting anything else.

## Conflict failure (both paths)

See `planner-errors.md`. Errors go to stderr; exit code is non-zero; no engine
operation has executed.

## `shrine delete application <name>`

```text
$ shrine delete application dashboard
Released host port 30000 for ops/dashboard.
Removed deployment record for ops/dashboard.
```

- `--team`/`-t` optional; without it shrine searches all teams and errors on
  ambiguity, listing the candidates (kubectl-style convention).
- If the container still exists in Docker: error
  `application "ops/dashboard" still has a running container; run "shrine teardown ops" first`
  and nothing is released (Docker-authoritative).
- If the app has no allocation and no record: soft success (idempotent delete).
- `--dry-run` prints what would be released, writes nothing.

## `shrine delete team <name>`

Existing output gains one line when ports were held:

```text
Released 3 host port(s) for team "media".
```

## Documentation deliverables (US5 / FR-018)

- `docs/content/reference/manifest-schema.md`: `networking.publish` field entry
  (both forms, valid ranges, automatic range 30000–32767) and the four-row
  combination table from `contracts/manifest-schema.md`, verbatim.
- `docs/content/guides/publish-localhost.md`: how-to covering fixed vs automatic
  ports, port stability across redeploys, conflict behavior, and releasing ports
  via delete commands — the four questions of SC-007.
