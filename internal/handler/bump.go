package handler

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// BumpOptions names the artifact to bump and the version to pin.
type BumpOptions struct {
	Kind    string // manifest.ApplicationKind or manifest.ResourceKind
	Name    string
	Team    string // optional; verified against metadata.owner
	Version string // "" means the manifest reference (newest)
}

// bumpTarget is everything known once the artifact is found, the policy
// checked, and the target built; both the real and the dry-run path end here.
type bumpTarget struct {
	Team          string
	Kind          string
	Name          string
	ManifestImage string // the manifest reference, as written
	Target        string // the reference to resolve: repo:tag, repo@sha256:…, or ManifestImage
}

// A version may name a tag or an exact version but never a repository, so a
// bump can only move an artifact within the image its manifest names.
var (
	bumpTagPattern    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	bumpDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// Bump records a new pin for one Pinned artifact; the next deploy applies it.
func Bump(b *app.BumpBundle, opts BumpOptions) error {
	if err := validateBumpVersion(opts.Version); err != nil {
		return err
	}
	set, err := planner.LoadDir(b.SpecsDir)
	if err != nil {
		return err
	}
	target, err := prepareBump(b.ErrOut, b.SpecsDir, set, b.Store, b.Cfg, opts)
	if err != nil {
		return err
	}
	return bumpResolved(b.Out, b.Store, b.ContainerBackend, target)
}

// BumpDryRun prints what Bump would resolve and pin; it has no backend, so it
// cannot record anything or contact a registry.
func BumpDryRun(out, errOut io.Writer, manifestDir string, store *state.Store, cfg *config.Config, opts BumpOptions) error {
	if err := validateBumpVersion(opts.Version); err != nil {
		return err
	}
	set, err := planner.LoadDir(manifestDir)
	if err != nil {
		return err
	}
	target, err := prepareBump(errOut, manifestDir, set, store, cfg, opts)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, formatBumpDryRun(target))
	return nil
}

func validateBumpVersion(version string) error {
	if version == "" || bumpTagPattern.MatchString(version) || bumpDigestPattern.MatchString(version) {
		return nil
	}
	return fmt.Errorf(`invalid version %q: use a tag (a letter, digit, or underscore, then up to 127 letters, digits, underscores, dots, or dashes) or an exact version "sha256:<64 hex>"`, version)
}

// prepareBump runs every check that needs no registry: lookup, team, the
// planning deploy would do, and the policy refusal.
func prepareBump(errOut io.Writer, manifestDir string, set *planner.ManifestSet, store *state.Store, cfg *config.Config, opts BumpOptions) (bumpTarget, error) {
	meta, image, err := findBumpArtifact(set, opts, manifestDir)
	if err != nil {
		return bumpTarget{}, err
	}
	result, _, err := planManifestSet(errOut, set, store, cfg, bumpFilter(opts.Kind, opts.Name))
	if err != nil {
		return bumpTarget{}, err
	}
	if policy := effectivePolicyOf(result.ManifestSet, opts.Kind, opts.Name); policy != manifest.ImagePullPolicyPinned {
		return bumpTarget{}, fmt.Errorf("%s %q: its version is manifest-owned (imagePullPolicy %s); edit the manifest to change it",
			kindWord(opts.Kind), opts.Name, policy)
	}
	return bumpTarget{
		Team:          meta.Owner,
		Kind:          opts.Kind,
		Name:          opts.Name,
		ManifestImage: image,
		Target:        buildBumpTarget(image, opts.Version),
	}, nil
}

// findBumpArtifact searches only the map of the requested kind; a set holds
// at most one artifact per kind and name, so a name is never ambiguous.
func findBumpArtifact(set *planner.ManifestSet, opts BumpOptions, manifestDir string) (manifest.Metadata, string, error) {
	meta, image, isFound := lookupArtifact(set, opts.Kind, opts.Name)
	if !isFound {
		return manifest.Metadata{}, "", fmt.Errorf("%s %q: no manifest found in %s", kindWord(opts.Kind), opts.Name, manifestDir)
	}
	if opts.Team != "" && opts.Team != meta.Owner {
		return manifest.Metadata{}, "", fmt.Errorf("%s %q not found in team %q (its manifest in %s is owned by %q)",
			kindWord(opts.Kind), opts.Name, opts.Team, manifestDir, meta.Owner)
	}
	return meta, image, nil
}

