---
title: "Quick Start"
description: "Deploy your first app with Shrine in under five minutes."
weight: 20
---

## What you'll do

Create a Team manifest (your namespace), write an Application manifest pointing to an nginx container, apply both, and verify the container is running. You'll use only `shrine generate` and `shrine deploy`.

## Step 1: create a workspace

```bash
mkdir -p ~/shrine-demo && cd ~/shrine-demo
```

## Step 2: generate a Team manifest

```bash
shrine generate team my-team --path .
```

Shrine writes `my-team.yml`. Edit `contact` and quotas as needed; the defaults are fine for this walkthrough. Register the team so the platform knows about it:

```bash
shrine apply teams --path .
```

## Step 3: generate an Application manifest

```bash
shrine generate application hello \
  --image nginx:alpine --port 80 \
  --domain hello.localhost --team my-team --path .
```

Shrine writes `hello.yml`:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: hello
  owner: my-team
spec:
  image: nginx:alpine
  port: 80
  replicas: 1
  routing:
    domain: hello.localhost
    pathPrefix: /hello
  networking:
    exposeToPlatform: false
```

## Step 4: dry-run the apply

Preview what Shrine would do without touching Docker (Constitution Principle II):

```bash
shrine deploy --dry-run --path .
```

```text
[shrine] Planning deployment from: /home/user/shrine-demo
[dry-run] create network shrine.my-team.private
[dry-run] create container my-team.hello (nginx:alpine, port 80)
```

## Step 5: apply for real

```bash
shrine deploy --path .
```

Shrine creates the team network, pulls the image if needed, and starts the container.

## Step 6: verify

```bash
shrine status app hello
docker ps --filter name=hello
```

## Step 7: tear down

Shrine tears down by team — stops and removes every container and network the team owns:

```bash
shrine teardown my-team
```

## What's next

- [CLI reference](/cli/) — every verb and flag documented.
- [Guides](/guides/) — production patterns including Traefik routing, aliases, and TLS.
