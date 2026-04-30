# Phase 0 Research: Preserve Operator-Edited traefik.yml

The spec is fully clarified. The Technical Context above carries no `NEEDS CLARIFICATION` markers. This document records the small set of design choices that were genuinely open after the spec, with rationale for each.

---

## Decision 1: Existence probe uses `os.Lstat`, not `os.Stat`

**Decision**: Probe `traefik.yml` via `os.Lstat` and treat any non-`os.IsNotExist` outcome as "present, do not write."

**Rationale**:
- FR-009 requires that the existence check detect any entry — including symlinks, regardless of whether their target exists. `os.Stat` follows symlinks and returns `ENOENT` for a broken symlink, which would cause Shrine to "regenerate" by writing through the link to its missing target — exactly the silent-overwrite failure mode this feature exists to prevent.
- `os.Lstat` does not follow links, so a broken symlink returns a successful `FileInfo` and the probe correctly reports "present."
- Non-regular files (directory, device, socket, named pipe) are likewise reported as present by `os.Lstat` — matches the second clarification: "anything at the `traefik.yml` path is operator-owned regardless of type."

**Alternatives considered**:
- `os.Stat`: simpler, but breaks the broken-symlink case (would write through), violating FR-009 and the first clarification.
- Two-phase probe (`Lstat` then `Stat`): unnecessary; we never need to distinguish symlink-with-good-target from regular-file, because the action is the same in both cases.

---

## Decision 2: Existence check fail-closed on stat errors other than NotExist

**Decision**: If `os.Lstat` returns an error and `os.IsNotExist(err)` is false (e.g., permission denied, parent missing for an unrelated reason, I/O error), `Plugin.Deploy()` returns the error wrapped as `traefik plugin: checking traefik.yml: <cause>`. Shrine MUST NOT fall back to overwriting.

**Rationale**: Direct codification of FR-007. Keeps the failure mode loud — an operator who hits a permission issue sees it immediately on the next deploy instead of silently losing edits because Shrine misread the error as "file is missing, regenerate."

**Alternatives considered**:
- Treat any stat error as "missing, regenerate": simplest code, but indistinguishable from the bug being fixed.
- Treat any stat error as "present, skip": hides real configuration problems (e.g., `traefik.yml` owned by another user) that the operator needs to know about.

---

## Decision 3: User-visible signal is an `engine.Observer` event, not a direct `fmt.Fprintln`

**Decision**: The plugin emits `gateway.config.preserved` (when the file already exists and the write is skipped) or `gateway.config.generated` (when the file is absent and the default is written) as `engine.StatusInfo` events through an `engine.Observer` injected at `traefik.New(...)`. `internal/ui/terminal_logger.go` gains two `case` branches that render them with the same emoji-prefixed line format used for other deploy steps. `internal/ui/file_logger.go` requires no changes — it persists every event by name.

**Rationale**:
- Matches FR-006 ("info-level observable signal") and SC-004 ("log clearly indicates… generated or preserved") without inventing a new logging path.
- Other deploy-step signals (container.create, network.ensure, routing.configure) already flow through `engine.Observer`; routing the gateway-config signal the same way keeps the deploy log internally consistent and the unit-test surface the same (assert events via a fake observer).
- The plugin already takes a `containerBackend` constructed with the observer baked in; passing the observer directly is a one-parameter constructor extension.

**Alternatives considered**:
- `fmt.Fprintln(os.Stdout, ...)` inside `config_gen.go`: simpler, but bypasses the existing observer pipeline, breaks `--dry-run`'s structured-output expectations, and makes the unit test for the signal awkward (would need to capture stdout).
- Plumb a separate `*log.Logger`: redundant with the observer; introduces a second logging surface where one is sufficient.

---

## Decision 4: The existence probe is extracted as a small named helper