func lookupArtifact(set *planner.ManifestSet, kind, name string) (manifest.Metadata, string, bool) {
	switch kind {
	case manifest.ApplicationKind:
		if application, ok := set.Applications[name]; ok {
			return application.Metadata, application.Spec.Image, true
		}
	case manifest.ResourceKind:
		if resource, ok := set.Resources[name]; ok {
			return resource.Metadata, resource.Spec.Image, true
		}
	}
	return manifest.Metadata{}, "", false
}

func bumpFilter(kind, name string) planner.Filter {
	if kind == manifest.ResourceKind {
		return planner.ByResource(name)
	}
	return planner.ByApp(name)
}

// effectivePolicyOf reads a planned set, where Plan has already written the
// declared, configured, or derived policy into every manifest.
func effectivePolicyOf(set *planner.ManifestSet, kind, name string) string {
	if kind == manifest.ResourceKind {
		return set.Resources[name].Spec.ImagePullPolicy
	}
	return set.Applications[name].Spec.ImagePullPolicy
}

// buildBumpTarget keeps the manifest's repository and replaces only its
// version; an empty version resolves the manifest's own reference.
func buildBumpTarget(image, version string) string {
	if version == "" {
		return image
	}
	separator := ":"
	if bumpDigestPattern.MatchString(version) {
		separator = "@"
	}
	return manifest.RepositoryOf(image) + separator + version
}

func repinOp(t bumpTarget) engine.ResolveImageOp {
	return engine.ResolveImageOp{
		Team:            t.Team,
		Name:            t.Name,
		Kind:            t.Kind,
		Image:           t.ManifestImage,
		ImagePullPolicy: manifest.ImagePullPolicyPinned,
		Repin:           t.Target,
	}
}

// previousPin is read before resolving because the backend overwrites the
// pin it replaces.
func previousPin(store *state.Store, team, name string) (state.ImagePin, bool, error) {
	if store == nil || store.ImagePins == nil {
		return state.ImagePin{}, false, nil
	}
	pin, err := store.ImagePins.Get(team, name)
	if errors.Is(err, state.ErrImagePinNotFound) {
		return state.ImagePin{}, false, nil
	}
	if err != nil {
		return state.ImagePin{}, false, fmt.Errorf("reading image pin for %s/%s: %w", team, name, err)
	}
	return pin, true, nil
}

// bumpResolved resolves the target through the backend, which is the only
// writer of pins, and reports the previous and the new version.
func bumpResolved(out io.Writer, store *state.Store, backend engine.ContainerBackend, t bumpTarget) error {
	previous, hasPrevious, err := previousPin(store, t.Team, t.Name)
	if err != nil {
		return err
	}
	resolved, err := backend.ResolveImage(repinOp(t))
	if err != nil {
		return fmt.Errorf("%s %q: %w", kindWord(t.Kind), t.Name, err)
	}
	fmt.Fprintln(out, formatBumpResult(t, previous, hasPrevious, resolved))
	return nil
}

func formatBumpResult(t bumpTarget, previous state.ImagePin, hasPrevious bool, resolved engine.ResolvedImage) string {
	next := manifest.ReadableVersion(resolved.Requested, resolved.Digest)
	if !hasPrevious {
		return fmt.Sprintf("Pinned %s/%s at %s; run \"shrine deploy\" to apply", t.Team, t.Name, next)
	}
	before := manifest.ReadableVersion(previous.Requested, manifest.DigestOf(previous.Pinned))
	return fmt.Sprintf("Bumped %s/%s: %s -> %s; run \"shrine deploy\" to apply", t.Team, t.Name, before, next)
}

func formatBumpDryRun(t bumpTarget) string {
	return fmt.Sprintf("[dry-run] would resolve %s and pin %s/%s", t.Target, t.Team, t.Name)
}

// kindWord is the manifest kind as the refusal messages spell it.
func kindWord(kind string) string {
	return strings.ToLower(kind)
}
