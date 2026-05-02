package manifest

import (
	"strings"
	"testing"
)

func TestValidate_ValidApplication(t *testing.T) {
	m, err := Parse(testdataPath("hello-api.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if err := Validate(m); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_ValidResource(t *testing.T) {
	m, err := Parse(testdataPath("hello-db.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if err := Validate(m); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_ValidTeam(t *testing.T) {
	m, err := Parse(testdataPath("hello-team.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if err := Validate(m); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_InvalidApplication(t *testing.T) {
	m, err := Parse(testdataPath("invalid-app.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	err = Validate(m)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	msg := err.Error()
	expected := []string{
		"metadata.name is required",
		"metadata.owner is required",
		"spec.image is required",
		"spec.port must be greater than 0",
	}
	for _, e := range expected {
		if !strings.Contains(msg, e) {
			t.Errorf("error missing %q, got: %s", e, msg)
		}
	}
}

func TestValidate_InvalidResource(t *testing.T) {
	m, err := Parse(testdataPath("invalid-resource.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	err = Validate(m)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	msg := err.Error()
	expected := []string{
		"metadata.name is required",
		"metadata.owner is required",
		"spec.type is required",
		"spec.version is required",
	}
	for _, e := range expected {
		if !strings.Contains(msg, e) {
			t.Errorf("error missing %q, got: %s", e, msg)
		}
	}
}

func TestValidate_InvalidKind(t *testing.T) {
	m := &Manifest{
		TypeMeta: TypeMeta{
			Kind:       "Deployment",
			APIVersion: "shrine/v1",
		},
	}

	err := Validate(m)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "kind must be one of: Team, Resource, Application") {
		t.Errorf("error missing kind validation, got: %s", msg)
	}
}

func TestValidate_ResourceOutputRules(t *testing.T) {
	cases := []struct {
		name    string
		outputs []Output
		wantErr string
	}{
		{
			name:    "host with value is rejected",
			outputs: []Output{{Name: "host", Value: "x"}},
			wantErr: "is a CLI built-in and must not set value/generated/template",
		},
		{
			name:    "host with template is rejected",
			outputs: []Output{{Name: "host", Template: "{{.name}}"}},
			wantErr: "is a CLI built-in and must not set value/generated/template",
		},
		{
			name:    "non-host bare output is rejected",
			outputs: []Output{{Name: "mystery"}},
			wantErr: "must set one of value/generated/template",
		},
		{
			name:    "conflicting kinds are rejected",
			outputs: []Output{{Name: "url", Value: "x", Template: "{{.name}}"}},
			wantErr: "value/generated/template are mutually exclusive",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{
				TypeMeta: TypeMeta{Kind: ResourceKind, APIVersion: "shrine/v1"},
				Resource: &ResourceManifest{
					Metadata: Metadata{Name: "r", Owner: "team-a"},
					Spec:     ResourceSpec{Type: "postgres", Version: "16", Outputs: tc.outputs},
				},
			}
			err := Validate(m)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidate_ApplicationEnvRules(t *testing.T) {
	cases := []struct {
		name    string
		env     []EnvVar
		wantErr string
	}{
		{
			name:    "missing fields is rejected",
			env:     []EnvVar{{Name: "X"}},
			wantErr: "must set one of value/valueFrom/template",
		},
		{
			name:    "value and valueFrom is rejected",
			env:     []EnvVar{{Name: "X", Value: "a", ValueFrom: "resource.db.url"}},
			wantErr: "value/valueFrom/template are mutually exclusive",
		},
		{
			name:    "value and template is rejected",
			env:     []EnvVar{{Name: "X", Value: "a", Template: "{{.Y}}"}},
			wantErr: "value/valueFrom/template are mutually exclusive",
		},
		{
			name:    "valueFrom and template is rejected",
			env:     []EnvVar{{Name: "X", ValueFrom: "resource.db.url", Template: "{{.Y}}"}},
			wantErr: "value/valueFrom/template are mutually exclusive",
		},
		{
			name:    "template only is valid",
			env:     []EnvVar{{Name: "X", Template: "{{.Y}}"}},
			wantErr: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{
				TypeMeta: TypeMeta{Kind: ApplicationKind, APIVersion: "shrine/v1"},
				Application: &ApplicationManifest{
					Metadata: Metadata{Name: "a", Owner: "team-a"},
					Spec:     ApplicationSpec{Image: "img", Port: 80, Env: tc.env},
				},
			}
			err := Validate(m)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestValidate_RoutingAliasRules(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	validBase := func(routing Routing) *Manifest {
		return &Manifest{
			TypeMeta: TypeMeta{Kind: ApplicationKind, APIVersion: "shrine/v1"},
			Application: &ApplicationManifest{
				Metadata: Metadata{Name: "a", Owner: "team-a"},
				Spec:     ApplicationSpec{Image: "img", Port: 80, Routing: routing},
			},
		}
	}

	cases := []struct {
		name    string
		routing Routing
		wantErr string
	}{
		{
			name: "V1: aliases set but domain empty",
			routing: Routing{
				Domain:  "",
				Aliases: []RoutingAlias{{Host: "x.example.com"}},
			},
			wantErr: "aliases is set but spec.routing.domain is empty",
		},
		{
			name: "V2: alias with empty host",
			routing: Routing{
				Domain:  "app.home.lab",
				Aliases: []RoutingAlias{{Host: ""}},
			},
			wantErr: "aliases[0].host is required",
		},
		{
			name: "V3: alias host containing a space",
			routing: Routing{
				Domain:  "app.home.lab",
				Aliases: []RoutingAlias{{Host: "bad host"}},
			},
			wantErr: "contains invalid characters",
		},
		{
			name: "V4: alias pathPrefix missing leading slash",
			routing: Routing{
				Domain:  "app.home.lab",
				Aliases: []RoutingAlias{{Host: "x.example.com", PathPrefix: "finances"}},
			},
			wantErr: "must start with \"/\"",
		},
		{
			name: "V5: alias pathPrefix is exactly /",
			routing: Routing{
				Domain:  "app.home.lab",
				Aliases: []RoutingAlias{{Host: "x.example.com", PathPrefix: "/"}},
			},
			wantErr: "must not be just \"/\"",
		},
		{
			name: "V6: alias pathPrefix containing a tab",
			routing: Routing{
				Domain:  "app.home.lab",
				Aliases: []RoutingAlias{{Host: "x.example.com", PathPrefix: "/has\ttab"}},
			},
			wantErr: "contains invalid characters",
		},
		{
			name: "V7: alias vs primary same host+pathPrefix",
			routing: Routing{
				Domain:     "gateway.tail9a6ddb.ts.net",
				PathPrefix: "/finances",
				Aliases:    []RoutingAlias{{Host: "gateway.tail9a6ddb.ts.net", PathPrefix: "/finances"}},
			},
			wantErr: "duplicate route",
		},
		{
			name: "V7: alias vs alias duplicate",
			routing: Routing{
				Domain: "app.home.lab",
				Aliases: []RoutingAlias{
					{Host: "gateway.tail9a6ddb.ts.net", PathPrefix: "/finances"},
					{Host: "gateway.tail9a6ddb.ts.net", PathPrefix: "/finances"},
				},
			},
			wantErr: "alias[1]",
		},
		{
			name: "V7: trailing slash normalized collision",
			routing: Routing{
				Domain:     "app.home.lab",
				PathPrefix: "/x",
				Aliases:    []RoutingAlias{{Host: "app.home.lab", PathPrefix: "/x/"}},
			},
			wantErr: "duplicate route",
		},
		{
			name: "valid alias with explicit stripPrefix false",
			routing: Routing{
				Domain:  "app.home.lab",
				Aliases: []RoutingAlias{{Host: "gw.example.com", PathPrefix: "/api", StripPrefix: boolPtr(false)}},
			},
			wantErr: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validBase(tc.routing)
			err := Validate(m)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateRoutingAliases_TLS_DoesNotAffectCollisionKey(t *testing.T) {
	validBase := func(routing Routing) *Manifest {
		return &Manifest{
			TypeMeta: TypeMeta{Kind: ApplicationKind, APIVersion: "shrine/v1"},
			Application: &ApplicationManifest{
				Metadata: Metadata{Name: "a", Owner: "team-a"},
				Spec:     ApplicationSpec{Image: "img", Port: 80, Routing: routing},
			},
		}
	}

	cases := []struct {
		name    string
		routing Routing
		wantErr string
	}{
		{
			// FR-006: tls flag is not a uniqueness key; same host+pathPrefix with
			// different tls values is still a duplicate collision.
			name: "same host+pathPrefix different tls is still a duplicate",
			routing: Routing{
				Domain: "app.home.lab",
				Aliases: []RoutingAlias{
					{Host: "x.example.com", PathPrefix: "/api", TLS: false},
					{Host: "x.example.com", PathPrefix: "/api", TLS: true},
				},
			},
			wantErr: "alias[1]",
		},
		{
			// Different hosts with both tls:true must not collide.
			name: "different hosts both tls true is valid",
			routing: Routing{
				Domain: "app.home.lab",
				Aliases: []RoutingAlias{
					{Host: "a.example.com", TLS: true},
					{Host: "b.example.com", TLS: true},
				},
			},
			wantErr: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validBase(tc.routing)
			err := Validate(m)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidate_InvalidTeam(t *testing.T) {
	m, err := Parse(testdataPath("invalid-team.yml"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	err = Validate(m)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	msg := err.Error()
	expected := []string{
		"metadata.name is required",
		"spec.displayName is required",
		"spec.contact is required",
	}
	for _, e := range expected {
		if !strings.Contains(msg, e) {
			t.Errorf("error missing %q, got: %s", e, msg)
		}
	}
}

func TestValidate_VolumeMountRules(t *testing.T) {
	cases := []struct {
		name    string
		mounts  []VolumeMount
		wantErr string
	}{
		{
			name:    "missing name is rejected",
			mounts:  []VolumeMount{{MountPath: "/data"}},
			wantErr: "spec.volumes[0].name is required",
		},
		{
			name:    "missing mountPath is rejected",
			mounts:  []VolumeMount{{Name: "data"}},
			wantErr: "spec.volumes[0].mountPath is required",
		},
		{
			name:    "non-absolute mountPath is rejected",
			mounts:  []VolumeMount{{Name: "data", MountPath: "relative/path"}},
			wantErr: "must be absolute (starts with /)",
		},
		{
			name:    "duplicate name is rejected",
			mounts:  []VolumeMount{{Name: "data", MountPath: "/a"}, {Name: "data", MountPath: "/b"}},
			wantErr: "spec.volumes has duplicate name \"data\"",
		},
		{
			name:    "duplicate mountPath is rejected",
			mounts:  []VolumeMount{{Name: "a", MountPath: "/data"}, {Name: "b", MountPath: "/data"}},
			wantErr: "spec.volumes has duplicate mountPath \"/data\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{
				TypeMeta: TypeMeta{Kind: ApplicationKind, APIVersion: "shrine/v1"},
				Application: &ApplicationManifest{
					Metadata: Metadata{Name: "a", Owner: "team-a"},
					Spec:     ApplicationSpec{Image: "img", Port: 80, Volumes: tc.mounts},
				},
			}
			err := Validate(m)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}
