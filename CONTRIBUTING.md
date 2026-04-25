# Contributing to Shrine

Thanks for taking the time to contribute. This document covers how to set up your environment, the commit style we follow, and the PR process.

## Development setup

**Prerequisites:** Go 1.24+, Docker (running locally for integration tests)

```bash
git clone https://github.com/CarlosHPlata/shrine.git
cd shrine
go build ./...
go test ./...
```

## Branching

Branch off `main` using one of these prefixes:

| Prefix | When to use |
|---|---|
| `feat/` | New feature or behaviour |
| `fix/` | Bug fix |
| `docs/` | Documentation only |
| `refactor/` | Code change with no behaviour change |
| `chore/` | Tooling, deps, CI |

Example: `feat/adguard-bulk-dns`

## Commit messages

We follow [Conventional Commits](https://www.conventionalcommits.org/). This is what GoReleaser uses to generate release notes, so consistency matters.

```
<type>: <short summary in present tense>

# Types: feat, fix, docs, refactor, test, chore
# Breaking change: add ! after the type — feat!: rename deploy flags
```

Examples:

```
feat: add --dry-run flag to teardown command
fix: prevent duplicate subnet allocation on redeploy
docs: add networking model section to README
chore: bump docker SDK to v28
```

Keep the subject line under 72 characters. A body is optional but welcome for non-obvious changes.

## Opening a pull request

1. Open an issue first for anything beyond a trivial fix — alignment before implementation saves time.
2. One logical change per PR. Split unrelated changes.
3. All CI checks must pass before review.
4. Fill in the PR template — especially the testing and breaking change sections.

## Running tests

```bash
# All tests
go test ./...

# A specific package
go test ./internal/planner/...

# With race detector
go test -race ./...
```

There is no test database or external service required. Tests that need Docker use the dry-run engine.

## Reporting bugs

Use the [bug report template](https://github.com/CarlosHPlata/shrine/issues/new?template=bug_report.md). Include `shrine version` output and the full error message.
