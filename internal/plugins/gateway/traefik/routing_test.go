package traefik

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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
	return &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: engine.NoopObserver{}}
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
	if strings.Contains(got, "websecure") {
		t.Error("T025: expected no websecure in non-TLS alias output")
	}
	if strings.Contains(got, "tls:") {
		t.Error("T025: expected no tls: in non-TLS alias output")
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
	if strings.Contains(got, "websecure") {
		t.Error("T025: expected no websecure in non-TLS alias output")
	}
	if strings.Contains(got, "tls:") {
		t.Error("T025: expected no tls: in non-TLS alias output")
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
	if strings.Contains(got, "websecure") {
		t.Error("T025: expected no websecure in non-TLS alias output")
	}
	if strings.Contains(got, "tls:") {
		t.Error("T025: expected no tls: in non-TLS alias output")
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
	if strings.Contains(got, "websecure") {
		t.Error("T025: expected no websecure in non-TLS alias output")
	}
	if strings.Contains(got, "tls:") {
		t.Error("T025: expected no tls: in non-TLS alias output")
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
	if strings.Contains(got, "websecure") {
		t.Error("T025: expected no websecure in non-TLS alias output")
	}
	if strings.Contains(got, "tls:") {
		t.Error("T025: expected no tls: in non-TLS alias output")
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
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}

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
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}

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
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}

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
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}

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
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}

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
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}

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
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}

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

// stubReadFileWebsecurePresent stubs readFileFn to return YAML with both web and websecure entrypoints.
func stubReadFileWebsecurePresent(t *testing.T) {
	t.Helper()
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = func(string) ([]byte, error) {
		return []byte("entryPoints:\n  web:\n    address: \":80\"\n  websecure:\n    address: \":443\"\n"), nil
	}
}

// stubReadFileWebsecureMissing stubs readFileFn to return YAML with only the web entrypoint.
func stubReadFileWebsecureMissing(t *testing.T) {
	t.Helper()
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = func(string) ([]byte, error) {
		return []byte("entryPoints:\n  web:\n    address: \":80\"\n"), nil
	}
}

// T011: alias with TLS=true must produce entryPoints:[web,websecure] and a tls:{} block.
// The primary-domain router must keep entryPoints:[web] with no tls key.
func TestWriteRoute_AliasWithTLS_AddsWebsecureAndTLSBlock(t *testing.T) {
	stubLstatNotExist(t)
	stubReadFileWebsecurePresent(t)
	captured := captureWriteFileFn(t)

	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: engine.NoopObserver{}}
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "alias.shrine.lab", TLS: true},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc struct {
		HTTP httpConfig `yaml:"http"`
	}
	if err := yaml.Unmarshal(*captured, &doc); err != nil {
		t.Fatalf("unmarshal failed: %v\nYAML:\n%s", err, *captured)
	}

	aliasRouter, ok := doc.HTTP.Routers["team-a-hello-api-alias-0"]
	if !ok {
		t.Fatalf("alias router key 'team-a-hello-api-alias-0' not found in routers")
	}
	if len(aliasRouter.EntryPoints) != 2 || aliasRouter.EntryPoints[0] != "web" || aliasRouter.EntryPoints[1] != "websecure" {
		t.Errorf("alias router entryPoints: got %v, want [web websecure]", aliasRouter.EntryPoints)
	}
	if aliasRouter.TLS == nil {
		t.Error("alias router: expected tls block to be present, got nil")
	}

	primary, ok := doc.HTTP.Routers["team-a-hello-api"]
	if !ok {
		t.Fatalf("primary router key 'team-a-hello-api' not found")
	}
	if len(primary.EntryPoints) != 1 || primary.EntryPoints[0] != "web" {
		t.Errorf("primary router entryPoints: got %v, want [web]", primary.EntryPoints)
	}
	if primary.TLS != nil {
		t.Error("primary router: expected no tls block, got non-nil")
	}
}

// T012: primary-domain router must never gain a TLS block regardless of alias mix.
func TestWriteRoute_AliasWithTLS_PreservesPrimaryRouterShape(t *testing.T) {
	stubLstatNotExist(t)
	stubReadFileWebsecurePresent(t)
	captured := captureWriteFileFn(t)

	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: engine.NoopObserver{}}
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "tls-alias.lab", TLS: true},
		{Host: "plain-alias.lab", TLS: false},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc struct {
		HTTP httpConfig `yaml:"http"`
	}
	if err := yaml.Unmarshal(*captured, &doc); err != nil {
		t.Fatalf("unmarshal failed: %v\nYAML:\n%s", err, *captured)
	}

	primary, ok := doc.HTTP.Routers["team-a-hello-api"]
	if !ok {
		t.Fatalf("primary router key 'team-a-hello-api' not found")
	}
	if len(primary.EntryPoints) != 1 || primary.EntryPoints[0] != "web" {
		t.Errorf("primary router entryPoints: got %v, want [web]", primary.EntryPoints)
	}
	if primary.TLS != nil {
		t.Error("primary router: must not have a tls block regardless of alias TLS flags")
	}
}

