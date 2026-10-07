package handler

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/state"
)

var errBoom = errors.New("boom")

const bumpSpecsDir = "/specs"

var (
	hexA = strings.Repeat("a", 64)
	hexB = strings.Repeat("b", 64)
)

// callLog records the order in which the pin store and the backend are
// reached, shared by both fakes of one test.
type callLog []string

// orderedPinStore logs every Get so a test can prove the previous pin is
// read before the backend records the new one.
type orderedPinStore struct {
	*memImagePinStore
	log    *callLog
	getErr error
}

func (s orderedPinStore) Get(team, name string) (state.ImagePin, error) {
	*s.log = append(*s.log, "pins.Get")
	if s.getErr != nil {
		return state.ImagePin{}, s.getErr
	}
	return s.memImagePinStore.Get(team, name)
}

// repinRecordingBackend answers ResolveImage with a canned result and
// records each op; every other method is the no-op stub's.
type repinRecordingBackend struct {
	stubContainerBackend
	resolved engine.ResolvedImage
	err      error
	ops      []engine.ResolveImageOp
	log      *callLog
}

func (r *repinRecordingBackend) ResolveImage(op engine.ResolveImageOp) (engine.ResolvedImage, error) {
	r.ops = append(r.ops, op)
	if r.log != nil {
		*r.log = append(*r.log, "backend.ResolveImage")
	}
	return r.resolved, r.err
}

// pinnedSet builds an in-memory set holding one artifact, the shape a
// manifest file loads into, so the handler is tested without the filesystem.
func pinnedSet(t *testing.T, kind, name, owner, image, policy string) *planner.ManifestSet {
	t.Helper()
	set := planner.NewManifestSet()
	if err := set.MergeManifest(artifactManifest(kind, name, owner, image, policy), "/specs/"+name+".yml"); err != nil {
		t.Fatalf("MergeManifest: %v", err)
	}
	return set
}

func artifactManifest(kind, name, owner, image, policy string) *manifest.Manifest {
	typeMeta := manifest.TypeMeta{APIVersion: "shrine/v1", Kind: kind}
	metadata := manifest.Metadata{Name: name, Owner: owner}
	if kind == manifest.ResourceKind {
		return &manifest.Manifest{TypeMeta: typeMeta, Resource: &manifest.ResourceManifest{
			TypeMeta: typeMeta,
			Metadata: metadata,
			Spec:     manifest.ResourceSpec{Type: manifest.RepositoryOf(image), Image: image, ImagePullPolicy: policy},
		}}
	}
	return &manifest.Manifest{TypeMeta: typeMeta, Application: &manifest.ApplicationManifest{
		TypeMeta: typeMeta,
		Metadata: metadata,
		Spec:     manifest.ApplicationSpec{Image: image, Port: 80, ImagePullPolicy: policy},
	}}
}

func bumpTestStore() *state.Store {
	return &state.Store{Teams: &memTeamStore{}, HostPorts: &memHostPortStore{ports: state.HostPortMap{}}}
}

func invalidVersionMessage(version string) string {
	return `invalid version "` + version + `": use a tag (a letter, digit, or underscore, then up to 127 letters, digits, underscores, dots, or dashes) or an exact version "sha256:<64 hex>"`
}

func TestValidateBumpVersion(t *testing.T) {
	accepted := []string{"", "17", "v1.4.0", "latest", "a_b.c-d", strings.Repeat("a", 128), "sha256:" + hexA}
	for _, version := range accepted {
		if err := validateBumpVersion(version); err != nil {
			t.Errorf("validateBumpVersion(%q) = %v, want nil", version, err)
		}
	}

	rejected := []string{
		strings.Repeat("a", 129),
		".hidden",
		"-dash",
		"v 9",
		"postgres:17",
		"repo@sha256:" + hexA,
		"sha256:" + hexA[:63],
		"sha256:" + hexA + "a",
		"sha256:" + strings.ToUpper(hexA),
	}
	for _, version := range rejected {
		err := validateBumpVersion(version)
		if err == nil {
			t.Errorf("validateBumpVersion(%q) = nil, want a refusal", version)
			continue
		}
		if err.Error() != invalidVersionMessage(version) {
			t.Errorf("validateBumpVersion(%q) message:\n got: %s\nwant: %s", version, err, invalidVersionMessage(version))
		}
	}
}

