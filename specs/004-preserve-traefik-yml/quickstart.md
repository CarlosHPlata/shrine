# Quickstart: Preserve Operator-Edited traefik.yml

A short manual walkthrough that exercises the three user stories end-to-end against a real Shrine binary. Mirrors the integration-test scenarios but runs on the operator's actual host. Treat this as the verification script you run after a release that ships this fix.

## Prerequisites

- A built `shrine` binary on PATH (or invoke from the repo with `go run ./...`).
- Docker daemon reachable.
- A Shrine config dir with a Traefik plugin block, e.g.:
  ```yaml
  # <config-dir>/config.yml
  plugins:
    gateway:
      traefik:
        routing-dir: /tmp/shrine-quickstart/traefik
        port: 8081
  ```
- An empty `--path` directory containing at minimum a `team.yaml` (or use `tests/testdata/deploy/traefik` from the repo).

## Walkthrough

### Story 1 — Operator edits survive redeploys

```bash
# 1. Clean slate.
rm -rf /tmp/shrine-quickstart/traefik

# 2. First deploy generates the default.
shrine deploy --config-dir <config-dir> --path <manifests>
#   Expected log line: "📝 Generated default traefik.yml: /tmp/shrine-quickstart/traefik/traefik.yml"
cat /tmp/shrine-quickstart/traefik/traefik.yml

# 3. Operator hand-edits the file.
echo "# operator note: do not delete" >> /tmp/shrine-quickstart/traefik/traefik.yml
sha256sum /tmp/shrine-quickstart/traefik/traefik.yml > /tmp/before.sha

# 4. Redeploy.
shrine deploy --config-dir <config-dir> --path <manifests>
#   Expected log line: "📄 Preserving operator-owned traefik.yml: /tmp/shrine-quickstart/traefik/traefik.yml"

# 5. Verify byte-for-byte preservation.
sha256sum /tmp/shrine-quickstart/traefik/traefik.yml > /tmp/after.sha
diff /tmp/before.sha /tmp/after.sha  # must be empty
```

**Pass criteria**: step 5 produces no diff. The Preserving log line appears on every redeploy.

### Story 2 — First deploy still bootstraps a working gateway

Already exercised by step 2 above. To isolate it: `rm -rf /tmp/shrine-quickstart/traefik` and run `shrine deploy ...`. The file appears with the same content the current generator produces, the gateway container starts, and traffic flows through `:8081`.

### Story 3 — Operator can opt back into the default by deleting the file

```bash
rm /tmp/shrine-quickstart/traefik/traefik.yml
shrine deploy --config-dir <config-dir> --path <manifests>
#   Expected log line: "📝 Generated default traefik.yml: /tmp/shrine-quickstart/traefik/traefik.yml"
cat /tmp/shrine-quickstart/traefik/traefik.yml
```

**Pass criteria**: a fresh default-generated file is present, content matches what the generator produces today (no operator edits remain).

## Edge-case spot checks

### Symlink (broken target) — should be preserved

```bash
rm -f /tmp/shrine-quickstart/traefik/traefik.yml
ln -s /nonexistent/path/traefik.yml /tmp/shrine-quickstart/traefik/traefik.yml

shrine deploy --config-dir <config-dir> --path <manifests>
#   Expected log line: "📄 Preserving operator-owned traefik.yml: ..."

readlink /tmp/shrine-quickstart/traefik/traefik.yml
# must still print "/nonexistent/path/traefik.yml"
test ! -f /nonexistent/path/traefik.yml
# Shrine must not have written through the symlink
```

### Non-regular file (directory) — should be preserved

```bash
rm -f /tmp/shrine-quickstart/traefik/traefik.yml
mkdir /tmp/shrine-quickstart/traefik/traefik.yml

shrine deploy --config-dir <config-dir> --path <manifests>
#   Expected: deploy command exits 0; preserve log line is emitted
test -d /tmp/shrine-quickstart/traefik/traefik.yml
# the directory is still there; Shrine did not replace it with a file
```

(The Traefik container will fail to start in this contrived case — that failure surfaces from Traefik, not from Shrine. The contract verified here is *Shrine's* behavior.)

### Stat error — deploy fails loudly

```bash
# Make the routing dir unreadable by the deploy user.
chmod 000 /tmp/shrine-quickstart/traefik

shrine deploy --config-dir <config-dir> --path <manifests>
# Expected: nonzero exit, stderr names traefik.yml and the underlying cause.
# Shrine MUST NOT silently regenerate.

chmod 755 /tmp/shrine-quickstart/traefik   # restore for subsequent runs
```

## Confidence checklist (fill in during release verification)

- [ ] First-deploy generates `traefik.yml`; gateway container starts.
- [ ] Operator edit survives ten consecutive redeploys (SC-005).
- [ ] Deploy log shows `gateway.config.preserved` on every redeploy with file present.
- [ ] Deploy log shows `gateway.config.generated` exactly once after `rm` of the file.
- [ ] Broken symlink left untouched after deploy.
- [ ] Directory at `traefik.yml` path left untouched after deploy.
- [ ] Permission-denied stat fails the deploy with a clear error (FR-007).
- [ ] No new flag, env var, or config field needed (SC-003).
