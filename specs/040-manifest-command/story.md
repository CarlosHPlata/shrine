# Manifest `command`

## Story
As an operator, I want to declare a `command` in an Application or Resource manifest, so that the container runs that command instead of the image's default one, as Docker Compose's `command` does.

## Acceptance criteria
- [ ] Given an Application or Resource manifest with `spec.command` as a list, when I deploy or apply it, then the container is created running that command in place of the image's default command (the image's entrypoint is kept); a manifest without `command` behaves exactly as today.
- [ ] Given a deployed workload, when I add, change or remove `command` and deploy again, then the container is recreated with the new command; when `command` is unchanged the container is left alone, and workloads with no `command` are not recreated just because Shrine was upgraded.
- [ ] Given a manifest whose `command` is invalid (empty list, empty item, or a plain string instead of a list), when I deploy or apply it, then it is rejected before anything changes, with a message naming the manifest and the field.
- [ ] Given the manifest schema reference in the docs, when I look up the Application or Resource kind, then `command` is listed with its type (list of strings), that it is optional, what it overrides, and an example.

## Plan
- Manifest: add an optional list-valued `command` to both the Application and Resource specs.
- Validation: reject the invalid shapes alongside the existing spec checks, for both kinds.
- Planning: carry the command through to the container-creation step, the same way env and volumes travel.
- Engine: pass the command to Docker on create, and include it in the config fingerprint only when set, so existing fingerprints stay stable.
- Docs: add `command` to the manifest schema reference (`docs/content/reference/manifest-schema.md`) for both the Application and Resource kinds, with one example, and mirror it in the manifest section of `AGENTS.md`.

## Out of scope
- The string form of `command` (`command: npm start`).
- Overriding the image `entrypoint`.
- Template or `valueFrom` substitution inside `command`.
- Showing the command in `describe`, `status`, `get --json`, or the dry-run plan.
- `command` on the Traefik gateway or other plugin-managed containers.

## Suggestions (your call)
- None; the field follows the path env and volumes already take, so no new pattern is needed.
