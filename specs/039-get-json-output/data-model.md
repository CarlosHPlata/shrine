# Data Model: Machine-Readable Output for `shrine get`

**Feature**: 039-get-json-output | **Date**: 2026-10-11

No stored data changes. This feature adds four output-only types in
`internal/handler`, built from the existing `state.Deployment`,
`state.ImagePin`, `manifest.TeamManifest` and `engine.ContainerInfo`.

## Listing envelope

`jsonListing[T]`: the top-level document for every `--json` listing.

| Field | JSON key | Type | Rule |
|-------|----------|------|------|
| Data | `data` | array of `T` | Never `null`; `[]` when nothing is listed (FR-008). Order equals the table's row order (FR-005). |

## Workload entry (`deploymentEntry`)

One per row of `get deployed`, `get applications` or `get resources`.

| JSON key | Type | Source | Rule |
|----------|------|--------|------|
| `team` | string | `teamedDeployment.Team` | As the table's TEAM. |
| `name` | string | `Deployment.Name` | As the table's NAME. |
| `kind` | string | `Deployment.Kind` | `Application` or `Resource`. |
| `image` | string | `Deployment.Image` | The reference the manifest named; `""` for a legacy record (where `describe` prints `-`). |
| `pullPolicy` | string | `Deployment.Policy` | `Always`, `IfNotPresent`, `Pinned`; `""` for a legacy record. |
| `version` | string | `versionOf(team, d, pins)` | Readable pin form (e.g. `v1.11.0@4f90b33ddca9`) when `pinOf` finds a pin, else `Deployment.Image`; `""` when that is empty. The table prints `valueOrUnknown` of the same value. |
| `pin` | object or `null` | `pinOf(team, d, pins)` | Object when the record's policy is `Pinned` and a pin of the same kind and name is on record; `null` otherwise. |
| `runningImage` | string or `null` | `runningImageOrNull(backend, Deployment.ContainerID)` | The image the runtime reports for the container; `null` when there is no backend, no container ID, or the inspect fails (FR-003a). |
| `containerId` | string | `Deployment.ContainerID` | The full ID, not shortened; `""` when no container has been recorded. |
| `configHash` | string | `Deployment.ConfigHash` | The full hash, not the 16-character preview; `""` for a record from before config hashes. |

## Pin (`pinEntry`)

| JSON key | Type | Source | Rule |
|----------|------|--------|------|
| `reference` | string | `ImagePin.Pinned` | The exact pullable reference, `…@sha256:<64 hex>`. |
| `version` | string | `readablePin(pin)` | Same string the table's VERSION and `describe`'s `Pinned:` parenthesis show. |
| `pinnedAt` | string | `ImagePin.PinnedAt` | `UTC().Format(time.RFC3339)`, e.g. `2026-10-11T14:03:22Z`. |

## Team entry (`teamEntry`)

One per row of `get teams`. No change from the original spec (clarification Q3).

| JSON key | Type | Source | Rule |
|----------|------|--------|------|
| `name` | string | `Metadata.Name` | As the table's NAME. |
| `displayName` | string | `Spec.DisplayName` | `""` when not set. |
| `maxApps` | number | `Spec.Quotas.MaxApps` | Integer, `0` when not set. |
| `maxResources` | number | `Spec.Quotas.MaxResources` | Integer, `0` when not set. |

## Helpers that derive the values

| Helper | Signature | Shared by |
|--------|-----------|-----------|
| `listDeployments` | `(team, kind string, store *state.Store) ([]teamedDeployment, error)` | All three table and all three JSON list functions; empty `kind` lists every kind. |
| `pinOf` | `(team string, d state.Deployment, pins map[string]state.ImagePin) (state.ImagePin, bool)` | `versionOf` and the JSON `pin`. Wraps the existing `pinFor` with the `Pinned` policy check `versionCell` makes today. |
| `versionOf` | `(team string, d state.Deployment, pins map[string]state.ImagePin) string` | `versionCell` (table) and the JSON `version`. |
| `inspectRunningImage` | `(backend engine.ContainerBackend, containerID string) (string, error)` | `runningImage` (describe's display line, which prints the error) and `runningImageOrNull` (JSON, which maps any error, and an empty container ID without a call, to `null`). A nil backend yields `errNoContainerRuntime`. |
| `encodeListing` | `[T any](entries []T) ([]byte, error)` | Workload and team JSON formatters. |

No state transitions: every value is read, nothing is written.
