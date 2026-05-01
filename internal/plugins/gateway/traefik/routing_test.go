package traefik

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

// stubLstatNotExist makes lstatFn behave as if the path is absent.
func stubLstatNotExist(t *testing.T) {
	t.Helper()
	orig := lstatFn
	t.Cleanup(func() { lstatFn = orig })
	lstatFn = func(path string) (os.FileInfo, error) {
		return nil, &fs.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
	}
}

// stubLstatPresent makes lstatFn behave as if the path exists.
func stubLstatPresent(t *testing.T) {
	t.Helper()
	orig := lstatFn
	t.Cleanup(func() { lstatFn = orig })
	lstatFn = func(string) (os.FileInfo, error) { return nil, nil }
}

// stubLstatError makes lstatFn return a non-IsNotExist error.
func stubLstatError(t *testing.T, err error) {
	t.Helper()
	orig := lstatFn
	t.Cleanup(func() { lstatFn = orig })
	lstatFn = func(string) (os.FileInfo, error) { return nil, err }
}

func TestBuildRouterRule(t *testing.T) {
	if got := buildRouterRule("h", ""); got != "Host(`h`)" {
		t.Errorf("buildRouterRule(h,''): got %q", got)
	}
	if got := buildRouterRule("h", "/p"); got != "Host(`h`) && PathPrefix(`/p`)" {
		t.Errorf("buildRouterRule(h,/p): got %q", got)
	}
}

func captureWriteFileFn(t *testing.T) *[]byte {
	t.Helper()
	var captured []byte
	origWrite := writeFileFn
	origMkdir := mkdirAllFn
	writeFileFn = func(_ string, data []byte, _ fs.FileMode) error {
		captured = data
		return nil
	}
	mkdirAllFn = func(_ string, _ fs.FileMode) error { return nil }
	t.Cleanup(func() {
		writeFileFn = origWrite
		mkdirAllFn = origMkdir
	})
	return &captured
}

func newTestBackend() *RoutingBackend {
	return &RoutingBackend{routingDir: "/fake", observer: engine.NoopObserver{}}
}

func baseOp() engine.WriteRouteOp {
	return engine.WriteRouteOp{
		Team:        "team-a",
		Domain:      "hello-api.home.lab",
		ServiceName: "hello-api",
		ServicePort: 8080,
	}
}

const wantNoAlias = `http:
    routers:
        team-a-hello-api:
            rule: Host(` + "`hello-api.home.lab`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
    services:
        team-a-hello-api:
            loadBalancer:
                servers:
                    - url: http://team-a.hello-api:8080
