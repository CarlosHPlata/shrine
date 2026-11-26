# Shared container spec

## Story
As a shrine maintainer, I want the container properties that Applications and Resources share defined once, so that a new container property (like the recent `spec.command`) is declared in one place instead of two.

## Acceptance criteria
- [ ] Given any Application or Resource manifest that loads today, when it's loaded after the refactor, then it parses to the same values with no YAML changes.
- [ ] Given any valid or invalid Application or Resource manifest, when it's validated or deployed, then the result (errors, plan, running containers) is identical to before.
- [ ] Given the existing unit and integration tests, when they run after the refactor, then they pass with only mechanical edits to how test specs are built, and no expected values change.

## Plan
- Define one shared container block with image, port, dependencies, env, networking, volumes, command and imagePullPolicy. It uses the field names and tags that exist today, with `image` marked `omitempty`, as it already is on Resource.
- Embed it inline in both specs, so the YAML shape and field names don't change. Application keeps replicas and routing; Resource keeps type, version and outputs.
- Code that reads these fields keeps working unchanged through the embedded struct. Only the places that build the specs as struct literals (mostly tests) need rewriting.
- Leave every validator as it is.
- Confirm with the unit tests plus one targeted integration test that covers both kinds.

## Technical details

### Current state
- Both specs live in `internal/manifest/types.go`: `ApplicationSpec` (line 178) and `ResourceSpec` (line 203).
- The fields to extract are identical in both, with the same Go types and yaml keys: `Image string`, `Port int`, `Dependencies []Dependency`, `Env []EnvVar`, `Networking Networking`, `Volumes []VolumeMount`, `Command Command`, `ImagePullPolicy string`.
- There is one tag difference: Application uses `yaml:"image"` and Resource uses `yaml:"image,omitempty"`. **Decision: use `image,omitempty`.** It was verified to have no effect on parsing: `omitempty` only applies when marshaling. A missing `image` still parses to `""` and is still rejected by `validateApplicationSpec` (`internal/manifest/validate.go:78`).
- Fields that stay per kind:
  - Application: `Replicas`, `Routing`.
  - Resource: `Type`, `Version`, `Outputs`.

### Target shape
```go
type ContainerSpec struct {
    Image           string        `yaml:"image,omitempty"`
    Port            int           `yaml:"port,omitempty"`
    Dependencies    []Dependency  `yaml:"dependencies,omitempty"`
    Env             []EnvVar      `yaml:"env,omitempty"`
    Networking      Networking    `yaml:"networking,omitempty"`
    Volumes         []VolumeMount `yaml:"volumes,omitempty"`
    Command         Command       `yaml:"command,omitempty"`
    ImagePullPolicy string        `yaml:"imagePullPolicy,omitempty"`
}

type ApplicationSpec struct {
    ContainerSpec `yaml:",inline"`
    Replicas int     `yaml:"replicas,omitempty"`
    Routing  Routing `yaml:"routing,omitempty"`
}

type ResourceSpec struct {
    ContainerSpec `yaml:",inline"`
    Type    string   `yaml:"type"`
    Version string   `yaml:"version"`
    Outputs []Output `yaml:"outputs,omitempty"`
}
```

### Why this is safe
- **YAML parsing:** `internal/manifest/parser.go` uses plain `yaml.Unmarshal`, with no `KnownFields`/strict mode. yaml.v3 flattens a struct embedded with `,inline` into the parent mapping, so the keys still sit directly under `spec:`.
- **Custom unmarshalers keep working:** `Networking.UnmarshalYAML` and `Command.UnmarshalYAML` are on the field types, so they still run when inlined. **Do not add an `UnmarshalYAML` method to `ContainerSpec` itself:** yaml.v3 doesn't support inline structs that have their own unmarshaler.
- **Key names:** the inlined fields don't share a yaml key with any field left on either spec.
- **Field reads:** promoted fields mean `spec.Image`, `app.Spec.Dependencies`, `res.Spec.ImagePullPolicy` and the like compile unchanged. These reads are in `internal/handler/bump.go`, `internal/handler/deploy_plan_format.go`, `internal/planner/*`, `internal/resolver/*` and `internal/engine/engine.go`, and none of them need edits.
- **No marshaling of specs in production code:** the only production `yaml.Marshal` calls are in `internal/plugins/gateway/traefik/`, which writes its own config structs. `shrine get --json` (`internal/handler/json_listing.go`) encodes dedicated listing entry types, not specs. The integration helper `tests/integration/testutils/manifest_file.go` marshals a generic `map[string]any`, not these types.
- **No reflection** over these types in non-test code.

### What breaks (mechanical fixes)
Composite literals that set the shared fields directly stop compiling, for example:

```go
// before
manifest.ApplicationSpec{Image: "nginx", Port: 80, Replicas: 2}
// after
manifest.ApplicationSpec{ContainerSpec: manifest.ContainerSpec{Image: "nginx", Port: 80}, Replicas: 2}
```

Literals that only set per-kind fields, or that assign fields one by one after construction, keep compiling. Let `go build ./... && go vet ./...` find the full list.

`ApplicationSpec{`/`ResourceSpec{` literals per file. All are in test files; production code has none:

| File | Count |
|---|---|
| internal/planner/resolve_test.go | 44 |
| internal/resolver/resolver_test.go | 27 |
| internal/manifest/validate_test.go | 15 |
| internal/planner/plan_test.go | 10 |
| internal/planner/enrich_valuefrom_test.go | 8 |
| internal/planner/enrich_test.go | 7 |
| internal/planner/order_test.go | 6 |
| internal/planner/policy_test.go | 3 |
| internal/planner/resolve_publish_test.go, filter_test.go, internal/manifest/validate_publish_test.go, internal/handler/deploy_plan_format_test.go, bump_test.go, internal/engine/engine_resolve_test.go, engine_publish_test.go, engine_command_test.go | 2 each |
| internal/planner/templates_test.go, hostports_test.go, collisions_test.go, internal/engine/engine_test.go | 1 each |

### Validators: do not touch
`validateApplicationSpec` and `validateResourceSpec` in `internal/manifest/validate.go` keep their bodies, error messages and order exactly as they are. They read the promoted fields, so they compile unchanged.

### Verification
- `go build ./... && go vet ./... && go test ./...`
- Integration: run **only** the targeted test, never the full suite. `TestDeployCommand` covers both an Application and a Resource:
  `go test -tags integration -v -timeout 5m ./tests/integration/ -run TestDeployCommand`
- After changing code, run `graphify update .` (project rule in CLAUDE.md).

## Out of scope
- Any change to validation: rules, messages or order.
- Required fields stay enforced by each kind's validator (e.g. Application still requires image and port).
- Merging other per-kind properties (replicas, routing, type, version, outputs).
- Changes to YAML field names, parsing behavior or defaults.

## Suggestions (your call)
- Do the test-literal rewrite in its own commit, so the actual type change is easy to review.