**Decision**: A private helper `isStaticConfigPresent(routingDir string) (present bool, err error)` lives in `config_gen.go`. `generateStaticConfig` calls it before `os.WriteFile`; if `present` is true, the function returns early after the caller emits the `preserved` event; if `err` is non-nil, the function propagates it.

**Rationale**: Constitution Principle VII requires boolean methods to start with `is`/`has`/`should` and demands self-documenting names. `isStaticConfigPresent` makes the WHY (operator-owned-on-exists policy) readable at the call site without comments. The helper is also the natural unit-test seam — tests can drop a regular file, a symlink to nowhere, a directory, or a 0o000-mode parent into a `t.TempDir()` and verify the boolean and error contract directly.

**Alternatives considered**:
- Inline `os.Lstat` at the call site: shorter (3 lines), but the existence-check policy is the conceptual heart of this feature; giving it a name is worth the indirection.
- Helper on `*Plugin`: unnecessary — the helper takes a path, not plugin state.

---

## Decision 5: Test surface — integration scenarios extend the existing suite, units cover the helper

**Decision**:
- **Integration** (canonical gate per Principle V): four new scenarios added to `tests/integration/traefik_plugin_test.go`, all using `NewDockerSuite`:
  1. **Preserve on redeploy (US1, AC1+AC2)**: deploy → mutate `traefik.yml` → redeploy → assert byte-for-byte identical content; container still running.
  2. **Regenerate after delete (US3)**: deploy → delete `traefik.yml` → redeploy → assert default content present; container still running.
  3. **Symlink (broken target)**: pre-create `traefik.yml` as a symlink whose target does not exist → deploy → assert symlink unchanged, target still does not exist; deploy succeeds (gateway container may fail health check; out of scope for this assertion — we assert Shrine's behavior, not Traefik's).
  4. **Non-regular file (directory)**: pre-create `traefik.yml` as a directory → deploy → assert directory still exists, no regular file at that path; Shrine deploy command exits 0 (the gateway container surface failures are not Shrine's responsibility).
- **Unit**: a new `config_gen_test.go` in `internal/plugins/gateway/traefik/` covers `isStaticConfigPresent` and `generateStaticConfig`'s skip-vs-write branches against a fake observer. Per the saved feedback memory ("unit tests must not touch the filesystem"), unit tests use `t.Setenv` and an in-memory or fake `Statter` shim — *not* `t.TempDir()` writes. The branch-on-existence logic is tested by injecting a fake stat function (function-typed package var, swapped per test).

**Rationale**: Mirrors the existing integration coverage for the `dynamic/` preserve policy ("should preserve operator-added files in the dynamic directory" already in the suite). The unit-test approach respects the project memory: filesystem touches belong in `tests/integration/`, not in `internal/.../*_test.go`.

**Alternatives considered**:
- Drop unit tests, rely on integration only: leaner, but the four-way fork in `isStaticConfigPresent` (regular file, symlink-broken, directory, NotExist) deserves fast feedback that doesn't require Docker.
- Use `t.TempDir()` in unit tests: violates the saved feedback memory (and the integration suite already covers filesystem reality).

---

## Decision 6: No backwards-compatibility shims, no migration

**Decision**: Ship the change directly. Operators who have not edited `traefik.yml` see no difference (file is regenerated to the same default content; or, on the first post-fix redeploy, preserved as-is — content is byte-equal). Operators who *have* edited it stop losing their edits starting with this release.

**Rationale**: Spec's Backwards Compatibility assumption ("the change is transparent for first-time deploys… and for deploys against hosts where the operator has not edited the file. There is no migration step.") This is a behavior fix, not a contract break.

**Alternatives considered**:
- Feature-flag via env var (e.g., `SHRINE_PRESERVE_TRAEFIK_YML=1`): explicitly forbidden by FR-005 and SC-003.
- One-time "snapshot then preserve" migration: unnecessary — the operator's current `traefik.yml` is already what they want preserved. There is nothing to migrate.

---

## Open questions

None. Spec is fully clarified, all design choices above are independently grounded in the spec or constitution.
