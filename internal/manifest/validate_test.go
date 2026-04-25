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
