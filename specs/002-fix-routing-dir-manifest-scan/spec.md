# Feature Specification: Fix Routing-Dir Manifest Scan Crash

**Feature Branch**: `002-fix-routing-dir-manifest-scan`  
**Created**: 2026-04-30  
**Status**: Draft  
**Input**: User description: "Bug: shrine deploy crashes when routing-dir is inside specsDir. When routing-dir is a subdirectory of specsDir, Shrine's manifest scanner picks up Traefik YAML files and tries to parse them as Shrine manifests, causing a crash. The scanner should skip files that are not valid Shrine manifests (missing apiVersion/kind) instead of crashing."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Deploy Succeeds With Default routing-dir Layout (Priority: P1)

A shrine operator enables the Traefik gateway plugin and accepts the default `routing-dir` (which lives inside `specsDir` at `{specsDir}/traefik/`). When they run `shrine deploy`, the deploy completes successfully even though the Traefik directory contains generated YAML files that are not Shrine manifests.

**Why this priority**: This is the default, documented configuration for the Traefik plugin. Today it crashes the deploy outright, blocking the most common path for any operator who turns the plugin on.

**Independent Test**: Configure the Traefik plugin with default settings, generate Traefik routing files into `{specsDir}/traefik/`, run `shrine deploy`, and confirm the command succeeds without errors and applies all valid Shrine manifests.

**Acceptance Scenarios**:

1. **Given** `routing-dir` is a subdirectory of `specsDir` and contains Traefik YAML files (whose `apiVersion` does not match the shrine pattern), **When** the operator runs `shrine deploy`, **Then** the deploy parses and applies only valid Shrine manifests and ignores the Traefik files without raising an error.
2. **Given** `routing-dir` lives outside `specsDir`, **When** the operator runs `shrine deploy`, **Then** behavior is unchanged from today: only files under `specsDir` are scanned, and all valid Shrine manifests are applied.
3. **Given** `specsDir` contains only valid Shrine manifests (each with `apiVersion: shrine/v1`), **When** the operator runs `shrine deploy`, **Then** every manifest is parsed and applied exactly as before — there is no regression in detection of legitimate manifests.

---

### User Story 2 - Foreign YAML Files Coexist In specsDir (Priority: P2)

