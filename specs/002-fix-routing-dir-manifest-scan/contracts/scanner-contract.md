# Contract: Manifest Directory Scanner

This document is the behavioural contract that every shrine command which scans a directory for manifests MUST satisfy after this fix lands. It is the source of truth for the assertions made by the unit tests of `internal/manifest/classify.go` and `internal/manifest/scan.go`, and for the integration tests that exercise `shrine deploy` / `shrine apply teams` against a fixture containing foreign YAML files.

There is no over-the-wire API to spec — shrine is a single-binary CLI — so the contract is expressed at two layers:

1. **Internal Go API** that every directory-scanning command MUST go through (`manifest.ScanDir`).
2. **CLI behaviour** observable by an operator running shrine commands.

---

## 1. Internal Go API contract — `manifest.ScanDir`

### Signature

```go
func ScanDir(dir string) (*ScanResult, error)
```

### Inputs

| Parameter | Constraint |
|-----------|------------|
| `dir` | Existing readable directory. If `dir` does not exist or is unreadable, the underlying `filepath.WalkDir` error is returned wrapped with `"scanning manifest directory %q"`. |

### Outputs

| Result field | Guarantee |
|--------------|-----------|
| `*ScanResult.Shrine` | Slice of `ShrineCandidate` — every `.yaml`/`.yml` file under `dir` whose `apiVersion` matched `^shrine/v\d+([a-z]+\d+)?$`. Order is the deterministic walk order returned by `filepath.WalkDir`. |
| `*ScanResult.Foreign` | Slice of `string` paths — every `.yaml`/`.yml` file under `dir` whose `apiVersion` did NOT match the regex (including absent / empty values). Order matches walk order. |
| `error` | `nil` if every admitted file was either Shrine or Foreign. Non-nil only when an admitted (.yaml/.yml) file fails `yaml.Unmarshal` — error message MUST contain the file path and the underlying parser message. |

### Invariants (MUST hold for every call)

1. **Extension filter never opens disallowed files.** `ScanDir` MUST NOT call `os.ReadFile` on any path whose extension is not exactly `.yaml` or `.yml`. (Verified by a unit test that places an unreadable `.json` file in the tree and asserts no error.)
2. **Foreign and Shrine are disjoint.** No file path may appear in both result slices.
3. **No silent loss.** Every `.yaml`/`.yml` file under `dir` either appears in `Shrine`, appears in `Foreign`, or causes the entire call to return an error. There is no third skip path.
4. **Non-mutating.** `ScanDir` MUST NOT create, modify, or delete any filesystem entry under `dir`.
5. **Deterministic.** Two calls to `ScanDir` on the same unchanged directory MUST return the same slices in the same order.

### Error contract

| Condition | Behaviour |
|-----------|-----------|
| `dir` does not exist | wrapped error from `filepath.WalkDir` (existing behaviour preserved) |
| `.yaml`/`.yml` file fails `yaml.Unmarshal` | wrapped error containing the file path and the parse-error message; scan aborts |
| `.yaml`/`.yml` file has no `apiVersion` field at all | classified as Foreign — no error |
| `.yaml`/`.yml` file has `apiVersion: ""` | classified as Foreign — no error |
| `.yaml`/`.yml` file has `apiVersion: traefik.io/...` (or any non-matching value) | classified as Foreign — no error |
| `.yaml`/`.yml` file has `apiVersion: shrine/v1` and any `kind` (including invalid) | classified as Shrine — no error from `ScanDir`; the caller's `Parse`/`Validate` produces the loud error per FR-003 |

---

## 2. CLI behaviour contract

These commands MUST satisfy the rules below for every project layout:

- `shrine deploy --path <dir>`
- `shrine apply -f <file>` and `shrine apply teams --path <dir>`
- `shrine generate <...>` (if it scans `specsDir`)
- Any future verb that scans for manifests

### Behaviour matrix

