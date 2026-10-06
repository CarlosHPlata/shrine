# Data Model: Deployed Version in get and describe

**Feature**: 031-deployed-version-columns | **Date**: 2026-10-06

## 1. Deployment record (`internal/state/deployments.go`) — MODIFIED

```go
type Deployment struct {
	Kind        string
	Name        string
	ContainerID string
	ConfigHash  string
	Image       string // the reference the manifest named, as written, e.g. reg:lab/hello-api:1.2.0
	Policy      string // effective image pull policy at deploy time
}
```

Per design section 3.3. The `DeploymentStore` interface (`Record`, `Remove`,
`List`) is unchanged.

| Field | Source at write time | Empty means |
|---|---|---|
| `Image` | `CreateContainerOp.Image` before registry-alias expansion | record predates this feature |
| `Policy` | `CreateContainerOp.ImagePullPolicy`, already the effective value from `manifest.EffectivePullPolicy` | record predates this feature |

Values today: `Policy` is `Always` or `IfNotPresent`. The field is a free string
so T3's `Pinned` needs no change here.

## 2. On-disk format (`<state-dir>/<team>/deployments.txt`) — MODIFIED

One line per artifact, fields separated by single spaces, in this order:

```text
<Kind> <Name> <ContainerID> <ConfigHash> <Image> <Policy>
```

Example:

```text
Application hello-api 9f1c…e2 3a7b…c9 reg:lab/hello-api:latest Always
Resource hello-db 5d0a…11 b2e4…77 postgres:16 IfNotPresent
```

**Writer rule** (`saveTeam`): always six fields, sorted by name, written whole by
atomic temp-and-rename. An empty optional value (`ConfigHash`, `Image`,
`Policy`) is written as `-` so the later fields keep their position.

**Reader rule** (`loadTeam`): a `#` starts a comment; blank lines are skipped; the
line is split on whitespace; fewer than three fields is ignored; fields four
(`ConfigHash`), five (`Image`), and six (`Policy`) are optional and read as empty
when absent or `-`. Lines therefore load as:

| Line shape | Loads as |
|---|---|
| `Kind Name ContainerID` | hash, image, policy empty |
| `Kind Name ContainerID ConfigHash` | image, policy empty |
| `Kind Name ContainerID - Image Policy` | hash empty, image and policy set |
| `Kind Name ContainerID ConfigHash Image Policy` | fully populated |

## 3. Lifecycle

- **Written** by the Docker backend after `ContainerStart` succeeds, on the fresh
  path (`createFreshContainer`) and on the up-to-date path (`ensureRunning`);
  the whole record is rewritten each time, so a legacy record gains `Image` and
  `Policy` on the next successful deploy of that artifact (T1-05).
- **Not written** by a dry run or by a deploy that fails before the container is
  started.
- **Removed** whole by teardown (`RemoveContainer`) and by `delete application`;
  nothing of the record outlives the deployment.

## 4. Backend capture (`internal/engine/local/dockercontainer/docker_container.go`)

```text
CreateContainer(op)
  record := newDeploymentRecord(op)          // Kind, Name, Image (as written), Policy
  expanded := expandRegistryAlias(op.Image)  // unchanged
  op.Image = expanded                         // unchanged
  digest := resolveImage(...)                 // unchanged
  record.ConfigHash = configHash(op, digest)  // hash inputs unchanged (TD-2)
  ... inspect, up-to-date check against record.ConfigHash ...
  ensureRunning(..., record) | createFreshContainer(..., record)
     → recordDeployment(op.Team, record, containerID)
```

## 5. Handler view (`internal/handler/deployments.go`)

No new type. `teamedDeployment{Team, Deployment}` already carries the record;
the formatters read `Deployment.Image` and `Deployment.Policy` and substitute `-`
for an empty value through `valueOrUnknown`.

| Surface | Reads | Shows |
|---|---|---|
| `get deployed` / `get applications` / `get resources` | `Image` | VERSION column after KIND |
| `describe app` / `describe resource` | `Image`, `Policy` | `Image:` and `Pull policy:` lines after `Kind:` |
