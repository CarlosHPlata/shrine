# Contract: Registry Config with Alias

**Feature**: 014-registry-alias | **Date**: 2026-05-08

## shrine.yml — registries entry (extended)

```yaml
registries:
  - host: <string>         # required — registry hostname or IP:port
    alias: <string>        # optional — short name for use in image: reg:<alias>/...
    username: <string>     # optional — pull credential
    password: <string>     # optional — pull credential
```

### Alias field rules

| Rule | Detail |
|------|--------|
| Format | `^[a-zA-Z0-9_-]+$` — alphanumeric, hyphens, underscores only |
| Uniqueness | All alias values across the `registries` list must be distinct (case-sensitive) |
| Optional | Omitting `alias` is valid; the entry behaves as before |
| Dots | Not permitted — avoids ambiguity with DNS hostnames in image paths |

### Validation errors (config load time)

| Condition | Error message |
|-----------|---------------|
| Duplicate alias `X` | `registries: alias "X" is defined more than once` |
| Invalid characters in alias `X` | `registries: alias "X" contains invalid characters (alphanumeric, hyphens, underscores only)` |

---

## Application / Resource manifest — image field (extended)

```yaml
spec:
  image: reg:<alias>/<image-name>:<tag>
```

### Image resolution errors (plan time)

| Condition | Error message |
|-----------|---------------|
| Alias portion is empty (`reg:/image:tag`) | `app "<name>": image "reg:/image:tag": alias name must not be empty` |
| Alias not found in config | `app "<name>": image "reg:X/...": alias "X" is not defined in config registries` |

### Dry-run output

Dry-run (`shrine deploy --dry-run`) prints the image value as written in the manifest
(`reg:<alias>/image:tag`). Expansion to the real host is not shown in dry-run output.

### Live execution

The container backend replaces the `reg:<alias>` prefix with the corresponding `host`
value before calling the Docker image pull API. The raw `reg:` string is never passed
to Docker.