func TestBuildBumpTarget(t *testing.T) {
	cases := []struct{ image, version, want string }{
		{"postgres", "17", "postgres:17"},
		{"postgres", "sha256:" + hexA, "postgres@sha256:" + hexA},
		{"postgres", "", "postgres"},
		{"127.0.0.1:5000/shrine/whoami", "v2", "127.0.0.1:5000/shrine/whoami:v2"},
		{"127.0.0.1:5000/shrine/whoami", "", "127.0.0.1:5000/shrine/whoami"},
		{"reg:lab/api", "1.2", "reg:lab/api:1.2"},
		{"postgres:latest", "17", "postgres:17"},
	}
	for _, c := range cases {
		if got := buildBumpTarget(c.image, c.version); got != c.want {
			t.Errorf("buildBumpTarget(%q, %q) = %q, want %q", c.image, c.version, got, c.want)
		}
	}
}

func TestFindBumpArtifact(t *testing.T) {
	set := pinnedSet(t, manifest.ApplicationKind, "web", "team-a", "nginx", manifest.ImagePullPolicyPinned)
	if err := set.MergeManifest(artifactManifest(manifest.ResourceKind, "db", "team-a", "postgres", manifest.ImagePullPolicyPinned), "/specs/db.yml"); err != nil {
		t.Fatalf("MergeManifest: %v", err)
	}

	t.Run("found by kind", func(t *testing.T) {
		meta, image, err := findBumpArtifact(set, BumpOptions{Kind: manifest.ApplicationKind, Name: "web"}, bumpSpecsDir)
		if err != nil {
			t.Fatalf("findBumpArtifact: %v", err)
		}
		if meta.Name != "web" || meta.Owner != "team-a" || image != "nginx" {
			t.Errorf("got meta %+v image %q", meta, image)
		}
		meta, image, err = findBumpArtifact(set, BumpOptions{Kind: manifest.ResourceKind, Name: "db"}, bumpSpecsDir)
		if err != nil {
			t.Fatalf("findBumpArtifact resource: %v", err)
		}
		if meta.Name != "db" || image != "postgres" {
			t.Errorf("got meta %+v image %q", meta, image)
		}
	})

	t.Run("team equal to the owner passes", func(t *testing.T) {
		if _, _, err := findBumpArtifact(set, BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Team: "team-a"}, bumpSpecsDir); err != nil {
			t.Errorf("findBumpArtifact with the owner's team: %v", err)
		}
	})

	refusals := []struct {
		name string
		opts BumpOptions
		set  *planner.ManifestSet
		want string
	}{
		{"other kind is not searched", BumpOptions{Kind: manifest.ResourceKind, Name: "web"}, set, `resource "web": no manifest found in /specs`},
		{"unknown names the directory", BumpOptions{Kind: manifest.ApplicationKind, Name: "nope"}, set, `application "nope": no manifest found in /specs`},
		{"wrong team", BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Team: "other"}, set, `application "web" not found in team "other" (its manifest in /specs is owned by "team-a")`},
		{"empty set", BumpOptions{Kind: manifest.ApplicationKind, Name: "web"}, planner.NewManifestSet(), `application "web": no manifest found in /specs`},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := findBumpArtifact(c.set, c.opts, bumpSpecsDir)
			if err == nil || err.Error() != c.want {
				t.Errorf("got %v, want %s", err, c.want)
			}
		})
	}
}

// prepareRefusal is one way prepareBump refuses before anything is resolved;
// the table is shared with the dry-run test because both entry points refuse
// through prepareBump alone.
type prepareRefusal struct {
	name          string
	set           func(t *testing.T) *planner.ManifestSet
	defaultPolicy string
	opts          BumpOptions
	want          string
}

