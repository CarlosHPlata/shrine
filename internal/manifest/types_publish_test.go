package manifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func unmarshalAppSpec(t *testing.T, specYAML string) (ApplicationSpec, error) {
	t.Helper()
	var m ApplicationManifest
	err := yaml.Unmarshal([]byte(specYAML), &m)
	return m.Spec, err
}

func TestPublishUnmarshal_TrueMeansAutomatic(t *testing.T) {
	spec, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish: true
`)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if spec.Networking.Publish == nil {
		t.Fatal("publish: true should yield a non-nil Publish")
	}
	if spec.Networking.Publish.HostPort != 0 {
		t.Errorf("publish: true should mean automatic (HostPort 0), got %d", spec.Networking.Publish.HostPort)
	}
}

func TestPublishUnmarshal_FalseEquivalentToOmission(t *testing.T) {
	spec, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish: false
`)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if spec.Networking.Publish != nil {
		t.Errorf("publish: false should yield nil Publish, got %+v", spec.Networking.Publish)
	}
}

func TestPublishUnmarshal_ExplicitHostPort(t *testing.T) {
	spec, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish:
      hostPort: 8080
`)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if spec.Networking.Publish == nil {
		t.Fatal("publish mapping should yield a non-nil Publish")
	}
	if spec.Networking.Publish.HostPort != 8080 {
		t.Errorf("got HostPort %d, want 8080", spec.Networking.Publish.HostPort)
	}
}

func TestPublishUnmarshal_OmissionYieldsNil(t *testing.T) {
	spec, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    exposeToPlatform: true
`)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if spec.Networking.Publish != nil {
		t.Errorf("omitted publish should yield nil, got %+v", spec.Networking.Publish)
	}
	if !spec.Networking.ExposeToPlatform {
		t.Error("exposeToPlatform should survive the custom Networking unmarshal")
	}
}

func TestPublishUnmarshal_NullYieldsNil(t *testing.T) {
	spec, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish:
`)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if spec.Networking.Publish != nil {
		t.Errorf("null publish should yield nil, got %+v", spec.Networking.Publish)
	}
}

func TestPublishUnmarshal_InvalidScalarRejected(t *testing.T) {
	_, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish: "yes please"
`)
	if err == nil {
		t.Fatal("non-boolean scalar publish should be rejected")
	}
	if !strings.Contains(err.Error(), "publish") {
		t.Errorf("error should name the publish field, got: %v", err)
	}
}

func TestPublishUnmarshal_UnknownMappingKeyRejected(t *testing.T) {
	_, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish:
      port: 8080
`)
	if err == nil {
		t.Fatal("publish mapping with unknown key should be rejected")
	}
	if !strings.Contains(err.Error(), "publish") || !strings.Contains(err.Error(), "hostPort") {
		t.Errorf("error should name publish and the valid hostPort key, got: %v", err)
	}
}

func TestPublishUnmarshal_NonIntegerHostPortRejected(t *testing.T) {
	_, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish:
      hostPort: eighty
`)
	if err == nil {
		t.Fatal("non-integer hostPort should be rejected")
	}
}

func TestPublishUnmarshal_SequenceRejected(t *testing.T) {
	_, err := unmarshalAppSpec(t, `
spec:
  image: nginx:alpine
  port: 80
  networking:
    publish:
      - 8080
`)
	if err == nil {
		t.Fatal("sequence publish should be rejected")
	}
}

func TestShouldAttachToPlatform(t *testing.T) {
	cases := []struct {
		name string
		n    Networking
		want bool
	}{
		{"neither", Networking{}, false},
		{"exposeOnly", Networking{ExposeToPlatform: true}, true},
		{"publishOnly", Networking{Publish: &Publish{}}, true},
		{"publishExplicitOnly", Networking{Publish: &Publish{HostPort: 8080}}, true},
		{"both", Networking{ExposeToPlatform: true, Publish: &Publish{}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.n.ShouldAttachToPlatform(); got != tc.want {
				t.Errorf("ShouldAttachToPlatform() = %v, want %v", got, tc.want)
			}
		})
	}
}
