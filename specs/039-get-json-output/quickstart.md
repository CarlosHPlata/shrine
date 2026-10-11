# Quickstart: verifying `shrine get … --json`

**Feature**: 039-get-json-output

Manual end-to-end check on a host with Docker and `jq`. The automated gate is
the integration suite in CI (`TestGetNoDocker`, `TestGetDocker`,
`TestPinnedVersionQueries`); locally, compile it only.

## 1. Build and unit-test

```bash
make build
```

```bash
go test ./...
```

```bash
go vet -tags integration ./tests/integration/...
```

## 2. Teams

```bash
./shrine get teams --json | jq '.data[] | {name, maxApps}'
```

Expect one object per row of `shrine get teams`; `maxApps` and `maxResources`
are numbers (`jq '.data[0].maxApps | type'` prints `"number"`).

## 3. Workloads match the table and `describe`

```bash
./shrine get deployed --json | jq -r '.data[] | [.team, .name, .kind, .version] | @tsv'
```

The rows equal `shrine get deployed`'s TEAM, NAME, KIND and VERSION columns, in
the same order (a `-` in the table is an empty field here).

```bash
./shrine get apps --json | jq '.data[] | select(.name == "whoami")'
```

Compare against `shrine describe app whoami`: same `image`, `pullPolicy`, the
`pin.reference` and `pin.version` from the `Pinned:` line, `runningImage` from
`Running image:`, the full `Container ID:`, and the full config hash (describe
shows its first sixteen characters).

## 4. Filtering and kinds

```bash
./shrine get resources --team sandbox --json | jq '[.data[].kind] | unique'
```

Expect `["Resource"]`, and only team `sandbox`'s entries.

## 5. Without the container runtime

```bash
DOCKER_HOST=tcp://127.0.0.1:1 ./shrine get deployed --json | jq '[.data[].runningImage] | unique'
```

Expect exit 0 and `[null]`; every other field is still filled.

## 6. Empty, unsupported, unchanged

```bash
./shrine get deployed --json --state-dir "$(mktemp -d)"
```

Expect `{"data": []}` (indented) and exit 0.

```bash
./shrine get config-dir --json
```

Expect `Error: unknown flag: --json` and exit 1.

```bash
./shrine get deployed
```

Expect today's table, unchanged.
