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
