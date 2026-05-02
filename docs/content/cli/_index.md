---
title: "CLI Reference"
description: "Auto-generated reference for every shrine subcommand."
weight: 20
cascade:
  type: docs
---

Every Shrine command follows a simple **verb-resource** pattern: the action comes first, then the object. For example: `shrine apply teams`, `shrine status app <name>`, `shrine describe resource <name>`.

Every write operation (`apply`, `deploy`, `teardown`) supports `--dry-run` to preview changes before committing. Read operations do not require `--dry-run`.

The `--team`/`-t` flag is always optional. When provided, it filters results to a single team; when omitted, Shrine searches all teams automatically.

The command pages below are **auto-generated** from `shrine <cmd> --help`. If you notice outdated flags or descriptions, regenerate them with `make docs-gen-cli`.

**Do not edit pages under `/cli/` by hand — they are overwritten on every docs build.**

See the left-hand navigation for the complete command index.
