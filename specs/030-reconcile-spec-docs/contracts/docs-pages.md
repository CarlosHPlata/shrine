# Contract: Docs Site Pages

What changes on the published docs site outside the new wiring guide (which has its own contract, [wiring-guide.md](wiring-guide.md)). Section 1 is verbatim. Sections 2–4 give each edit's location and required meaning.

All paths are under `docs/content/`. Links follow the site's existing convention: plain root-relative Markdown links, no shortcodes. Quoted output lines are taken from the renderer ([research.md](../research.md) F3) and must not be paraphrased.

## 1. The known-limitations section (verbatim, into `guides/traefik.md`)

Inserted as a new second-level section between "Dashboard access" and "Common pitfalls".

Link target used everywhere else: `/guides/traefik/#generated-gateway-files-are-written-once`

````markdown
## Known limitations

### Generated gateway files are written once

Shrine writes each file it generates for the gateway only when that file does not exist yet. From then on the file is yours: no deploy rewrites it and no teardown deletes it. Hand edits survive every deploy — and so do old values after you change a manifest or the plugin config.

Shrine does not track whether you edited a file. One it generated on an earlier deploy and you never touched is preserved exactly like one you rewrote by hand.

| File | Generated from | Not picked up once the file exists |
|------|----------------|------------------------------------|
| `{routing-dir}/traefik.yml` | `port`, `tlsPort`, `dashboard.port` | Changing `port`; adding, changing, or removing `tlsPort`; enabling the dashboard or changing its port |
| `{routing-dir}/dynamic/<team>-<app>.yml` | The application's `routing` block and `port` | Changing `routing.domain` or `routing.pathPrefix`; adding, removing, or editing an alias (`host`, `pathPrefix`, `stripPrefix`, `tls`); changing `spec.port` |
| `{routing-dir}/dynamic/__shrine-dashboard.yml` | `dashboard.username`, `dashboard.password` | Changing the credentials |

Two things do follow your config on every deploy: the Traefik container's host port bindings, and the removal of the dashboard file — delete the `dashboard` block and the next deploy deletes `__shrine-dashboard.yml`.

**How to tell.** The deploy output reports every file as generated or preserved:

```text
  📝 Generated route file: /srv/shrine/traefik/dynamic/my-team-hello.yml
  📄 Preserving operator-owned route file: /srv/shrine/traefik/dynamic/my-team-hello.yml
```

"Preserving operator-owned" means the file already existed and was left alone.

**How to apply a change.** Delete the file and deploy again; Shrine regenerates it from the current manifest and config:

```bash
rm /srv/shrine/traefik/dynamic/my-team-hello.yml
shrine deploy
```

Or edit the file by hand. Traefik watches the `dynamic/` directory, so route changes apply immediately. `traefik.yml` is read only when Traefik starts: after regenerating or editing it, restart the gateway unless that deploy already recreated the container:

```bash
docker restart platform.traefik
```

**After a teardown.** `shrine teardown <team>` removes the team's containers and leaves their route files in place, printing the command to remove each one:

```text
  ⚠️  Orphan route file left on disk; remove with: rm /srv/shrine/traefik/dynamic/my-team-hello.yml
```

Until you remove the file, Traefik keeps a router that points at a container that no longer exists.
````

Every fact in this section must agree with the canonical statement in [spec-amendments.md](spec-amendments.md) §1. If one changes, both change.

## 2. Links at the point of use (FR-009)

"→ link" means a link to the §1 target with the text "Generated gateway files are written once" (or a natural shortening).

