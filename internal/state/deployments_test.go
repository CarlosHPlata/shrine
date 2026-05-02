package state

import "testing"

func TestConfigHash_DifferentPortSpecs_ProduceDifferentHashes(t *testing.T) {
	base := ConfigHash("img@sha256:abc", nil, nil, []string{"80:80/tcp"}, false)
	other := ConfigHash("img@sha256:abc", nil, nil, []string{"443:443/tcp"}, false)

	if base == other {
		t.Fatalf("expected different hashes for different port specs; both produced %q", base)
	}
}

func TestConfigHash_PortSpecsAreOrderInvariant(t *testing.T) {
	a := ConfigHash("img@sha256:abc", nil, nil, []string{"80:80/tcp", "443:443/tcp", "8080:8080/tcp"}, false)
	b := ConfigHash("img@sha256:abc", nil, nil, []string{"443:443/tcp", "8080:8080/tcp", "80:80/tcp"}, false)
	c := ConfigHash("img@sha256:abc", nil, nil, []string{"8080:8080/tcp", "80:80/tcp", "443:443/tcp"}, false)

	if a != b || b != c {
		t.Fatalf("expected identical hashes for the same port specs in different orders\na=%s\nb=%s\nc=%s", a, b, c)
	}
}

func TestConfigHash_NilAndEmptyPortSpecs_ProduceSameHash(t *testing.T) {
	withNil := ConfigHash("img@sha256:abc", nil, nil, nil, false)
	withEmpty := ConfigHash("img@sha256:abc", nil, nil, []string{}, false)

	if withNil != withEmpty {
		t.Fatalf("expected identical hashes for nil vs empty port specs\nnil=%s\nempty=%s", withNil, withEmpty)
	}
}

func TestConfigHash_PortSpecsAffectHash_IndependentOfEnvAndVolumes(t *testing.T) {
	withoutPorts := ConfigHash("img", []string{"K=V"}, []string{"data:/var/lib"}, nil, false)
	withPorts := ConfigHash("img", []string{"K=V"}, []string{"data:/var/lib"}, []string{"80:80/tcp"}, false)

	if withoutPorts == withPorts {
		t.Fatalf("expected port specs to be a distinct hash input; both produced %q", withoutPorts)
	}
}
