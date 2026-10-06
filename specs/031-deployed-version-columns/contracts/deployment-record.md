# Contract: Deployment Record

**Feature**: 031-deployed-version-columns | Binding: design section 3.3, TD-4

## Store interface — unchanged

```go
type DeploymentStore interface {
	Record(team string, deployment Deployment) error
	Remove(team string, name string) error
	List(team string) ([]Deployment, error)
}
```

## Record — extended

```go
type Deployment struct {
	Kind, Name, ContainerID, ConfigHash string
	Image  string // manifest's reference as written (alias unexpanded); "" when unknown
	Policy string // effective pull policy at deploy time; "" when unknown
}
```

## File `<state-dir>/<team>/deployments.txt`

- One line per artifact: `Kind Name ContainerID ConfigHash Image Policy`, single
  spaces, sorted by `Name`, written whole and atomically.
- The writer always emits six fields. An empty optional value (`ConfigHash`,
  `Image`, `Policy`) is written as `-` and read back as empty, so later fields
  keep their position.
- The reader splits on whitespace, ignores `#` comments and blank lines, skips
  lines with fewer than three fields, and reads fields four to six as optional:
  absent or `-` means empty.
- Image references never contain spaces; this is what keeps the line unambiguous.

## Guarantees

| Guarantee | Verified by |
|---|---|
| A four-field line loads with `Image == ""` and `Policy == ""` | unit test on `loadTeam` through injected read |
| A three-field line loads with `ConfigHash`, `Image`, `Policy` empty | same |
| A six-field line round-trips through `Record` and `List` | unit test through injected write and read |
| `Record` of a fully populated deployment writes six fields in order | unit test on the written bytes |
| A record with an empty `ConfigHash` and a set `Image` and `Policy` writes `-` in the hash position and round-trips with every field in place | unit test on the written bytes and on `List` |
| A legacy line re-saved by another record's write keeps loading | unit test: legacy line present, `Record` another, `List` both |
| The record is written only after the container started | existing backend unit tests; CI integration suites |
| A redeploy that finds the container up to date rewrites the record with `Image` and `Policy` | backend unit test on the up-to-date path; `TestGetDocker` legacy scenario in CI |