func webSet(image, policy string) func(t *testing.T) *planner.ManifestSet {
	return func(t *testing.T) *planner.ManifestSet {
		return pinnedSet(t, manifest.ApplicationKind, "web", "team-a", image, policy)
	}
}

func prepareRefusals() []prepareRefusal {
	webOpts := BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Version: "17"}
	return []prepareRefusal{
		{
			name: "declared IfNotPresent",
			set:  webSet("nginx", manifest.ImagePullPolicyIfNotPresent),
			opts: webOpts,
			want: `application "web": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it`,
		},
		{
			name:          "configured Always",
			set:           webSet("nginx", ""),
			defaultPolicy: manifest.ImagePullPolicyAlways,
			opts:          webOpts,
			want:          `application "web": its version is manifest-owned (imagePullPolicy Always); edit the manifest to change it`,
		},
		{
			name: "derived IfNotPresent",
			set:  webSet("nginx:1.27", ""),
			opts: webOpts,
			want: `application "web": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it`,
		},
		{
			name: "unknown",
			set:  webSet("nginx", manifest.ImagePullPolicyPinned),
			opts: BumpOptions{Kind: manifest.ApplicationKind, Name: "nope", Version: "17"},
			want: `application "nope": no manifest found in /specs`,
		},
		{
			name: "wrong team",
			set:  webSet("nginx", manifest.ImagePullPolicyPinned),
			opts: BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Team: "other", Version: "17"},
			want: `application "web" not found in team "other" (its manifest in /specs is owned by "team-a")`,
		},
	}
}

func runPrepareBump(t *testing.T, set *planner.ManifestSet, defaultPolicy string, opts BumpOptions) (bumpTarget, string, error) {
	t.Helper()
	var errOut bytes.Buffer
	cfg := &config.Config{ImagePullPolicy: defaultPolicy}
	target, err := prepareBump(&errOut, bumpSpecsDir, set, bumpTestStore(), cfg, opts)
	return target, errOut.String(), err
}

func TestPrepareBump(t *testing.T) {
	for _, c := range prepareRefusals() {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := runPrepareBump(t, c.set(t), c.defaultPolicy, c.opts)
			if err == nil || err.Error() != c.want {
				t.Errorf("got %v, want %s", err, c.want)
			}
		})
	}

	t.Run("declared Pinned returns the target", func(t *testing.T) {
		set := pinnedSet(t, manifest.ApplicationKind, "web", "team-a", "nginx", manifest.ImagePullPolicyPinned)
		target, _, err := runPrepareBump(t, set, "", BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Version: "17"})
		if err != nil {
			t.Fatalf("prepareBump: %v", err)
		}
		want := bumpTarget{Team: "team-a", Kind: manifest.ApplicationKind, Name: "web", ManifestImage: "nginx", Target: "nginx:17"}
		if target != want {
			t.Errorf("got %+v, want %+v", target, want)
		}
	})

	t.Run("an exact version targets the manifest repository by digest", func(t *testing.T) {
		set := pinnedSet(t, manifest.ApplicationKind, "web", "team-a", "nginx", manifest.ImagePullPolicyPinned)
		target, _, err := runPrepareBump(t, set, "", BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Version: "sha256:" + hexA})
		if err != nil {
			t.Fatalf("prepareBump: %v", err)
		}
		if want := "nginx@sha256:" + hexA; target.Target != want {
			t.Errorf("Target = %q, want %q", target.Target, want)
		}
	})

	t.Run("a Pinned resource returns its target", func(t *testing.T) {
		set := pinnedSet(t, manifest.ResourceKind, "db", "team-a", "postgres", manifest.ImagePullPolicyPinned)
		target, _, err := runPrepareBump(t, set, "", BumpOptions{Kind: manifest.ResourceKind, Name: "db", Version: "17"})
		if err != nil {
			t.Fatalf("prepareBump: %v", err)
		}
		want := bumpTarget{Team: "team-a", Kind: manifest.ResourceKind, Name: "db", ManifestImage: "postgres", Target: "postgres:17"}
		if target != want {
			t.Errorf("got %+v, want %+v", target, want)
		}
	})

	t.Run("team equal to the owner verifies and returns the target", func(t *testing.T) {
		set := pinnedSet(t, manifest.ApplicationKind, "web", "team-a", "nginx", manifest.ImagePullPolicyPinned)
		target, _, err := runPrepareBump(t, set, "", BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Team: "team-a", Version: "17"})
		if err != nil {
			t.Fatalf("prepareBump: %v", err)
		}
		if target.Team != "team-a" || target.Target != "nginx:17" {
			t.Errorf("got %+v", target)
		}
	})

	t.Run("an empty set names the directory", func(t *testing.T) {
		_, _, err := runPrepareBump(t, planner.NewManifestSet(), "", BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Version: "17"})
		if err == nil || err.Error() != `application "web": no manifest found in /specs` {
			t.Errorf("got %v, want the no-manifest refusal naming /specs", err)
		}
	})

	t.Run("Pinned naming a fixed version fails validation", func(t *testing.T) {
		set := pinnedSet(t, manifest.ApplicationKind, "web", "team-a", "nginx:1.27", manifest.ImagePullPolicyPinned)
		_, errOut, err := runPrepareBump(t, set, "", BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Version: "17"})
		if err == nil || err.Error() != "Spec validation errors" {
			t.Fatalf("got %v, want Spec validation errors", err)
		}
		if !strings.Contains(errOut, "names a fixed version") {
			t.Errorf("errOut should carry the validation line, got %q", errOut)
		}
	})
}

