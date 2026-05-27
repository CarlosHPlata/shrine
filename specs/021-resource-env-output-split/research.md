# Phase 0 Research: Split Resource `env` and `output` (SRP)

This phase resolves the open decisions deferred from `/speckit-clarify` and the
unknowns surfaced while mapping the change against the current code.

## Decision 1 — Generated-secret stability across the output→env move

**Decision**: Keep the secret-store key as `<resource-name>.<var-name>`. When an
operator moves a `generated` output to `env`, they keep the same variable name,
so the key is unchanged and the already-generated secret is reused — no
rotation.

**Rationale**: Today the key is `res.Metadata.Name + "." + output.Name`
(`internal/resolver/resolver.go`, `ResolveResource`). The new env-level
generation uses `res.Metadata.Name + "." + env.Name`. Migration guidance is
"declare the same name under `env`", which makes the key identical. This is what
makes SC-005 ("no change to the resolved container environment") true for the
common migration.

**Alternatives considered**:
- Re-key generated secrets by a new scheme (e.g. include a block discriminator):
  rejected — it would silently rotate every existing generated secret on
  upgrade, breaking running resources and contradicting SC-005.

## Decision 2 — Reserved built-in names in resource `env`

**Decision**: `host`, `port`, `team`, and `name` are reserved built-ins. A
resource `env` entry MUST NOT use one of these as its `name`; doing so is a
validation error.

**Rationale**: Output entries and templates resolve `host`/`port` from the CLI
built-ins and `team`/`name` from metadata. Allowing an env var named `host`
would make `output: [{name: host}]` and `{{.host}}` ambiguous (env value vs.
built-in). Reserving the four names keeps every `{{.X}}` and every non-template
output unambiguous with one simple rule.

**Alternatives considered**:
- "Env wins over built-in": rejected — surprising, and an operator who shadows
  `host` likely made a mistake; a hard error is clearer.
- Reserve only `host`/`port`: rejected — `team`/`name` are equally referenceable
  in templates; reserving all four is the simplest consistent rule.

## Decision 3 — Output template referenceable names

**Decision**: An `output` `template` may reference (a) any declared resource
`env` var name — exported or not — and (b) the built-ins `host`, `port`,
`team`, `name`. It may NOT reference other `output` entries. Referencing any
other name is a validation error.

**Rationale**: Matches the Q2 clarification (templates may fold private env vars
into an exported value) and keeps the render context a flat map of "resolved env
+ built-ins", identical in spirit to the Application env render context. Banning
output→output references avoids a second topological layer; the natural data
source for a connection string is the env vars, which are already fully resolved
before exports are computed.

**Alternatives considered**:
- Allow output→output references (the old sibling-output behaviour): rejected —
  unnecessary for the connection-string use case and would re-add a topo sort
  over outputs. Can be added later if a real need appears (YAGNI).

## Decision 4 — Resolver contract: env vs exports

**Decision**: `Resolver.ResolveResource(res, deps) (ResolvedResource, error)`
where `ResolvedResource{ Env, Exports map[string]string }`. `Env` is the fully
resolved environment fed to the container; `Exports` is the export allowlist fed
to consumers (`deps.Resources[name]`).

**Rationale**: The container env and the consumer interface are now genuinely
different sets (the whole point of the split). One map cannot represent both
without the engine re-deriving the allowlist — which would push manifest policy
into engine core. Two named fields make the contract self-documenting and let
the strict allowlist be enforced simply by what lands in `Exports`.

**Alternatives considered**:
- Two methods (`ResolveResourceEnv`, `ResolveResourceExports`): rejected —
  doubles secret-store/vault work and risks divergent resolution between the two
  calls.
- Keep `map[string]string` and tag built-ins: rejected — that is exactly the
  current conflation.

## Decision 5 — Resource as first-class consumer (FR-014) reuse strategy

**Decision**: Generalize feature-020's `applyEnrichmentRule` so it scans
**both** Applications and Resources as consumers, add a `cloneResourceWithDeps`
mirror of `cloneApplicationWithDeps`, and have `planner.Order` read
`res.Spec.Dependencies`. The engine resolves resources in the topological order
already produced by `Order`, populating `deps.Resources[name]` with each
resource's `Exports` as it goes.

**Rationale**: Resources-as-consumers is the same problem the enrichment +
ordering + access/reachability machinery already solves for Applications; the
honest move is to widen the existing loop, not fork a parallel one (Principle
VII / DRY). `copyManifestSetShallow` already clones the resource map, so
copy-on-write at `Spec.Dependencies` extends cleanly.

**Alternatives considered**:
- Restrict resource env `valueFrom` to vault only (the rejected Q1 option A):
  out of scope per the operator's Q1 = B choice.
- A separate resource-only enrichment rule: rejected — duplicates the loop and
  the cross-team/absent fail-fast logic.

## Decision 6 — Old-manifest rejection mechanics (FR-011)

**Decision**: Retain the `Value`, `Generated`, and `ValueFrom` fields on the
`Output` struct so old YAML still unmarshals, and have `validateResourceSpec`
emit a clear error naming the resource and output when any of them is set,
directing the operator to move the value to `env`.

**Rationale**: Go's YAML unmarshal silently drops unknown fields; if the fields
were removed, an old `generated: true` output would be parsed as a bare name and
fail confusingly ("not a recognized built-in") instead of with actionable
migration guidance. Keeping the fields purely for detection follows the existing
precedent of `rejectTLSOutsideAliasEntries` in `parser.go`.

**Alternatives considered**:
- Remove the fields: rejected — silent/confusing failure, poor migration UX.
- Strict-unmarshal (`KnownFields(true)`): rejected — broad blast radius across
  all manifest kinds, out of scope.

## Decision 7 — Dry-run representation (FR-013)

**Decision**: `DryRunResolver.ResolveResource` returns the same
`ResolvedResource{Env, Exports}` shape with placeholder values
(`[GENERATED]`, `[VAULT:...]`, `[PORT]`) for `Env`, then computes `Exports` from
the output list over those placeholders. The dry-run output distinguishes the
resource's container environment (`Env`) from its published interface
(`Exports`).

**Rationale**: Preserves the no-side-effects guarantee (no secret generation, no
vault reads) while making the split visible in the plan, satisfying FR-013.

## Best-practice notes

- **Multi-error validation** (Principle I): env and output validation must
  accumulate into the existing `[]string` error slice in `validateResourceSpec`;
  do not return on the first offending entry.
- **Shared env resolution**: the env-resolution switch
  (`value` / `valueFrom` / `template` / `generated`) is common to Applications
  (minus `generated`) and Resources; extract a shared helper so the two callers
  do not duplicate the switch (Principle VII).
- **Determinism**: iterate consumers in sorted-by-name order in the enrichment
  loop (already done for Applications) and resolve resources in `Order` sequence
  so error reporting and edge provenance are reproducible.