`

const wantOneAliasStrip = `http:
    middlewares:
        team-a-hello-api-strip-0:
            stripPrefix:
                prefixes:
                    - /p
    routers:
        team-a-hello-api:
            rule: Host(` + "`hello-api.home.lab`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
        team-a-hello-api-alias-0:
            rule: Host(` + "`x`" + `) && PathPrefix(` + "`/p`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
            middlewares:
                - team-a-hello-api-strip-0
    services:
        team-a-hello-api:
            loadBalancer:
                servers:
                    - url: http://team-a.hello-api:8080
`

const wantOneAliasNoStrip = `http:
    routers:
        team-a-hello-api:
            rule: Host(` + "`hello-api.home.lab`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
        team-a-hello-api-alias-0:
            rule: Host(` + "`x`" + `) && PathPrefix(` + "`/p`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
    services:
        team-a-hello-api:
            loadBalancer:
                servers:
                    - url: http://team-a.hello-api:8080
`

const wantHostOnlyAlias = `http:
    routers:
        team-a-hello-api:
            rule: Host(` + "`hello-api.home.lab`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
        team-a-hello-api-alias-0:
            rule: Host(` + "`x`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
    services:
        team-a-hello-api:
            loadBalancer:
                servers:
                    - url: http://team-a.hello-api:8080
`

const wantThreeAliasesSparse = `http:
    middlewares:
        team-a-hello-api-strip-1:
            stripPrefix:
                prefixes:
                    - /p1
    routers:
        team-a-hello-api:
            rule: Host(` + "`hello-api.home.lab`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
        team-a-hello-api-alias-0:
            rule: Host(` + "`a`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
        team-a-hello-api-alias-1:
            rule: Host(` + "`b`" + `) && PathPrefix(` + "`/p1`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
            middlewares:
                - team-a-hello-api-strip-1
        team-a-hello-api-alias-2:
            rule: Host(` + "`c`" + `) && PathPrefix(` + "`/p2`" + `)
            service: team-a-hello-api
            entryPoints:
                - web
    services:
        team-a-hello-api:
            loadBalancer:
                servers:
                    - url: http://team-a.hello-api:8080
`

func TestWriteRoute_NoAliases(t *testing.T) {
	stubLstatNotExist(t)
	captured := captureWriteFileFn(t)
	rb := newTestBackend()
	op := baseOp()

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(*captured)
	if got != wantNoAlias {
		t.Errorf("YAML mismatch.\ngot:\n%s\nwant:\n%s", got, wantNoAlias)
	}
	if strings.Contains(got, "middlewares:") {
		t.Error("expected no middlewares section")
	}
	if strings.Count(got, "routers:") != 1 {
		t.Error("expected exactly one routers block")
	}
	if !strings.Contains(got, "team-a-hello-api:") {
		t.Error("expected primary router key")
	}
}

func TestWriteRoute_OneAlias_Strip(t *testing.T) {
	stubLstatNotExist(t)
	captured := captureWriteFileFn(t)
	rb := newTestBackend()
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "x", PathPrefix: "/p", StripPrefix: true},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(*captured)
	if got != wantOneAliasStrip {
		t.Errorf("YAML mismatch.\ngot:\n%s\nwant:\n%s", got, wantOneAliasStrip)
	}
	if !strings.Contains(got, "team-a-hello-api-strip-0:") {
		t.Error("expected strip middleware named team-a-hello-api-strip-0")
	}
	if !strings.Contains(got, "team-a-hello-api-alias-0:") {
		t.Error("expected alias router named team-a-hello-api-alias-0")
	}
	if !strings.Contains(got, "- team-a-hello-api-strip-0") {
		t.Error("expected alias router to list strip middleware")
	}
}

func TestWriteRoute_OneAlias_NoStrip(t *testing.T) {
	stubLstatNotExist(t)
	captured := captureWriteFileFn(t)
	rb := newTestBackend()
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "x", PathPrefix: "/p", StripPrefix: false},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(*captured)
	if got != wantOneAliasNoStrip {
		t.Errorf("YAML mismatch.\ngot:\n%s\nwant:\n%s", got, wantOneAliasNoStrip)
	}
	if strings.Contains(got, "middlewares:") {
		t.Error("expected no middlewares section when StripPrefix=false")
	}
	if !strings.Contains(got, "team-a-hello-api-alias-0:") {
		t.Error("expected alias router")
	}
}

func TestWriteRoute_HostOnlyAlias(t *testing.T) {
	stubLstatNotExist(t)
	captured := captureWriteFileFn(t)
	rb := newTestBackend()
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "x", PathPrefix: ""},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(*captured)
	if got != wantHostOnlyAlias {
		t.Errorf("YAML mismatch.\ngot:\n%s\nwant:\n%s", got, wantHostOnlyAlias)
	}
	if !strings.Contains(got, "Host(`x`)") {
		t.Error("expected Host-only rule for alias with no prefix")
	}
	if strings.Contains(got, "middlewares:") {
		t.Error("expected no middlewares section for host-only alias")
	}
}

func TestWriteRoute_ThreeAliases_SparseStrip(t *testing.T) {
	stubLstatNotExist(t)
	captured := captureWriteFileFn(t)
	rb := newTestBackend()
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "a", PathPrefix: "", StripPrefix: false},            // index 0: host-only
		{Host: "b", PathPrefix: "/p1", StripPrefix: true},          // index 1: strip
		{Host: "c", PathPrefix: "/p2", StripPrefix: false},         // index 2: no strip
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(*captured)
	if got != wantThreeAliasesSparse {
		t.Errorf("YAML mismatch.\ngot:\n%s\nwant:\n%s", got, wantThreeAliasesSparse)
	}
	if !strings.Contains(got, "team-a-hello-api-strip-1:") {
		t.Error("expected only strip-1 middleware (sparse)")
	}
	if strings.Contains(got, "team-a-hello-api-strip-0:") {
		t.Error("expected no strip-0 middleware (host-only alias at index 0)")
	}
	if strings.Contains(got, "team-a-hello-api-strip-2:") {
		t.Error("expected no strip-2 middleware (StripPrefix=false at index 2)")
	}
}

const wantPath = "/fake/dynamic/team-a-hello-api.yml"

// T006: when the per-app file already exists, WriteRoute must skip the write
// entirely and emit gateway.route.preserved.
func TestWriteRoute_FilePresent_Preserves(t *testing.T) {
	stubLstatPresent(t)

	writeCalled := false
	origWrite := writeFileFn
	origMkdir := mkdirAllFn
	writeFileFn = func(string, []byte, os.FileMode) error {
		writeCalled = true
		return nil
	}
	mkdirAllFn = func(string, os.FileMode) error { return nil }
	t.Cleanup(func() {
		writeFileFn = origWrite
		mkdirAllFn = origMkdir
	})

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", observer: rec}

	if err := rb.WriteRoute(baseOp()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writeCalled {
		t.Fatal("expected writeFileFn NOT to be called when file is present")
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	ev := rec.events[0]
	if ev.Name != "gateway.route.preserved" {
		t.Errorf("expected event name gateway.route.preserved, got %q", ev.Name)
	}
	if ev.Status != engine.StatusInfo {
		t.Errorf("expected StatusInfo, got %q", ev.Status)
	}
	if ev.Fields["team"] != "team-a" {
		t.Errorf("expected team=team-a, got %q", ev.Fields["team"])
	}
	if ev.Fields["name"] != "hello-api" {
		t.Errorf("expected name=hello-api, got %q", ev.Fields["name"])
	}
	if ev.Fields["path"] != wantPath {
		t.Errorf("expected path=%q, got %q", wantPath, ev.Fields["path"])
	}
}

// T007: a fresh write (file absent) must emit gateway.route.generated after the write succeeds.
func TestWriteRoute_FreshWrite_EmitsGenerated(t *testing.T) {
	stubLstatNotExist(t)
	captured := captureWriteFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", observer: rec}

	if err := rb.WriteRoute(baseOp()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(*captured) != wantNoAlias {
		t.Errorf("YAML mismatch.\ngot:\n%s\nwant:\n%s", *captured, wantNoAlias)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	ev := rec.events[0]
	if ev.Name != "gateway.route.generated" {
		t.Errorf("expected event name gateway.route.generated, got %q", ev.Name)
	}
	if ev.Status != engine.StatusInfo {
		t.Errorf("expected StatusInfo, got %q", ev.Status)
	}
	if ev.Fields["team"] != "team-a" || ev.Fields["name"] != "hello-api" || ev.Fields["path"] != wantPath {
		t.Errorf("unexpected fields: %+v", ev.Fields)
	}
}

// T008: a non-IsNotExist stat error emits gateway.route.stat_error (Warning) and returns nil so the deploy continues.
func TestWriteRoute_StatError_EmitsWarningAndContinues(t *testing.T) {
	stubLstatError(t, errors.New("permission denied"))

	writeCalled := false
	origWrite := writeFileFn
	origMkdir := mkdirAllFn
	writeFileFn = func(string, []byte, os.FileMode) error {
		writeCalled = true
		return nil
	}
	mkdirAllFn = func(string, os.FileMode) error { return nil }
	t.Cleanup(func() {
		writeFileFn = origWrite
		mkdirAllFn = origMkdir
	})

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", observer: rec}

	if err := rb.WriteRoute(baseOp()); err != nil {
		t.Fatalf("expected nil error (deploy must continue), got %v", err)
	}
	if writeCalled {
		t.Fatal("expected writeFileFn NOT to be called on stat error")
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	ev := rec.events[0]
	if ev.Name != "gateway.route.stat_error" {
		t.Errorf("expected event name gateway.route.stat_error, got %q", ev.Name)
	}
	if ev.Status != engine.StatusWarning {
		t.Errorf("expected StatusWarning, got %q", ev.Status)
	}
	if !strings.Contains(ev.Fields["error"], "permission denied") {
		t.Errorf("expected error field to contain 'permission denied', got %q", ev.Fields["error"])
	}
}

// T013: after a previously-Generated state, an operator rm puts the file in Absent.
// The next WriteRoute must regenerate the file from the *current* manifest, not any cached prior state.
func TestWriteRoute_AbsentAfterPreviousPresent_RegeneratesFromManifest(t *testing.T) {
	stubLstatNotExist(t)
	captured := captureWriteFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", observer: rec}

	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "freshly-added.lab", PathPrefix: ""},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(*captured)
	if !strings.Contains(got, "freshly-added.lab") {
		t.Errorf("expected fresh write to include the new alias, got:\n%s", got)
	}
	if len(rec.events) != 1 || rec.events[0].Name != "gateway.route.generated" {
		t.Errorf("expected single gateway.route.generated event, got %+v", rec.events)
	}
}

// captureRemoveFileFn fails the test if removeFileFn is invoked. RemoveRoute under
// the new orphan-warn policy must NEVER call os.Remove — the operator does the rm by hand.
func captureRemoveFileFn(t *testing.T) {
	t.Helper()
	orig := removeFileFn
	t.Cleanup(func() { removeFileFn = orig })
	removeFileFn = func(path string) error {
		t.Fatalf("removeFileFn must not be called; got path=%q", path)
		return nil
	}
}

// T015: when the per-app file is present at teardown, RemoveRoute emits an orphan
// warning and does NOT delete the file.
func TestRemoveRoute_FilePresent_EmitsOrphanWarning(t *testing.T) {
	stubLstatPresent(t)
	captureRemoveFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", observer: rec}

	if err := rb.RemoveRoute("team-a", "hello-api"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	ev := rec.events[0]
	if ev.Name != "gateway.route.orphan" {
		t.Errorf("expected event name gateway.route.orphan, got %q", ev.Name)
	}
	if ev.Status != engine.StatusWarning {
		t.Errorf("expected StatusWarning, got %q", ev.Status)
	}
	if ev.Fields["team"] != "team-a" || ev.Fields["name"] != "hello-api" {
		t.Errorf("unexpected fields: %+v", ev.Fields)
	}
	if ev.Fields["path"] != wantPath {
		t.Errorf("expected path=%q, got %q", wantPath, ev.Fields["path"])
	}
}

// T016: when the per-app file is absent, RemoveRoute is a silent no-op.
func TestRemoveRoute_FileAbsent_IsNoOp(t *testing.T) {
	stubLstatNotExist(t)
	captureRemoveFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", observer: rec}

	if err := rb.RemoveRoute("team-a", "hello-api"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rec.events) != 0 {
		t.Fatalf("expected 0 events for absent file, got %+v", rec.events)
	}
}

// T017: a non-IsNotExist stat error during RemoveRoute emits a stat_error warning and returns nil.
func TestRemoveRoute_StatError_EmitsWarningAndContinues(t *testing.T) {
	stubLstatError(t, errors.New("permission denied"))
	captureRemoveFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", observer: rec}

	if err := rb.RemoveRoute("team-a", "hello-api"); err != nil {
		t.Fatalf("expected nil error (teardown must continue), got %v", err)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	ev := rec.events[0]
	if ev.Name != "gateway.route.stat_error" {
		t.Errorf("expected event name gateway.route.stat_error, got %q", ev.Name)
	}
	if ev.Status != engine.StatusWarning {
		t.Errorf("expected StatusWarning, got %q", ev.Status)
	}
	if !strings.Contains(ev.Fields["error"], "permission denied") {
		t.Errorf("expected error field to contain 'permission denied', got %q", ev.Fields["error"])
	}
}
