# Contract: `networking.publish` manifest schema

**Feature**: 023-publish-host-ports

## Field forms

Automatic allocation:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: dashboard
  owner: ops
spec:
  image: reg:dashboard:1.2.0
  port: 3000
  networking:
    publish: true
```

Explicit host port:

```yaml
spec:
  image: reg:dashboard:1.2.0
  port: 3000
  networking:
    publish:
      hostPort: 8080
```

Not published (all equivalent):

```yaml
networking: {}
```
```yaml
networking:
  publish: false
```
(or the `publish` key omitted entirely)

## Validation rules (parse/validate time, multi-error)

| Rule | Error condition |
|---|---|
| `hostPort` range | set and outside `1024–65535` |
| Automatic-range exclusion | set and inside `30000–32767` |
| Node shape | `publish` is neither a boolean nor a mapping with `hostPort` |

Errors are appended to the existing per-manifest multi-error report; they never
fail fast individually.

## Behavior combination table (canonical — this table ships verbatim into the reference docs)

| `exposeToPlatform` | `publish` | On platform network | Host port published | Other teams may depend on it |
|---|---|---|---|---|
| off | off | no | no | no |
| on | off | yes | no | yes |
| off | on | yes (implied) | yes | no |
| on | on | yes | yes | yes |

## Compatibility guarantees

- Manifests without `publish` parse, validate, hash, and deploy byte-for-byte as
  before (SC-006).
- `publish` does not alter routing behavior: `routing.domain`/aliases still
  require the raw `exposeToPlatform` field, exactly as today.
- The published host port always maps to `spec.port`; there is no separate
  container-port override.