| Page | Location | Edit |
|---|---|---|
| `guides/traefik.md` | "Configure entrypoints", paragraph beginning "When `tlsPort` is set" | Rewrite: Shrine publishes `<tlsPort>:443/tcp` on the container and, when it generates `traefik.yml`, includes a `websecure` entrypoint. An existing `traefik.yml` is not modified → link. |
| `guides/traefik.md` | "Per-app routing", after "Shrine writes a dynamic config file at …" | Add one sentence: the file is written once; later changes to the `routing` block do not rewrite it → link. |
| `guides/traefik.md` | "Dashboard access" | Add one sentence: changing credentials later does not rewrite the file → link; removing the `dashboard` block deletes it on the next deploy. |
| `guides/traefik.md` | "Common pitfalls" | Add a bullet: "**A manifest or config change has no effect on routing**" → link. |
| `guides/routing-and-aliases.md` | New section "Changing routing after the first deploy", before "Logging" | Three or four sentences: the route file is written on the app's first deploy and never rewritten; adding, removing, or editing an alias — or changing `routing.domain`, `pathPrefix`, `stripPrefix`, `tls` — takes effect only after the file is deleted and the app redeployed, or the file is edited; show the `rm` + `shrine deploy` pair and the "Preserving operator-owned route file" line → link. |
| `guides/tls.md` | "Mark an alias as TLS", after the paragraph on the generated router | Add: if the application was already deployed, its route file exists and is not rewritten; delete it and redeploy for `tls: true` to take effect → link. |
| `troubleshooting/_index.md` | New entry before "See also" | Heading "A routing change in the manifest did not take effect". Symptom: a changed domain, alias, `stripPrefix`, `tls`, or gateway port is not reflected after a redeploy. Evidence: the deploy output shows `Preserving operator-owned route file` or `Preserving operator-owned traefik.yml`. Fix: delete the file named in that line and redeploy, or edit it → link. |

## 3. Corrections to statements that contradict the policy (FR-010)

`guides/tls.md`, "Configure the gateway":

| Current | Must say afterwards |
|---|---|
| "Run `shrine deploy` once. Shrine recreates the Traefik container with the new port binding and, when the static config (`traefik.yml`) is Shrine-generated (not operator-preserved), adds the `websecure` entrypoint:" | Run `shrine deploy`. Shrine recreates the Traefik container with the new port binding. If `traefik.yml` does not exist yet, Shrine generates it with the `websecure` entrypoint. (The YAML sample that follows is kept.) |
| "If your `traefik.yml` was preserved from a prior operator edit (per Shrine's preservation regime), Shrine will not overwrite it. You must add the `websecure` entrypoint yourself, or delete the file so Shrine regenerates it." | On a host where the gateway has been deployed before, `traefik.yml` already exists and Shrine does not modify it; the deploy prints a warning beginning `tlsPort set but traefik.yml is missing websecure entrypoint`. Either delete `traefik.yml` before deploying — Shrine regenerates it with `websecure` in the same deploy — or add the entrypoint to the file by hand and restart the gateway → link. |

No other sentence in the three gateway guides was found to contradict the policy. "your preserved `traefik.yml`" in the Certificates section of the TLS guide is accurate and stays.

## 4. Secrets vault guide, "Dry-run behaviour" (FR-032)

`guides/secrets-vault.md`. The section currently shows `env DB_PASSWORD=[VAULT:…]` as example output; the preview prints no environment values (research F7).

Must say afterwards: `shrine deploy --dry-run` does not contact the vault — every `vault:` reference is resolved to a placeholder internally, so the preview succeeds without network access or credentials. The preview validates manifest structure and dependency wiring and prints the deploy order and the container operations. It does not print environment values, so the placeholders are not displayed. The closing sentence about CI lint jobs and laptops without vault access stays.

The fabricated example block is removed. Any other sentence in the guide that says the placeholder is "shown" or "rendered in the plan output" is corrected the same way; `grep -n 'VAULT:' docs/content` finds them.

## 5. Front matter and listings for the new guide

```yaml
---
title: "Wiring env and outputs"
description: "Connect resources and applications: private config, exported outputs, and deploy order."
weight: 15
---
```

- `guides/_index.md`, Contents list: add `- [Wiring env and outputs](wiring-env-and-outputs/) — Connect resources and applications: private config, exported outputs, and deploy order.` after the TLS entry.
- `reference/manifest-schema.md`, Resource section, after the "Strict allowlist" paragraph: add `See the [Wiring env and outputs guide](/guides/wiring-env-and-outputs/) for a walkthrough.` (FR-028)
