# Quickstart: Publish Application Ports on Localhost

Manual end-to-end verification of feature 023. Assumes a built `shrine` binary,
a running Docker daemon, and a scratch config/state directory.

## 0. Pre-flight

```bash
work=$(mktemp -d)
mkdir -p "$work/manifests"

cat > "$work/manifests/team.yaml" <<'EOF'
apiVersion: shrine/v1
kind: Team
metadata:
  name: demo
EOF

shrine apply teams --path "$work/manifests"
```

## 1. Explicit port — hit localhost

```bash
cat > "$work/manifests/web.yaml" <<'EOF'
apiVersion: shrine/v1
kind: Application
metadata:
  name: web
  owner: demo
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish:
      hostPort: 8080
EOF

shrine deploy --dry-run --path "$work/manifests"
# expect:  publish=127.0.0.1:8080->80/tcp
#          attach to platform network=shrine.platform   (implied — no exposeToPlatform set)

shrine deploy --path "$work/manifests"
curl -fsS http://localhost:8080 >/dev/null && echo "OK: reachable on localhost:8080"

docker inspect demo.web \
  --format '{{json .HostConfig.PortBindings}}'
# expect: {"80/tcp":[{"HostIp":"127.0.0.1","HostPort":"8080"}]}
```

## 2. Conflict — fail fast at dry run and deploy

```bash
cat > "$work/manifests/web2.yaml" <<'EOF'
apiVersion: shrine/v1
kind: Application
metadata:
  name: web2
  owner: demo
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish:
      hostPort: 8080
EOF

shrine deploy --dry-run --path "$work/manifests"
# expect non-zero exit and:
#   host port collision: port 8080 declared by "demo/web" and "demo/web2"

shrine deploy --path "$work/manifests"
# expect the same error; verify web2 was never created:
docker ps -a --filter name=demo.web2 --format '{{.Names}}'   # (empty)

rm "$work/manifests/web2.yaml"
```

## 3. Automatic port — allocated, reported, stable

```bash
cat > "$work/manifests/api.yaml" <<'EOF'
apiVersion: shrine/v1
kind: Application
metadata:
  name: api
  owner: demo
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish: true
EOF

shrine deploy --path "$work/manifests"
# expect:     📡 Published demo/api on 127.0.0.1:30000 -> 80/tcp

grep api "$XDG_STATE_HOME"/shrine/hostports.txt   # or your configured state dir
# expect: demo/api=30000
curl -fsS http://localhost:30000 >/dev/null && echo "OK: reachable on assigned port"
```

## 4. Stability across redeploys, recreation, and teardown

```bash
# 4a. plain redeploy — same port
shrine deploy --path "$work/manifests"

# 4b. force recreation (env change alters the config hash) — same port
sed -i 's/publish: true/publish: true\n  env:\n    - name: FORCED\n      value: "1"/' "$work/manifests/api.yaml" 2>/dev/null || \
  echo "(edit api.yaml: add any env var to force recreation)"
shrine deploy --path "$work/manifests"

# 4c. teardown + redeploy — same port
shrine teardown demo
shrine deploy --path "$work/manifests"

curl -fsS http://localhost:30000 >/dev/null && echo "OK: port survived all three"
```

## 5. Dry-run has no side effects

```bash
sum_before=$(md5sum "$XDG_STATE_HOME"/shrine/hostports.txt)
shrine deploy --dry-run --path "$work/manifests"
# for demo/api (already allocated) expect: publish=127.0.0.1:30000->80/tcp
sum_after=$(md5sum "$XDG_STATE_HOME"/shrine/hostports.txt)
[ "$sum_before" = "$sum_after" ] && echo "OK: state file untouched"
```

## 6. Release — delete application, delete team

```bash
shrine teardown demo

shrine delete application api
# expect: Released host port 30000 for demo/api.

shrine delete team demo
# expect: Released 1 host port(s) for team "demo".   (demo/web=8080 goes with the team)

grep demo "$XDG_STATE_HOME"/shrine/hostports.txt || echo "OK: no demo allocations remain"
```

## 7. Cleanup

```bash
docker rm -f demo.web demo.api 2>/dev/null
rm -rf "$work"
```
