# Quickstart: Verify the Routing-Dir Scan Fix

This guide walks an operator (or reviewer) through reproducing the original crash on `main`, confirming the fix is in place on this branch, and exercising every spec acceptance scenario by hand. Estimated time: ~10 minutes.

## Prerequisites

- A working Docker daemon (`docker info` succeeds).
- Go 1.24+ on `PATH`.
- This repo checked out, branch `002-fix-routing-dir-manifest-scan`.

```bash
cd /root/projects/shrine
go version            # >= 1.24
docker info >/dev/null && echo "docker OK"
```

## Step 1 — Reproduce the crash on `main` (optional, baseline)

```bash
git stash                     # save in-progress edits
git checkout main
go build -o /tmp/shrine-main ./cmd/shrine

# Build a minimal project where routing-dir lives inside specsDir
work=$(mktemp -d)
cp -r tests/testdata/deploy/basic/* "$work/"
cp tests/testdata/deploy/team.yaml "$work/"   # adjust path if your fixture differs
mkdir -p "$work/traefik"
cat > "$work/traefik/traefik.yml" <<'EOF'
# Mimics what internal/plugins/gateway/traefik/config_gen.go writes:
# a static Traefik config with NO apiVersion field.
entryPoints:
  web:
    address: ":80"
providers:
  file:
    directory: /etc/traefik/dynamic
EOF

state=$(mktemp -d)
/tmp/shrine-main apply teams --path "$work" --state-dir "$state"
/tmp/shrine-main deploy --path "$work" --state-dir "$state"
# EXPECTED on main: exits non-zero with `unknown manifest kind: ""` referencing
# $work/traefik/traefik.yml. This is the SC-001 regression.

git checkout 002-fix-routing-dir-manifest-scan
git stash pop || true
```

If you skip Step 1, just trust the integration test added under [tests/integration/traefik_plugin_test.go](../../tests/integration/traefik_plugin_test.go).

## Step 2 — Build the fixed binary

```bash
go build -o /tmp/shrine-fix ./cmd/shrine
```

## Step 3 — Acceptance scenario from User Story 1 (P1)

> Default Traefik plugin layout — `routing-dir` inside `specsDir` — must deploy cleanly.

```bash
work=$(mktemp -d)
cp -r tests/testdata/deploy/basic/* "$work/"
cp tests/testdata/deploy/team.yaml "$work/"
mkdir -p "$work/traefik/dynamic"
cat > "$work/traefik/traefik.yml" <<'EOF'
entryPoints:
  web:
    address: ":80"
providers:
  file:
    directory: /etc/traefik/dynamic
EOF
cat > "$work/traefik/dynamic/team-foo-app.yml" <<'EOF'
http:
  routers:
    foo:
      rule: "Host(`foo.shrine.lab`)"
      service: foo
EOF

state=$(mktemp -d)
/tmp/shrine-fix apply teams --path "$work" --state-dir "$state"
/tmp/shrine-fix deploy     --path "$work" --state-dir "$state"
# EXPECTED:
#   - exit code 0
#   - one informational line listing the two Traefik files as ignored (FR-006)
#   - the basic app container is created and running
docker ps | grep -E '\.whoami\b'
```

## Step 4 — Acceptance scenario from User Story 2 (P2)

> Foreign YAML files (no apiVersion / wrong apiVersion) coexist in `specsDir` — silently skipped.

```bash
cat > "$work/editor-config.yaml" <<'EOF'
# Some other tool's settings; no apiVersion.
indent: 2
useTabs: false
EOF
cat > "$work/k8s-style.yaml" <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata: { name: not-shrine }
EOF

/tmp/shrine-fix deploy --path "$work" --state-dir "$state"
# EXPECTED: exit 0; the FR-006 notice now lists 4 foreign files (the two
# Traefik files + editor-config.yaml + k8s-style.yaml). No new containers.
```

## Step 5 — Acceptance scenario from User Story 3 (P2)

> A file that **claims** to be a shrine manifest (`apiVersion: shrine/v1`) but has a typo'd `kind` MUST still fail loudly.

```bash
cat > "$work/typo.yaml" <<'EOF'
apiVersion: shrine/v1
kind: Aplication        # typo on purpose
metadata: { name: oops, owner: shrine-deploy-test }
spec:
  image: nginx
  port: 80
EOF

/tmp/shrine-fix deploy --path "$work" --state-dir "$state"
# EXPECTED: non-zero exit. stderr names $work/typo.yaml and the offending
# kind value. (FR-003)

rm "$work/typo.yaml"
```

## Step 6 — Edge case: malformed YAML in a `.yaml` file

> Files admitted by extension but unparseable as YAML must error loudly (FR-004).

```bash
cat > "$work/broken.yaml" <<'EOF'
apiVersion: shrine/v1
kind: [this list never closes
EOF

/tmp/shrine-fix deploy --path "$work" --state-dir "$state"
# EXPECTED: non-zero exit. stderr names $work/broken.yaml and the YAML parse error.

rm "$work/broken.yaml"
```

## Step 7 — Edge case: non-YAML siblings are invisible

> Non-`.yaml`/`.yml` extensions must be ignored without being read (FR-001(a)).

```bash
echo "*.log" > "$work/.gitignore"
echo "{ broken json" > "$work/config.json"
chmod 000 "$work/config.json"   # unreadable on purpose
/tmp/shrine-fix deploy --path "$work" --state-dir "$state"
# EXPECTED: exit 0. Shrine never opens config.json; the .gitignore is invisible.
chmod 644 "$work/config.json" && rm "$work/config.json" "$work/.gitignore"
```

## Step 8 — Run the automated gate

The hand-walked scenarios above must also be enforced by the integration suite (Constitution V):

```bash
make test-integration
# OR
go test -tags integration ./tests/integration/... -run 'TestDeploy|TestTraefikPlugin' -v
```

EXPECTED: every test passes, including the two new cases:

- `TestDeploy/should_deploy_successfully_when_specsDir_contains_foreign_YAML_files`
- `TestTraefikPlugin/should_succeed_when_routing-dir_is_inside_specsDir`

## Step 9 — Cleanup

```bash
docker ps -aq --filter "name=^shrine-deploy-test\." | xargs -r docker rm -f
docker network ls -q --filter "name=^shrine\.shrine-deploy-test\." | xargs -r docker network rm
rm -rf "$work" "$state"
```

## Success Criteria mapping

| Spec criterion | Where verified |
|----------------|----------------|
| SC-001 (default Traefik layout deploys) | Steps 1+3, plus `TestTraefikPlugin/should_succeed_when_routing-dir_is_inside_specsDir` |
| SC-002 (no command crashes on foreign YAML) | Step 4, plus `TestDeploy/should_deploy_successfully_when_specsDir_contains_foreign_YAML_files` |
| SC-003 (integration test asserts exact applied set) | Step 8 — the new TestDeploy case asserts which containers were and were not created |
| SC-004 (regression: shrine/v1 + bad kind still fails loudly) | Step 5, plus `internal/manifest/classify_test.go` table case "Shrine v1 with bogus kind" → followed by a planner-level test asserting the loud error |
| SC-005 (default plugin layout works out of the box) | Step 3 — same fixture as SC-001, exercised manually and via integration test |
