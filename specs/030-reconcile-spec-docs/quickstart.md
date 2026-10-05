# Quickstart: Verifying the Reconciliation

How to check this change locally before it is pushed, and what CI checks afterwards. Nothing here needs Docker. Run everything from the repository root on branch `030-reconcile-spec-docs`.

## Gate 1 — Scope: no product files in the diff (FR-029, SC-009)

```bash
{ git diff --name-only main; git ls-files --others --exclude-standard; } | sort -u | grep -Ev '^(specs/|docs/content/|AGENTS\.md$|CLAUDE\.md$|\.specify/feature\.json$)'
```

Expected: no output. Any line printed is a file this feature must not touch. (The command covers committed, uncommitted, and untracked files.)

```bash
git diff --name-only main -- docs/content/cli
```

Expected: no output (the CLI reference is generated).

## Gate 2 — Docs site builds and passes the checks CI runs (FR-030, SC-010)

```bash
bash scripts/lint-docs-frontmatter.sh docs/content
```

```bash
make docs-build
```

```bash
bash scripts/check-md-companions.sh docs/public
```

```bash
bash scripts/check-md-shape.sh docs/public
```

Expected: each exits 0. `make docs-tools` installs Hugo if it is missing.

## Gate 3 — Links and anchors resolve (FR-030, SC-003)

CI does not check plain Markdown links, so this gate is local only. Run it after Gate 2.

The new guide was built:

```bash
test -f docs/public/guides/wiring-env-and-outputs/index.html && echo OK
```

The known-limitations anchor exists (the minified HTML writes ids unquoted):

```bash
grep -c 'id=generated-gateway-files-are-written-once' docs/public/guides/traefik/index.html
```

Expected: `1`.

Every page that should link to it does:

```bash
grep -rl 'generated-gateway-files-are-written-once' docs/content | sort
```

Expected: `guides/routing-and-aliases.md`, `guides/tls.md`, `guides/traefik.md`, `troubleshooting/_index.md` (each under `docs/content/`).

Every root-relative link in a changed page has a built target:

```bash
{ git diff --name-only main -- docs/content; git ls-files --others --exclude-standard -- docs/content; } | sort -u | xargs -r grep -ohE '\]\(/[^)#]*' | sed 's/^](//' | sort -u | while read -r p; do [ -f "docs/public${p}index.html" ] || echo "BROKEN: $p"; done
```

Expected: no output.

## Gate 4 — Spec text (SC-001, SC-004, SC-005, SC-008)

These greps find the places a reviewer must read; a hit is acceptable only where noted.

Each preserve-policy spec links the canonical statement:

```bash
grep -c 'generated-gateway-file-lifecycle-canonical' specs/004-*/spec.md specs/006-*/spec.md specs/008-*/spec.md specs/011-*/spec.md specs/012-*/spec.md
```

Expected: every count is at least 1.

The canonical section exists exactly once:

```bash
grep -rc '^## Generated Gateway File Lifecycle (Canonical)' specs/*/spec.md | grep -v ':0'
```

Expected: one line, for spec 009.

No requirement or criterion identifier was renamed, added, or dropped in an amended spec:

```bash
for s in 004-preserve-traefik-yml 006-routing-aliases 008-alias-strip-prefix 009-preserve-app-configs 011-traefik-tlsport-config 012-tls-alias-routers 015-infisical-secrets-vault 021-resource-env-output-split; do diff <(git show "main:specs/$s/spec.md" | grep -oE '\*\*(FR|SC)-[0-9]+[a-z]?\*\*' | sort -u) <(grep -oE '\*\*(FR|SC)-[0-9]+[a-z]?\*\*' "specs/$s/spec.md" | sort -u) >/dev/null && echo "ok   $s" || echo "DIFF $s"; done
```

Expected: eight `ok` lines.

Spec 015 no longer presents vault references on Resource outputs as supported:

```bash
grep -n -i 'resource output\|spec\.outputs' specs/015-infisical-secrets-vault/spec.md
```

Every hit must be inside an "Originally:" note or state that the form is not valid.

Spec 009 no longer places the orphan warning on deploy:

```bash
grep -n -i 'orphan' specs/009-preserve-app-configs/spec.md | grep -i 'deploy'
```

Every hit must be inside an "Originally:" note, or say that deploy does *not* emit the warning.

Both descoped requirements point to known gaps that exist:

```bash
grep -n -i 'descoped' specs/021-*/spec.md specs/012-*/spec.md specs/015-*/spec.md specs/021-*/tasks.md
```

```bash
grep -n 'does not print resolved environment\|rejected with a decoder error' specs/progress.md
```

Expected: at least one hit per spec in the first; two hits in the second.

Tracking metadata:

```bash
grep -n 'PlanSingle' AGENTS.md
```

Expected: no output.

```bash
grep -n 'Phase 9\|Phase 11\|Next phase\|Go version' specs/progress.md
```

Read the four lines: Phases 9 and 11 checked, next phase is 10, Go version matches `go.mod`.

```bash
ls -d specs/005-traefik-entrypoints specs/007-fix-traefik-dynamic-dashboard 2>/dev/null
```

Expected: no output.

## Gate 5 — The guide's manifests work as written (FR-027, SC-006)

Use throwaway directories so the preview cannot touch real Shrine state. `$W` below is any empty scratch directory outside the repository.

```bash
go build -o "$W/shrine" .
```

Copy the five complete same-team manifests out of `docs/content/guides/wiring-env-and-outputs.md` — exactly as the page shows them — into `$W/guide/`, then:

```bash
"$W/shrine" --config-dir "$W/cfg" --state-dir "$W/state" apply teams --path "$W/guide"
```

```bash
"$W/shrine" --config-dir "$W/cfg" --state-dir "$W/state" deploy --dry-run --path "$W/guide"; echo "exit=$?"
```

Expected: `exit=0`, and the output from `Deploy order:` onward matches [contracts/wiring-guide.md](contracts/wiring-guide.md) §4 and the block shown in the guide.

Then add the `ops` team and `reporter` from the guide's cross-team step and repeat the two commands at each stage the guide describes. Expected: the three messages in the contract's §5, in order, then `exit=0`.

If any output differs from what the guide prints, the guide is wrong: fix the guide, not the expectation.

## Gate 6 — Reading checks a command cannot do

- **SC-002**: read spec 006 FR-009, the canonical section in spec 009, the routing guide's new section, and the known-limitations section. All four must give the same answer to "I removed an alias and redeployed — is the route gone?"
- **Consistency rule 2** ([data-model.md](data-model.md) §3): the canonical statement and the known-limitations section list the same files and the same inputs.
- **SC-007**: have someone who has not used Resource outputs follow the guide from an empty directory.
- **`specs/features/integration-tests.md`**: each phase marked complete was checked against the sub-test names in `tests/integration/`.

## After the gates

```bash
graphify update .
```

Keeps the knowledge graph current; `graphify-out/` is ignored by git, so this adds nothing to the diff.

## CI

The docs workflow runs Gate 2 plus the CLI drift check on the pull request. The integration suite is unaffected — no product or test file changes — and is not run locally.

## If a statement turns out to be wrong

Research was done against `main` @ `64c93bc`. If, while editing, the product is found to behave differently from what [research.md](research.md) records, the text follows the product (FR-031): stop, note the divergence in the pull request description, and update the research finding and the affected contract before continuing.