// T013: when static config lacks websecure, a warning is emitted for TLS-enabled aliases.
func TestWriteRoute_AliasWithTLS_EmitsWarning_WhenStaticConfigLacksWebsecure(t *testing.T) {
	stubLstatNotExist(t)
	stubReadFileWebsecureMissing(t)
	captureWriteFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "a.example.com", TLS: true},
		{Host: "b.example.com", PathPrefix: "/x", TLS: true},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var warningEvents []engine.Event
	for _, ev := range rec.events {
		if ev.Name == "gateway.alias.tls_no_websecure" {
			warningEvents = append(warningEvents, ev)
		}
	}
	if len(warningEvents) != 1 {
		t.Fatalf("expected exactly 1 gateway.alias.tls_no_websecure event, got %d: %+v", len(warningEvents), rec.events)
	}
	ev := warningEvents[0]
	if ev.Status != engine.StatusWarning {
		t.Errorf("expected StatusWarning, got %q", ev.Status)
	}
	if ev.Fields["team"] != "team-a" {
		t.Errorf("expected team=team-a, got %q", ev.Fields["team"])
	}
	if ev.Fields["name"] != "hello-api" {
		t.Errorf("expected name=hello-api, got %q", ev.Fields["name"])
	}
	if ev.Fields["path"] != "/fake/traefik.yml" {
		t.Errorf("expected path=/fake/traefik.yml, got %q", ev.Fields["path"])
	}
	wantTLSAliases := "a.example.com,b.example.com+/x"
	if ev.Fields["tls_aliases"] != wantTLSAliases {
		t.Errorf("expected tls_aliases=%q, got %q", wantTLSAliases, ev.Fields["tls_aliases"])
	}
	if !strings.Contains(ev.Fields["hint"], "websecure") {
		t.Errorf("hint should contain 'websecure', got %q", ev.Fields["hint"])
	}
}

// T014: no warning emitted when websecure IS present and aliases use TLS.
func TestWriteRoute_AliasWithTLS_NoWarning_WhenWebsecurePresent(t *testing.T) {
	stubLstatNotExist(t)
	stubReadFileWebsecurePresent(t)
	captureWriteFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "a.example.com", TLS: true},
		{Host: "b.example.com", PathPrefix: "/x", TLS: true},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, ev := range rec.events {
		if ev.Name == "gateway.alias.tls_no_websecure" {
			t.Errorf("unexpected gateway.alias.tls_no_websecure event when websecure is present: %+v", ev)
		}
	}
}

// T015: no warning emitted when no alias has TLS=true, regardless of static config.
func TestWriteRoute_NoAliasHasTLS_NoWarning_RegardlessOfStaticConfig(t *testing.T) {
	stubLstatNotExist(t)
	stubReadFileWebsecureMissing(t)
	captureWriteFileFn(t)

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "a.example.com", TLS: false},
		{Host: "b.example.com", PathPrefix: "/p", TLS: false},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, ev := range rec.events {
		if ev.Name == "gateway.alias.tls_no_websecure" {
			t.Errorf("unexpected gateway.alias.tls_no_websecure event when no alias has TLS=true: %+v", ev)
		}
	}
}

func TestWriteRoute_AliasWithTLS_EmitsProbeError_WhenStaticConfigUnreadable(t *testing.T) {
	stubLstatNotExist(t)
	captureWriteFileFn(t)

	origRead := readFileFn
	t.Cleanup(func() { readFileFn = origRead })
	readFileFn = func(string) ([]byte, error) {
		return []byte("entryPoints:\n  web: ::: not yaml :::\n"), nil
	}

	rec := &recordingObserver{}
	rb := &RoutingBackend{routingDir: "/fake", staticConfigPath: "/fake/traefik.yml", observer: rec}
	op := baseOp()
	op.AdditionalRoutes = []engine.AliasRoute{
		{Host: "a.example.com", TLS: true},
	}

	if err := rb.WriteRoute(op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var probeErrors []engine.Event
	for _, ev := range rec.events {
		if ev.Name == "gateway.config.tls_port_probe_error" {
			probeErrors = append(probeErrors, ev)
		}
		if ev.Name == "gateway.alias.tls_no_websecure" {
			t.Errorf("must not emit gateway.alias.tls_no_websecure on probe error: %+v", ev)
		}
	}
	if len(probeErrors) != 1 {
		t.Fatalf("expected exactly 1 gateway.config.tls_port_probe_error event, got %d: %+v", len(probeErrors), rec.events)
	}
	ev := probeErrors[0]
	if ev.Status != engine.StatusWarning {
		t.Errorf("expected StatusWarning, got %q", ev.Status)
	}
	if ev.Fields["path"] != "/fake/traefik.yml" {
		t.Errorf("expected path=/fake/traefik.yml, got %q", ev.Fields["path"])
	}
	if ev.Fields["error"] == "" {
		t.Error("expected non-empty error field on probe-error event")
	}
}
