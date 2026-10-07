# Data Model: `shrine delete resource` and pin release on every delete

**Feature**: 037-delete-resource | **Date**: 2026-10-07

No recorded state changes shape: `deployments.txt`, `pins.txt`, and `hostports.txt` keep their formats, and no store interface gains a method. The change is in the handler's values: one options type renamed and shared, one kind parameter threaded through the delete path, and one guard on the pin read.

## 1. Delete options (`internal/handler`) — RENAMED

```go
// DeleteOptions parameterizes DeleteApplication and DeleteResource. Team is
// optional (all teams are searched, ambiguity is an error). DryRun prints
// what would be released without writing.
type DeleteOptions struct {
	Name   string
	Team   string
	DryRun bool
}
```

Replaces `DeleteApplicationOptions` (same three fields). Callers: `cmd/delete.go`, `internal/handler/deployments_test.go`.

## 2. Held state, per kind (`internal/handler`, inside `deleteArtifact`)

The values `deleteArtifact` reads before deciding, in order:

| Value | Read from | Application | Resource |
|---|---|---|---|
| `team` | `resolveDeleteTeam(store, kind, name, opts.Team)` | candidates from host ports, records of kind, pins of kind | candidates from records of kind, pins of kind |
| container exists | `container.InspectContainer(team + "." + name)` returns no error | refusal | refusal |
| `port, hasPort` | `store.HostPorts.GetHostPort(team, name)` | read | never read; `hasPort` is false |
| `pin, hasPin` | `findImagePin(store, team, kind, name)`: `ImagePins.Get` and `pin.Kind == kind` | read | read |
| `record` | `findDeploymentRecord(store, team, kind, name)`: a record with that name and kind in `Deployments.List(team)` | read | read |
| `nothingHeld` | `!hasPort && !hasPin && !record` | | |

Release order, unchanged from `DeleteApplication`: host port (applications only), pin, record. Each step prints one line; a failure returns at that step with the earlier releases kept.

## 3. Kind word in messages

`kindWord := strings.ToLower(kind)` gives `application` or `resource` from `manifest.ApplicationKind` / `manifest.ResourceKind`, used in every message that names the kind (refusal, nothing-to-delete, ambiguity). The `<team>/<name>` form and the `<team>.<name>` container name are kind-independent.

## 4. Pin record (`internal/state`) — UNCHANGED, one invariant made explicit

`ImagePin.Kind` is `Application` or `Resource`. The key `team/name` carries no kind, so the delete path reads a pin as the artifact's only when `pin.Kind` equals the requested kind, the rule `resolveDeleteTeam` (listing) and the queries (`isPinFor`) already apply. One pin, one record, and one container per `team/name` means the guard is a consistency check rather than a reachable branch (research R3).

## 5. Command (`cmd/delete.go`) — EXTENDED

```text
delete
├── team [name]                              (unchanged)
├── application [name] [-t team] [--dry-run] (unchanged surface; RunE through runDelete)
└── resource    [name] [-t team] [--dry-run] NEW
```

Per-subcommand flag variables (`deleteAppTeam`, `deleteAppDryRun`, `deleteResTeam`, `deleteResDryRun`); `runDelete(kind, team *string, dryRun *bool)` builds `app.NewQueryContainerBackend(cfg, store)` and dispatches by kind.

## 6. Integration world (`tests/integration`) — REUSED

`newPinnedSuite` (from `pinned_image_policy_test.go`, same package) provides: a loopback registry with `shrine/whoami:latest`, a manifest directory with `whoami-pinned` (Application) and `cache-pinned` (Resource) under `Pinned`, and helpers `pinnedDeploy`, `pinnedTeardown`, `pinsPath`, `deploymentsPath`, `readFileOrEmpty`, `nonEmptyLines`. `TestDeleteResource` in `delete_test.go` uses them; no new fixture directory.