| Project layout | `shrine deploy` exit code | stdout | stderr |
|----------------|---------------------------|--------|--------|
| `specsDir` contains only valid shrine manifests | `0` | unchanged from today (per-step `[APPLY]` lines etc.) | empty |
| `specsDir` contains valid shrine manifests + foreign YAML (e.g. Traefik routing files under `specsDir/traefik/`) | `0` | unchanged steps PLUS one informational notice naming the foreign paths (FR-006) | empty |
| `specsDir` contains valid shrine manifests + non-YAML siblings (`.json`, `.md`, `Makefile`) | `0` | unchanged from today — non-YAML siblings produce no notice (FR-006) | empty |
| `specsDir` contains a `.yaml` file with `apiVersion: shrine/v1` and a typo'd / missing `kind` | non-zero | unchanged successful steps up to the failure | error message identifying the file and the offending kind value (FR-003) |
| `specsDir` contains a `.yaml` file that cannot be parsed as YAML | non-zero | unchanged successful steps up to the failure | error message identifying the file and the parse error (FR-004) |
| `specsDir` contains a `.yaml` file with `apiVersion: Shrine/v1` (capital S typo) | `0` (file is Foreign and skipped) | one informational notice listing the file as foreign | empty (deliberate trade-off, see spec §Assumptions) |
| Default Traefik plugin enabled, `routing-dir` unset → defaults to `specsDir/traefik/` | `0` | normal deploy output PLUS foreign-files notice listing the generated Traefik routing files | empty (this is SC-001) |

### What the contract DOES NOT promise

- No new exit codes are introduced.
- No new flags are added.
- The exact wording of the foreign-files notice is not stable — operators MUST NOT parse it (FR-006: "downstream tooling should not parse it").
- Multi-document YAML support — explicitly out of scope (spec §Assumptions).

---

## 3. Unit test obligations (`internal/manifest/classify_test.go`)

The following table-driven tests MUST exist and pass:

| Case | Input file content | Expected result |
|------|--------------------|-----------------|
| Strict shrine v1 | `apiVersion: shrine/v1\nkind: Application\n...` | `ClassShrine`, `TypeMeta{APIVersion: "shrine/v1", Kind: "Application"}` |
| Strict shrine v1beta1 | `apiVersion: shrine/v1beta1\nkind: Resource\n...` | `ClassShrine` |
| Strict shrine v10alpha7 | `apiVersion: shrine/v10alpha7\nkind: Team\n...` | `ClassShrine` |
| Capital S typo | `apiVersion: Shrine/v1\n...` | `ClassForeign` |
| Plural typo | `apiVersion: shrines/v1\n...` | `ClassForeign` |
| No version suffix | `apiVersion: shrine/dev\n...` | `ClassForeign` |
| Trailing space (YAML scalar trim) | `apiVersion: "shrine/v1 "\n...` | `ClassForeign` (space inside quoted scalar fails the `$` anchor) |
| Empty apiVersion | `apiVersion: ""\n...` | `ClassForeign` |
| Missing apiVersion | `kind: Application\n...` | `ClassForeign` |
| Foreign apiVersion | `apiVersion: traefik.containo.us/v1alpha1\n...` | `ClassForeign` |
| Empty file | `` | `ClassForeign` (no apiVersion to inspect) |
| Comments only | `# nothing here` | `ClassForeign` |
| Malformed YAML | `apiVersion: shrine/v1\nkind: [unclosed` | error containing the file path |
| Shrine v1 with bogus kind | `apiVersion: shrine/v1\nkind: Aplication\n...` | `ClassShrine` (Classify does not check kind; Parse will fail loudly later) |

Plus assertions on `ScanDir`:

| Case | Expected |
|------|----------|
| Empty directory | `&ScanResult{}, nil` |
| Directory with only `.json` / `.md` / extensionless files | `&ScanResult{}, nil` (no reads) |
| Directory with one valid + one foreign | `Shrine` len 1, `Foreign` len 1, no error |
| Directory with one malformed YAML | error wrapping the file path |
| Nested subdir containing foreign YAML (mirrors `specsDir/traefik/`) | foreign path collected; no error |

---

## 4. Integration test obligations (Principle V gate)

The fix is not "done" until BOTH of the following pass under `make test-integration`:

| Test | Location | Scenario |
|------|----------|----------|
| `TestDeploy/should_deploy_successfully_when_specsDir_contains_foreign_YAML_files` | [tests/integration/deploy_test.go](../../../tests/integration/deploy_test.go) | Deploy a fixture with `team.yaml` + `app.yaml` + `traefik/traefik.yml` (no apiVersion); assert exit `0`, the app container running, and no container created from the foreign file. |
| `TestTraefikPlugin/should_succeed_when_routing-dir_is_inside_specsDir` | [tests/integration/traefik_plugin_test.go](../../../tests/integration/traefik_plugin_test.go) | Configure the Traefik plugin with default `routing-dir = {specsDir}/traefik`; run `shrine deploy` twice (first to generate Traefik files, second to re-scan a populated tree); assert both runs exit `0` and the Traefik container is running. This is SC-001's canonical regression. |

Both tests use `NewDockerSuite` with the real shrine binary built in `TestMain` per Principle V.