func pinnedResult(requested, digest string) engine.ResolvedImage {
	return engine.ResolvedImage{
		Ref:       "nginx@" + digest,
		Digest:    digest,
		Source:    engine.ImageSourceRepinned,
		Requested: requested,
	}
}

func webTarget() bumpTarget {
	return bumpTarget{Team: "team-a", Kind: manifest.ApplicationKind, Name: "web", ManifestImage: "nginx", Target: "nginx:17"}
}

// bumpFixture wires an ordered pin store and a recording backend to one log.
type bumpFixture struct {
	store   *state.Store
	pins    *memImagePinStore
	backend *repinRecordingBackend
	log     *callLog
}

func newBumpFixture(resolved engine.ResolvedImage, resolveErr error, previous ...state.ImagePin) bumpFixture {
	log := &callLog{}
	pins := &memImagePinStore{pins: map[string]state.ImagePin{}}
	for _, pin := range previous {
		pins.pins[state.ImagePinKey("team-a", pin.Name)] = pin
	}
	store := bumpTestStore()
	store.ImagePins = orderedPinStore{memImagePinStore: pins, log: log}
	return bumpFixture{
		store:   store,
		pins:    pins,
		backend: &repinRecordingBackend{resolved: resolved, err: resolveErr, log: log},
		log:     log,
	}
}

func (f bumpFixture) run(t *testing.T, target bumpTarget) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := bumpResolved(&out, f.store, f.backend, target)
	return out.String(), err
}

