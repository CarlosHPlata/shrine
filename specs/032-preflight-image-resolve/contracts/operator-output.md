# Contract: operator-visible output

**Feature**: 032-preflight-image-resolve

## Deploy output (terminal observer)

Per artifact, in step order, before the first `🚀 Deploying Application` or `📦 Deploying Resource` header:

```text
🔎 Resolving image for shrine-deploy-test.test-cache (traefik/whoami)
    📥 Pulling image traefik/whoami...            (indicator, cleared when the pull ends)
    ✅ Pulled image traefik/whoami
  🔎 Resolved shrine-deploy-test.test-cache traefik/whoami@a1b2c3d4e5f6
🔎 Resolving image for shrine-deploy-test.whoami-res (traefik/whoami)
    📥 Pulling image traefik/whoami...
    ✅ Pulled image traefik/whoami
  🔎 Resolved shrine-deploy-test.whoami-res traefik/whoami@a1b2c3d4e5f6
📦 Deploying Resource: test-cache (type: cache)
  🌐 Ensuring network: shrine.shrine-deploy-test.private
  …
```

Exact strings:

| Event | Status | Condition | Line |
|---|---|---|---|
| `image.resolve` | started | always | `🔎 Resolving image for <team>.<name> (<ref>)\n` |
| `image.resolve` | finished | `source == "manifest"`, digest present | `  🔎 Resolved <team>.<name> <ref>@<12 hex>\n` |
| `image.resolve` | finished | `source == "manifest"`, digest empty | `  🔎 Resolved <team>.<name> <ref>\n` |
| `image.resolve` | finished | any other source | nothing (added by T3 and T6) |
| `image.resolve` | error | always | the generic `  ❌ Error [image.resolve]: <error>\n` |

`<ref>` is the expanded reference. `<12 hex>` is the digest with its `sha256:` prefix removed and truncated to twelve characters. The started line is a plain line, not a progress indicator.

Under `IfNotPresent` with the image present locally, no `📥 Pulling image` or `✅ Pulled image` line appears between the two lines.

## Failure

```text
🔎 Resolving image for shrine-deploy-test.zz-broken (localhost:1/shrine/unresolvable:1.0.0)
  ❌ Error [image.pull]: pulling image "localhost:1/shrine/unresolvable:1.0.0": Error response from daemon: …
  ❌ Error [image.resolve]: application "zz-broken": pulling image "localhost:1/shrine/unresolvable:1.0.0": Error response from daemon: …
Error: application "zz-broken": pulling image "localhost:1/shrine/unresolvable:1.0.0": Error response from daemon: …
```

The last line is the command's error on stderr (unchanged mechanism). Nothing else is printed for the deploy: no network, container, or routing line follows, because nothing ran. Earlier artifacts' resolving and resolved lines do appear, since resolution is sequential and the failure may come after them.

## Dry run

```text
[DOCKER] ImageResolve: name=shrine-deploy-test.test-cache image=traefik/whoami policy=Always -> manifest-owned
[DOCKER] ImageResolve: name=shrine-deploy-test.whoami-res image=traefik/whoami policy=Always -> manifest-owned
[DOCKER] CreatePlatformNetwork name=shrine.platform
[DOCKER] NetworkCreate: name=shrine-deploy-test
[DOCKER] ContainerCreate: name=shrine-deploy-test.test-cache image=traefik/whoami
…
```

One `ImageResolve` line per planned step, all before `CreatePlatformNetwork`. `image=` shows the reference as the manifest wrote it (an alias stays `reg:<alias>/…`, as on the `ContainerCreate` line). `policy=` is the effective policy (`Always` or `IfNotPresent`). Repeated dry runs change no recorded state.

## Log file (`shrine.log`)

Generic lines, no new formatting:

```text
2026-10-06T10:42:17Z [started] image.resolve name=test-cache ref=traefik/whoami team=shrine-deploy-test
2026-10-06T10:42:19Z [finished] image.resolve digest=sha256:… name=test-cache ref=traefik/whoami source=manifest team=shrine-deploy-test
2026-10-06T10:42:19Z [error] image.resolve error=application "zz-broken": … name=zz-broken ref=localhost:1/shrine/unresolvable:1.0.0 team=shrine-deploy-test
```

## Unchanged

Every existing line keeps its text and position relative to the other existing lines; the only change for existing manifests is the added block above the first deploy header, and the `ImageResolve` lines at the top of a dry run.