A shrine operator keeps unrelated YAML files inside `specsDir` (for example, an editor settings file, a CI snippet, or another tool's config). When they run any shrine command that scans `specsDir` for manifests, those foreign files are skipped silently and shrine continues with the manifests it understands.

**Why this priority**: Generalises the fix beyond Traefik. Any tool whose YAML lives alongside shrine manifests benefits from the same skip behaviour, and it avoids a long tail of one-off bug reports for each new collocated tool.

**Independent Test**: Place a foreign YAML file (with no `apiVersion`, or an `apiVersion` that does not match the shrine pattern) in `specsDir`, run a shrine command that triggers a manifest scan, and confirm the command succeeds and processes only the legitimate manifests.

**Acceptance Scenarios**:

1. **Given** `specsDir` contains a YAML file with no `apiVersion` field at all, **When** any shrine command scans `specsDir`, **Then** that file is skipped and the command proceeds.
2. **Given** `specsDir` contains a YAML file whose `apiVersion` does not match the shrine pattern (e.g., `traefik.containo.us/v1alpha1`, `apps/v1`, or even `shrine/dev`), **When** shrine scans the directory, **Then** the file is skipped silently — only the strict shrine pattern qualifies a file as a shrine manifest.

---

### User Story 3 - Genuinely Broken Shrine Manifests Still Fail Loudly (Priority: P2)

A shrine operator has a YAML file whose `apiVersion` matches the shrine pattern (e.g., `shrine/v1`) but whose `kind` is missing, misspelled, or the spec body is invalid. When they run `shrine deploy`, the command fails with a clear error pointing to that file — the "skip foreign files" rule does not mask authoring mistakes once a file has self-identified as a shrine manifest.

**Why this priority**: Without this guard, the fix could silently swallow typos like `kind: Aplication` in a `shrine/v1` document and let a broken project deploy partially. Operators must still see real errors when intent is clear.

**Independent Test**: Place a YAML file with `apiVersion: shrine/v1` and a `kind` value that is missing or non-empty but unrecognised (e.g., `Aplication`) in `specsDir`, run `shrine deploy`, and confirm the command fails with an error that names the file and the offending kind.

**Acceptance Scenarios**:

1. **Given** a YAML file whose `apiVersion` matches the shrine pattern and whose `kind` is missing, empty, or non-empty but unrecognised by shrine, **When** shrine scans `specsDir`, **Then** the command fails with an error identifying the file and the offending kind value.
2. **Given** a YAML file whose `apiVersion` matches the shrine pattern and whose `kind` is recognised but whose body fails validation (e.g., missing required fields), **When** shrine scans `specsDir`, **Then** the command fails with the existing validation error for that manifest — behaviour unchanged from today.

---

### Edge Cases

- A file in the scanned tree has no `.yaml` or `.yml` extension (e.g., `.gitignore`, `README.md`, `Makefile`, `config.json`, `app.toml`, an extension-less binary, or a `*.tmpl`/`*.bak`) → not opened or read by the scanner; skipped silently regardless of contents.
- A YAML file is empty or contains only comments → passes the extension filter, has no `apiVersion`, classified as foreign, skipped.
- A YAML file is malformed (cannot be parsed as YAML at all) → passes the extension filter, fails loudly with an error identifying the file and the parse error; this is treated as an authoring mistake, not a foreign file, because shrine cannot tell intent from unparseable bytes.
- A YAML file's `apiVersion` is `shrine/v1` but the value is mistyped as `Shrine/v1`, `shrines/v1`, or `shrine/dev` → does not match the strict shrine regex, so the file is skipped silently. Operators are expected to detect this through "manifest not applied" rather than a parser error.
- A YAML file contains multiple documents separated by `---` → the existing single-document behaviour is preserved; multi-document support is out of scope for this fix.
- A non-YAML payload stored in a file with a `.yml` or `.yaml` extension (e.g., a templating placeholder) → passes the extension filter; if the bytes parse as YAML and the `apiVersion` does not match the shrine pattern, it is skipped; if they do not parse as YAML, it errors per the rule above.
- `routing-dir` is a symlink pointing inside `specsDir` → treated the same as a real subdirectory; foreign files inside it are skipped.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The manifest scanner MUST process candidates in two ordered steps:
  - **(a) Extension filter** — only files whose extension is exactly `.yaml` or `.yml` (case-sensitive, matching the existing convention) are admitted as candidates. Files with any other extension, or no extension at all, MUST be skipped silently; the scanner does not open or read them.
  - **(b) apiVersion classification** — for each admitted candidate, the scanner MUST inspect the top-level `apiVersion` field FIRST, and MUST treat the file as a shrine manifest only when `apiVersion` matches the regex `^shrine/v\d+([a-z]+\d+)?$` (e.g., `shrine/v1`, `shrine/v1beta1`, `shrine/v2`). All admitted candidates that fail this check (no `apiVersion`, empty value, or any non-matching value) MUST be classified as foreign.
- **FR-002**: When a file is skipped by FR-001(a) or classified as foreign by FR-001(b), shrine MUST NOT raise an error, MUST NOT include it in the manifest set, and MUST continue processing remaining files.
- **FR-003**: When a file's `apiVersion` matches the shrine regex, shrine MUST then validate `kind`: if `kind` is missing, empty, or non-empty but not a kind shrine recognises, shrine MUST fail loudly with an error that identifies the file path and the offending kind value. Files self-identified as shrine manifests must not be silently skipped on any kind-related defect.
- **FR-004**: The manifest scanner MUST fail loudly on any file admitted by FR-001(a) that cannot be parsed as YAML, with an error message identifying the file path and the parse error. Malformed YAML is never silently skipped, since the apiVersion cannot be inspected.
- **FR-005**: The two-step classification rule (extension filter + apiVersion check) MUST apply uniformly to every code path that scans a directory for shrine manifests (deploy, apply, generate, and any other manifest-driven command), so that no command crashes on the same foreign file or unrelated file type.
- **FR-006**: When at least one file is classified as foreign by FR-001(b), shrine SHOULD emit a single concise notice (at info or debug level) listing the foreign paths, so operators can confirm the file was intentionally ignored. Files skipped by the extension filter (FR-001(a)) MUST NOT be listed. The presence or absence of this notice MUST NOT change command exit status.
- **FR-007**: Existing behaviour for valid shrine manifests (Application, Resource, Team) MUST be unchanged: same parsing, same validation errors, same downstream effects.

### Key Entities

- **Shrine Manifest File**: A file with extension `.yaml` or `.yml` whose top-level `apiVersion` matches the regex `^shrine/v\d+([a-z]+\d+)?$`. These files are parsed, validated, and applied; their `kind` and body must be valid or shrine fails loudly.
- **Foreign YAML File**: A file with extension `.yaml` or `.yml` whose `apiVersion` does not match the shrine regex (including absent, empty, or unrelated values such as `traefik.containo.us/v1alpha1`). These files are silently skipped by the scanner. Traefik routing files generated into `{specsDir}/traefik/` are the canonical example.
- **Non-YAML Sibling**: Any file in the scanned tree whose extension is not `.yaml` or `.yml` (or that has no extension). These are filtered out before any read/parse step; they never enter manifest classification at all.
- **Mistyped-Shrine File**: A YAML file the operator intended as a shrine manifest but whose `apiVersion` typo (e.g., `Shrine/v1`, `shrine/dev`) prevents the regex from matching. Treated as Foreign and skipped silently — the trade-off accepted in exchange for a strict, unambiguous detection rule.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With the Traefik plugin enabled at default settings (so generated routing files live under `{specsDir}/traefik/`), `shrine deploy` completes successfully with exit code 0 on a project that previously crashed on the same inputs.
- **SC-002**: Across every shrine command that scans `specsDir` (deploy, apply, generate, and equivalents), zero commands crash on a project containing only valid shrine manifests plus any number of foreign YAML files in the same tree.
- **SC-003**: An integration test runs `shrine deploy` against a fixture where `specsDir` contains both valid manifests and foreign YAML files, and asserts both that the deploy succeeds and that exactly the expected set of manifests was applied — no more, no less.
- **SC-004**: A regression test asserts that a YAML file with `apiVersion: shrine/v1` and a missing or misspelled `kind` still causes the corresponding command to fail with an error naming the file.
- **SC-005**: Operators no longer need to set a custom `routing-dir` outside `specsDir` to avoid the crash; the default plugin layout works out of the box.

## Clarifications

### Session 2026-04-30

- Q: What pattern qualifies a YAML file as a shrine manifest based on its `apiVersion` field? → A: Strict regex `^shrine/v\d+([a-z]+\d+)?$` — accepts versioned shrine apiVersions like `shrine/v1`, `shrine/v1beta1`, `shrine/v2`; rejects everything else (foreign, mistyped, or non-versioned values).
- Q: When a YAML file under `specsDir` cannot be parsed as YAML, should shrine fail loudly or skip it silently? → A: Fail loudly with an error naming the file and the parse error — malformed YAML is treated as an authoring mistake, never silently skipped, since the apiVersion cannot be inspected to classify the file.
- Q: Should files in the scanned tree without `.yaml` or `.yml` extensions be considered by the manifest scanner? → A: No — the scanner first applies an extension filter; only files with extension `.yaml` or `.yml` (case-sensitive) are admitted as candidates and proceed to apiVersion classification. Files with any other extension (or no extension) are skipped silently and never opened, so they cannot trigger malformed-YAML errors.

## Assumptions

- The fix targets the existing single-document YAML scanner; multi-document YAML support is out of scope and not introduced by this change.
- The extension filter (`.yaml` / `.yml`, case-sensitive) is the FIRST gate; any file failing it is invisible to subsequent checks. This means malformed-YAML errors (FR-004) only ever fire for files whose extension already implied YAML intent.
- A file's identity as a shrine manifest is determined by the `apiVersion` field alone (evaluated against a strict regex) AFTER the extension filter passes; `kind` is only considered after both prior checks succeed.
- Operators who actually intended to write a shrine manifest but typo'd the `kind` value prefer a loud failure over a silent skip; this is preserved by FR-003 once a file's apiVersion identifies it as shrine.
- Operators who typo the `apiVersion` itself (e.g., `Shrine/v1`) accept that the file will be silently skipped; they will detect this as "my manifest didn't take effect" rather than a parser error. This is a deliberate trade-off for a precise, unambiguous detection rule.
- The skip notice (FR-006) is a usability nicety, not a contract; downstream tooling should not parse it.
- Existing manifest validation rules (required fields, schema checks) apply unchanged once a file is recognised as a shrine manifest.
