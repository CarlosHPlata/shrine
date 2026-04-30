package traefik

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

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
	return &RoutingBackend{routingDir: "/fake"}
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