func TestBumpResolved(t *testing.T) {
	t.Run("previous pin on record", func(t *testing.T) {
		previous := state.ImagePin{Kind: manifest.ApplicationKind, Name: "web", Requested: "nginx:16", Pinned: "nginx@sha256:" + hexA}
		f := newBumpFixture(pinnedResult("nginx:17", "sha256:"+hexB), nil, previous)

		out, err := f.run(t, webTarget())
		if err != nil {
			t.Fatalf("bumpResolved: %v", err)
		}

		wantOp := engine.ResolveImageOp{Team: "team-a", Name: "web", Kind: manifest.ApplicationKind, Image: "nginx", ImagePullPolicy: manifest.ImagePullPolicyPinned, Repin: "nginx:17"}
		if len(f.backend.ops) != 1 || f.backend.ops[0] != wantOp {
			t.Errorf("ops = %+v, want [%+v]", f.backend.ops, wantOp)
		}
		if got := strings.Join(*f.log, ","); got != "pins.Get,backend.ResolveImage" {
			t.Errorf("call order = %s, want the previous pin read before ResolveImage", got)
		}
		want := "Bumped team-a/web: 16@aaaaaaaaaaaa -> 17@bbbbbbbbbbbb; run \"shrine deploy\" to apply\n"
		if out != want {
			t.Errorf("out:\n got: %q\nwant: %q", out, want)
		}
		if len(f.pins.puts) != 0 {
			t.Errorf("the handler must not write pins, got %+v", f.pins.puts)
		}
	})

	t.Run("no previous pin", func(t *testing.T) {
		f := newBumpFixture(pinnedResult("nginx:17", "sha256:"+hexB), nil)
		out, err := f.run(t, webTarget())
		if err != nil {
			t.Fatalf("bumpResolved: %v", err)
		}
		want := "Pinned team-a/web at 17@bbbbbbbbbbbb; run \"shrine deploy\" to apply\n"
		if out != want {
			t.Errorf("out:\n got: %q\nwant: %q", out, want)
		}
	})

	t.Run("digest previous", func(t *testing.T) {
		digestRef := "nginx@sha256:" + hexA
		previous := state.ImagePin{Kind: manifest.ApplicationKind, Name: "web", Requested: digestRef, Pinned: digestRef}
		f := newBumpFixture(pinnedResult("nginx:17", "sha256:"+hexB), nil, previous)
		out, err := f.run(t, webTarget())
		if err != nil {
			t.Fatalf("bumpResolved: %v", err)
		}
		want := "Bumped team-a/web: aaaaaaaaaaaa -> 17@bbbbbbbbbbbb; run \"shrine deploy\" to apply\n"
		if out != want {
			t.Errorf("out:\n got: %q\nwant: %q", out, want)
		}
	})

	t.Run("newest again prints previous and new even when equal", func(t *testing.T) {
		previous := state.ImagePin{Kind: manifest.ApplicationKind, Name: "web", Requested: "nginx", Pinned: "nginx@sha256:" + hexA}
		f := newBumpFixture(pinnedResult("nginx", "sha256:"+hexA), nil, previous)
		target := webTarget()
		target.Target = target.ManifestImage
		out, err := f.run(t, target)
		if err != nil {
			t.Fatalf("bumpResolved: %v", err)
		}
		if f.backend.ops[0].Repin != "nginx" {
			t.Errorf("Repin = %q, want the manifest reference", f.backend.ops[0].Repin)
		}
		want := "Bumped team-a/web: latest@aaaaaaaaaaaa -> latest@aaaaaaaaaaaa; run \"shrine deploy\" to apply\n"
		if out != want {
			t.Errorf("out:\n got: %q\nwant: %q", out, want)
		}
	})

	t.Run("undeployed artifact never reads deployment records", func(t *testing.T) {
		f := newBumpFixture(pinnedResult("nginx:17", "sha256:"+hexB), nil)
		f.store.Deployments = &memDeploymentStore{listErr: errBoom}
		out, err := f.run(t, webTarget())
		if err != nil {
			t.Fatalf("bumpResolved must not read deployments, got %v", err)
		}
		want := "Pinned team-a/web at 17@bbbbbbbbbbbb; run \"shrine deploy\" to apply\n"
		if out != want {
			t.Errorf("out:\n got: %q\nwant: %q", out, want)
		}
	})

	t.Run("backend error", func(t *testing.T) {
		f := newBumpFixture(engine.ResolvedImage{}, errBoom)
		out, err := f.run(t, webTarget())
		if err == nil || err.Error() != `application "web": boom` {
			t.Fatalf("got %v, want application \"web\": boom", err)
		}
		if !errors.Is(err, errBoom) {
			t.Errorf("the backend's error must stay reachable through errors.Is")
		}
		if out != "" {
			t.Errorf("nothing may be printed on failure, got %q", out)
		}
	})

	t.Run("nil pin store means no previous", func(t *testing.T) {
		store := bumpTestStore()
		backend := &repinRecordingBackend{resolved: pinnedResult("nginx:17", "sha256:"+hexB)}
		var out bytes.Buffer
		if err := bumpResolved(&out, store, backend, webTarget()); err != nil {
			t.Fatalf("bumpResolved: %v", err)
		}
		want := "Pinned team-a/web at 17@bbbbbbbbbbbb; run \"shrine deploy\" to apply\n"
		if out.String() != want {
			t.Errorf("out:\n got: %q\nwant: %q", out.String(), want)
		}
	})

	t.Run("pin read error", func(t *testing.T) {
		f := newBumpFixture(pinnedResult("nginx:17", "sha256:"+hexB), nil)
		f.store.ImagePins = orderedPinStore{memImagePinStore: f.pins, log: f.log, getErr: errBoom}
		out, err := f.run(t, webTarget())
		if err == nil || err.Error() != "reading image pin for team-a/web: boom" || !errors.Is(err, errBoom) {
			t.Fatalf("got %v, want reading image pin for team-a/web: boom", err)
		}
		if len(f.backend.ops) != 0 || out != "" {
			t.Errorf("a failed read must stop before the backend, ops %+v out %q", f.backend.ops, out)
		}
	})
}

