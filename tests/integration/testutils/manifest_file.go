//go:build integration

package testutils

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFile is a private copy of a fixture manifest that a test can edit
// between deploys, field by field, without touching the shared testdata.
type ManifestFile struct {
	tc   *TestCase
	path string
	doc  map[string]any
}

// CopyManifest copies the fixture at src into its own directory under the
// test's temp dir, so the copy can be deployed with --path m.Dir().
func (tc *TestCase) CopyManifest(src string) *ManifestFile {
	tc.t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		tc.t.Fatalf("reading manifest %s: %v", src, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		tc.t.Fatalf("parsing manifest %s: %v", src, err)
	}
	dir, err := os.MkdirTemp(tc.tmpDir, "manifest-*")
	if err != nil {
		tc.t.Fatalf("creating manifest dir: %v", err)
	}
	m := &ManifestFile{tc: tc, path: filepath.Join(dir, filepath.Base(src)), doc: doc}
	m.save()
	return m
}

// Dir is the directory holding only this manifest.
func (m *ManifestFile) Dir() string { return filepath.Dir(m.path) }

// Set writes value at a dotted field path such as "spec.command".
func (m *ManifestFile) Set(field string, value any) *ManifestFile {
	m.tc.t.Helper()
	parent, key := m.parentOf(field)
	parent[key] = value
	m.save()
	return m
}

// Remove deletes the field at a dotted path such as "spec.command".
func (m *ManifestFile) Remove(field string) *ManifestFile {
	m.tc.t.Helper()
	parent, key := m.parentOf(field)
	delete(parent, key)
	m.save()
	return m
}

func (m *ManifestFile) parentOf(field string) (map[string]any, string) {
	m.tc.t.Helper()
	parts := strings.Split(field, ".")
	node := m.doc
	for _, p := range parts[:len(parts)-1] {
		child, ok := node[p].(map[string]any)
		if !ok {
			m.tc.t.Fatalf("manifest %s has no mapping at %q", m.path, p)
		}
		node = child
	}
	return node, parts[len(parts)-1]
}

func (m *ManifestFile) save() {
	m.tc.t.Helper()
	data, err := yaml.Marshal(m.doc)
	if err != nil {
		m.tc.t.Fatalf("encoding manifest %s: %v", m.path, err)
	}
	if err := os.WriteFile(m.path, data, 0o644); err != nil {
		m.tc.t.Fatalf("writing manifest %s: %v", m.path, err)
	}
}
