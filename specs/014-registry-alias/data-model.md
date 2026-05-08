# Data Model: Registry Aliases

**Feature**: 014-registry-alias | **Date**: 2026-05-08

## Modified Entities

### RegistryConfig (`internal/config/config.go`)

Extended with a new optional `Alias` field.

| Field    | Type   | Required | Constraints |
|----------|--------|----------|-------------|
| Host     | string | yes      | Non-empty; registry hostname or IP:port |
| Username | string | no       | Credential for the registry |
| Password | string | no       | Credential for the registry |
| Alias    | string | no       | If provided: `[a-zA-Z0-9_-]+`; unique across all entries |

**Validation rules** (enforced by `Config.ValidateRegistries()`):
- If `Alias` is present and non-empty, it MUST match `^[a-zA-Z0-9_-]+$`.
- All `Alias` values across the `Registries` slice MUST be unique (case-sensitive).
- An entry with no `Alias` field is valid and unchanged from current behaviour.

**YAML representation**:
```yaml
registries:
  - host: 192.168.1.1:3000
    alias: myregistry
    username: admin
    password: secret
  - host: ghcr.io
    # no alias — anonymous pull, no expansion
```

---

## Image Reference Format

The `image` field in Application and Resource manifests gains support for the
`reg:<alias>` prefix:

```
reg:<alias>/<image-name>:<tag>
```

| Segment      | Description |
|--------------|-------------|
| `reg:`       | Reserved prefix signalling alias resolution |
| `<alias>`    | Matches a defined `alias` in config `registries`; `[a-zA-Z0-9_-]+` |
| `/`          | Required separator between alias and image path |
| `<image-name>:<tag>` | Standard Docker image reference (name + optional tag) |

**Examples**:

| Manifest value                    | Config alias      | Resolved reference               |
|-----------------------------------|-------------------|----------------------------------|
| `reg:myregistry/postgres:15`      | `myregistry → 192.168.1.1:3000` | `192.168.1.1:3000/postgres:15` |
| `reg:prod/myapp:v2.0`             | `prod → 10.0.0.5:5000`          | `10.0.0.5:5000/myapp:v2.0`     |
| `nginx:latest`                    | (no prefix)       | `nginx:latest` (unchanged)       |

**Constraints**:
- The portion after `reg:` and before the first `/` is the alias.
- An empty alias (`reg:/image:tag`) is invalid; the planner reports an error.
- The alias MUST resolve to a configured registry; an unknown alias is a planner error.

---

## No New Persistent State

Registry aliases are resolved at runtime from the config file. No new state files,
database tables, or secrets entries are introduced by this feature.
