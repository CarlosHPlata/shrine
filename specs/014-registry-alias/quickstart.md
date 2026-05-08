# Quickstart: Registry Aliases

**Feature**: 014-registry-alias

## 1. Define an alias in shrine.yml

```yaml
registries:
  - host: 192.168.1.1:3000
    alias: myregistry
    username: admin
    password: s3cr3t
```

## 2. Use the alias in a manifest

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: myapp
  owner: myteam
spec:
  image: reg:myregistry/myapp:latest
  port: 8080
```

The `reg:myregistry/` prefix is resolved to `192.168.1.1:3000/` at deployment time.

## 3. Verify with dry-run

```bash
shrine deploy --dry-run
```

Dry-run output preserves the alias form:

```
[DOCKER] ContainerCreate: name=myteam.myapp image=reg:myregistry/myapp:latest
```

This is expected — the alias is expanded only when the live container engine pulls
the image. The dry-run confirms the alias is valid (no "alias not found" error) but
does not show the resolved hostname.

## 4. Deploy

```bash
shrine deploy
```

The container engine expands `reg:myregistry/` → `192.168.1.1:3000/` and pulls
`192.168.1.1:3000/myapp:latest` using the configured credentials.

## Error: unknown alias

If you reference an alias that is not in the config:

```yaml
image: reg:typo/myapp:latest
```

The planner will report at plan or dry-run time:

```
app "myapp": image "reg:typo/myapp:latest": alias "typo" is not defined in config registries
```

Fix: add the missing registry entry in `shrine.yml` or correct the alias name.

## Notes

- Dots are not allowed in alias names. Use hyphens or underscores: `my-registry`, `registry_prod`.
- One alias per registry entry. To have two names for the same host, add two entries with the same `host` but different `alias` values.
- Plain image references (no `reg:` prefix) continue to work exactly as before.