// TestPrepareBump_RefusalsAreSharedByDryRun reuses the refusal table:
// Bump and BumpDryRun differ only after prepareBump returns, so each refusal
// here is a refusal of both entry points, and it reads and records nothing.
func TestPrepareBump_RefusalsAreSharedByDryRun(t *testing.T) {
	for _, c := range prepareRefusals() {
		t.Run(c.name, func(t *testing.T) {
			f := newBumpFixture(pinnedResult("nginx:17", "sha256:"+hexB), nil)
			var errOut bytes.Buffer
			_, err := prepareBump(&errOut, bumpSpecsDir, c.set(t), f.store, &config.Config{ImagePullPolicy: c.defaultPolicy}, c.opts)
			if err == nil || err.Error() != c.want {
				t.Fatalf("got %v, want %s", err, c.want)
			}
			if len(*f.log) != 0 || len(f.pins.puts) != 0 {
				t.Errorf("a refusal must read no pin and record nothing, calls %v puts %+v", *f.log, f.pins.puts)
			}
		})
	}

	// An invalid version is refused before the directory is read, so both
	// entry points can be called here without touching the filesystem.
	t.Run("invalid version refused by both entry points", func(t *testing.T) {
		const unreadDir = "/never/read/specs"
		opts := BumpOptions{Kind: manifest.ApplicationKind, Name: "web", Version: "not valid!"}
		var out, errOut bytes.Buffer
		dryRunErr := BumpDryRun(&out, &errOut, unreadDir, bumpTestStore(), &config.Config{}, opts)
		bumpErr := Bump(&app.BumpBundle{Out: &out, ErrOut: &errOut, Cfg: &config.Config{}, Store: bumpTestStore(), SpecsDir: unreadDir}, opts)
		for _, err := range []error{dryRunErr, bumpErr} {
			if err == nil || err.Error() != invalidVersionMessage("not valid!") {
				t.Errorf("got %v, want %s", err, invalidVersionMessage("not valid!"))
			}
		}
		if out.Len() != 0 || errOut.Len() != 0 {
			t.Errorf("a refusal prints nothing, out %q errOut %q", out.String(), errOut.String())
		}
	})
}

func TestFormatBumpDryRun(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   string
	}{
		{"tagged", "nginx:17", "[dry-run] would resolve nginx:17 and pin team-a/web"},
		{"exact version", "nginx@sha256:" + hexA, "[dry-run] would resolve nginx@sha256:" + hexA + " and pin team-a/web"},
		{"manifest reference", "nginx", "[dry-run] would resolve nginx and pin team-a/web"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target := webTarget()
			target.Target = c.target
			if got := formatBumpDryRun(target); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
