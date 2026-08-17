package manifest

import (
	"strings"
	"testing"
)

func validAppManifest(publish *Publish) *Manifest {
	return &Manifest{
		TypeMeta: TypeMeta{APIVersion: "shrine/v1", Kind: ApplicationKind},
		Application: &ApplicationManifest{
			TypeMeta: TypeMeta{APIVersion: "shrine/v1", Kind: ApplicationKind},
			Metadata: Metadata{Name: "web", Owner: "demo"},
			Spec: ApplicationSpec{
				Image:      "nginx:alpine",
				Port:       80,
				Networking: Networking{Publish: publish},
			},
		},
	}
}

func TestValidatePublish_ValidPorts(t *testing.T) {
	for _, port := range []int{0, 1024, 8080, 29999, 32768, 65535} {
		if err := Validate(validAppManifest(&Publish{HostPort: port})); err != nil {
			t.Errorf("hostPort %d should be valid, got: %v", port, err)
		}
	}
}

func TestValidatePublish_NilPublishIsValid(t *testing.T) {
	if err := Validate(validAppManifest(nil)); err != nil {
		t.Errorf("nil publish should be valid, got: %v", err)
	}
}

func TestValidatePublish_OutOfRangeRejected(t *testing.T) {
	for _, port := range []int{-1, 1, 80, 1023, 65536, 100000} {
		err := Validate(validAppManifest(&Publish{HostPort: port}))
		if err == nil {
			t.Errorf("hostPort %d should be rejected", port)
			continue
		}
		if !strings.Contains(err.Error(), "publish.hostPort") {
			t.Errorf("hostPort %d: error should name spec.networking.publish.hostPort, got: %v", port, err)
		}
	}
}

func TestValidatePublish_AutomaticRangeExcluded(t *testing.T) {
	for _, port := range []int{30000, 31000, 32767} {
		err := Validate(validAppManifest(&Publish{HostPort: port}))
		if err == nil {
			t.Errorf("hostPort %d inside the automatic range should be rejected", port)
			continue
		}
		if !strings.Contains(err.Error(), "automatic allocation") {
			t.Errorf("hostPort %d: error should mention the automatic allocation range, got: %v", port, err)
		}
	}
}

func TestValidatePublish_ErrorsAccumulateWithOtherSpecErrors(t *testing.T) {
	m := validAppManifest(&Publish{HostPort: 80})
	m.Application.Spec.Image = ""

	err := Validate(m)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	if !strings.Contains(err.Error(), "spec.image is required") {
		t.Errorf("missing-image error should be reported alongside publish error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "publish.hostPort") {
		t.Errorf("publish error should be reported alongside missing-image error, got: %v", err)
	}
}

func TestValidatePublish_RejectedOnResource(t *testing.T) {
	m := &Manifest{
		TypeMeta: TypeMeta{APIVersion: "shrine/v1", Kind: ResourceKind},
		Resource: &ResourceManifest{
			TypeMeta: TypeMeta{APIVersion: "shrine/v1", Kind: ResourceKind},
			Metadata: Metadata{Name: "db", Owner: "demo"},
			Spec: ResourceSpec{
				Type:       "postgres",
				Version:    "16",
				Networking: Networking{Publish: &Publish{HostPort: 8080}},
			},
		},
	}
	err := Validate(m)
	if err == nil {
		t.Fatal("publish on a Resource should be rejected")
	}
	if !strings.Contains(err.Error(), "publish") || !strings.Contains(err.Error(), "Application") {
		t.Errorf("error should say publish is Application-only, got: %v", err)
	}
}
