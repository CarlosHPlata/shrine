# Contract: `routing.aliases` manifest schema

**Feature**: 006-routing-aliases
**Audience**: operators authoring Application manifests; integration test authors

This contract defines the externally-visible YAML interface introduced by the feature. It is the canonical reference for what operators may write and what errors they will see when they write something invalid.

---

## YAML shape

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: hello-api
  owner: team-a
spec:
  image: hello-api
  port: 8080
  routing:
    domain: hello-api.home.lab          # required (when any routing is declared)
    pathPrefix: /hello-api              # optional; pre-existing field, unchanged
    aliases:                            # optional; new in this feature
      - host: gateway.tail9a6ddb.ts.net # required for each alias entry
        pathPrefix: /finances           # optional
        stripPrefix: true               # optional; defaults to true when pathPrefix is set
      - host: lan.home.lab              # alias with no pathPrefix
```

---

## Field reference

### `spec.routing.aliases` (list, optional)

Zero or more alias entries. Omitting the field, setting it to `null`, or setting it to `[]` are all equivalent — the application is published only at its primary domain.

### `spec.routing.aliases[].host` (string, required when the alias entry exists)

The additional hostname at which the application should be reachable. Must:

- Be non-empty.
- Not contain spaces or control characters.

Validation does **not** enforce full RFC 1035 hostname syntax. Typos that are still syntactically valid (e.g., `gatway.tail9a6ddb.ts.net`) are accepted; the failure surfaces at DNS resolution time.

### `spec.routing.aliases[].pathPrefix` (string, optional)

A path prefix that further narrows alias routing to a subtree of the host. Must, when present:

- Start with `/`.
- Not be the empty string.
- Not be just `/` (use omission instead).
- Not contain spaces or control characters.

A trailing `/` (e.g., `/finances/`) is accepted and normalized away internally so `/finances` and `/finances/` produce identical gateway behavior.

### `spec.routing.aliases[].stripPrefix` (bool, optional)

Controls whether the matched `pathPrefix` is removed from the request path before the backend receives it.

- Default: `true` when `pathPrefix` is set.
- No effect when `pathPrefix` is empty or omitted.
- Set to `false` when the backend expects the prefix on the wire (e.g., it serves at `/finances/...` natively).

---

## Examples

### Single alias, default strip

```yaml
spec:
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
```

A backend serving at root keeps working: requests to `gateway.tail9a6ddb.ts.net/finances/api/balance` reach the backend as `/api/balance`.

### Multiple aliases, mixed strip

```yaml
spec:
  routing:
    domain: notes.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /notes
        # stripPrefix defaults to true → backend sees /api/list
      - host: notes-v2.home.lab
        # no pathPrefix → backend sees full path; stripPrefix has no effect
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /notes-raw
        stripPrefix: false
        # backend sees /notes-raw/api/list (e.g., for an app that serves under /notes-raw natively)
```

### Empty aliases list (equivalent to omission)

```yaml
spec:
  routing:
    domain: hello.home.lab
    aliases: []   # behaves identically to omitting the field
```

---

## Validation errors (operator-facing)

When `shrine apply` or `shrine deploy` rejects a manifest because of alias misuse, the error message is one of the following shapes (joined into a multi-error report when multiple apply):

| Trigger | Error shape |
|---|---|
| `aliases` set but `domain` empty | `validation failed:\n- spec.routing.aliases is set but spec.routing.domain is empty` |
| Missing `host` on an alias entry | `validation failed:\n- spec.routing.aliases[2].host is required` |
| `host` contains invalid characters | `validation failed:\n- spec.routing.aliases[0].host "bad host" contains invalid characters` |
| `pathPrefix` missing leading `/` | `validation failed:\n- spec.routing.aliases[1].pathPrefix "finances" must start with "/"` |
| `pathPrefix` is bare `/` | `validation failed:\n- spec.routing.aliases[1].pathPrefix must not be just "/"` |
| `pathPrefix` contains invalid characters | `validation failed:\n- spec.routing.aliases[0].pathPrefix "/has space" contains invalid characters` |
| Same `(host, pathPrefix)` declared twice on the same app | `validation failed:\n- spec.routing: duplicate route gateway.tail9a6ddb.ts.net/finances declared on alias[1]` |
| Two different apps collide on `(host, pathPrefix)` | `routing collision: host="gateway.tail9a6ddb.ts.net" pathPrefix="/finances" declared by "team-a/finances" and "team-b/notes"` (one bullet per colliding pair, joined as a multi-error if more than one) |

All error formats include the offending application's `<owner>/<name>` (or alias index) so operators can locate the bad manifest in seconds.

---

## Compatibility guarantees

- Manifests that omit `aliases` are byte-identical in behavior to pre-feature releases (SC-005).
- The pre-existing `routing.pathPrefix` on the primary domain is unchanged: no implicit strip, no other behavior changes.
- A manifest with `aliases` deployed against a host that has no Traefik gateway plugin succeeds without warning (FR-010).
- A manifest with `aliases` deployed against a host that has a non-Traefik gateway plugin succeeds; that plugin chooses whether to consume aliases (FR-010).
