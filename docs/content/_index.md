---
title: "Shrine"
description: "Declarative, kubectl-style infrastructure for homelabs and small teams."
toc: false
---

## About Shrine

Shrine is a CLI tool for deploying and orchestrating infrastructure through a single Docker agent. Inspired by kubectl, it lets you define your infrastructure in YAML manifests with a declarative workflow — all the power of Kubernetes without running an actual cluster. Perfect for homelabs and small teams.

## Get started

- **[Install](getting-started/install/)** — Download and verify the Shrine CLI
- **[Quick Start](getting-started/quick-start/)** — Create and deploy your first app
- **[CLI Reference](cli/)** — Complete command reference

## What you get

- **Declarative manifests** — Define teams, resources, and applications as YAML. Git-friendly and fully version-controlled.
- **kubectl-style CLI** — Familiar verb-resource syntax: `shrine apply teams`, `shrine status app <name>`, `shrine describe resource <name>`.
- **Traefik-backed routing** — Automatic HTTP/HTTPS exposure of your apps with pluggable gateway configuration.
- **Dry-run for everything** — Preview any operation with `--dry-run` before making changes.
