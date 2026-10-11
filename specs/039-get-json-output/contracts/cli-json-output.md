# Contract: `shrine get … --json`

**Feature**: 039-get-json-output

## Command surface

| Command (aliases) | `--json` | Entry type |
|-------------------|----------|------------|
| `shrine get deployed` | accepted | workload |
| `shrine get applications` (`apps`) | accepted | workload, `kind` = `Application` |
| `shrine get resources` (`res`) | accepted | workload, `kind` = `Resource` |
| `shrine get teams` (`team`) | accepted | team |
| `shrine get config-dir` | rejected: `Error: unknown flag: --json`, exit 1 | — |
| `shrine get state-dir` | rejected: `Error: unknown flag: --json`, exit 1 | — |

`--team/-t` combines with `--json` and filters exactly as for the table. Flag
help text: `--json   Print the listing as JSON instead of a table`.

## Streams and exit codes

| Situation | stdout | stderr | exit |
|-----------|--------|--------|------|
| Listing with entries | one JSON document, trailing newline | empty | 0 |
| Nothing to list | `{"data": []}` (indented form below), trailing newline | empty | 0 |
| Container runtime unreachable / container gone | full document; affected `runningImage` are `null` | empty | 0 |
| State cannot be read | empty | `Error: <cause>` | 1 |
| Without `--json` | today's table, byte for byte | as today | as today |

## Workload document

```json
{
  "data": [
    {
      "team": "sandbox",
      "name": "whoami",
      "kind": "Application",
      "image": "reg:falcon/whoami",
      "pullPolicy": "Pinned",
      "version": "v1.11.0@4f90b33ddca9",
      "pin": {
        "reference": "192.168.1.206:5000/whoami@sha256:4f90b33ddca9c4d4f06527070d6e503b16d71016edea036842be2a84e60c91cb",
        "version": "v1.11.0@4f90b33ddca9",
        "pinnedAt": "2026-10-11T14:03:22Z"
      },
      "runningImage": "192.168.1.206:5000/whoami@sha256:4f90b33ddca9c4d4f06527070d6e503b16d71016edea036842be2a84e60c91cb",
      "containerId": "15a9b7eafaaa26e092964ee66bfaf5a8db4abece0211ec9dd45d4043a39bafad",
      "configHash": "b00e074e6e5866f6c3a1d29e7b05f48a1c6e2d9b30f7a4e85d16c2b9f0e3a7d4"
    },
    {
      "team": "sandbox",
      "name": "cache",
      "kind": "Resource",
      "image": "redis:7.4",
      "pullPolicy": "IfNotPresent",
      "version": "redis:7.4",
      "pin": null,
      "runningImage": null,
      "containerId": "7a3e5c9b1d2f8e04a6b3c5d7e9f1a2b4c6d8e0f2a4b6c8d0e2f4a6b8c0d2e4f6",
      "configHash": "5e1f…"
    }
  ]
}
```

Rules:
- Every key is present in every entry. Keys appear in the order shown.
- `pin` is `null` unless `pullPolicy` is `Pinned` and a matching pin is on record.
- `runningImage` is `null` exactly when the runtime could not report it.
- Strings that the table or `describe` show as `-` are `""`.
- `version` equals the table's VERSION cell (with `-` → `""`); `pin.version` equals the readable form `describe` prints in parentheses after `Pinned:`.

## Team document

```json
{
  "data": [
    {
      "name": "sandbox",
      "displayName": "Sandbox Team",
      "maxApps": 5,
      "maxResources": 3
    }
  ]
}
```

## Empty document

```json
{
  "data": []
}
```

## Encoding

- UTF-8, two-space indentation, one trailing newline.
- `&`, `<`, `>` are written literally; quotes, backslashes and control
  characters are escaped as JSON requires.
- Scripts must not depend on whitespace or indentation.
