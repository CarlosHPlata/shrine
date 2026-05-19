---
title: "Guides"
description: "Task-focused walkthroughs for common Shrine operations."
weight: 30
cascade:
  type: docs
---

Learn how to accomplish specific tasks with Shrine. These guides walk through real-world scenarios step-by-step.

## Contents

- [Traefik gateway](traefik/) — Configure the Traefik plugin to expose your apps publicly.
- [Routing & aliases](routing-and-aliases/) — Multiple hostnames and path prefixes per app.
- [TLS / HTTPS](tls/) — Terminate HTTPS at Traefik for any aliased route.
- [Custom registries](custom-registries/) — Pull from private registries and use short aliases in manifests.
- [Secrets vault](secrets-vault/) — Store secrets in an external vault and reference them from manifests.
- [Team-scoped deploy](team-scoped-deploy/) — Use `shrine deploy team <name>` to reconcile only one team's stack.
