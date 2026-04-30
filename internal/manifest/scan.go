package manifest

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// ShrineCandidate holds the path and already-probed TypeMeta of a file that
// passed the shrine apiVersion classifier. Callers reuse the TypeMeta to avoid
// a second YAML parse when dispatching to manifest.Parse.
type ShrineCandidate struct {
	Path     string   // absolute file path as returned by filepath.WalkDir
	TypeMeta TypeMeta // probed apiVersion + kind
}

// ScanResult holds the bucketed output of ScanDir.
// Shrine and Foreign are disjoint; every .yaml/.yml file appears in exactly one
// slice (or caused the call to return an error).
type ScanResult struct {
	Shrine  []ShrineCandidate // ordered by deterministic walk order
	Foreign []string          // .yaml/.yml paths whose apiVersion failed the shrine regex
}

// ScanDir walks dir recursively and classifies every .yaml / .yml file it finds.
//
// Extension filter: files whose extension is not exactly ".yaml" or ".yml"
// (case-sensitive) are silently skipped without opening them — this is
// invariant 1 of the scanner contract.
//
// For admitted files, Classify is called. A malformed-YAML error aborts the
// entire scan and is returned wrapped with the directory path.
//
// Walk order is the deterministic order provided by filepath.WalkDir (lexical
// within each directory level), so two calls on the same unchanged tree return
// identical slices.
func ScanDir(dir string) (*ScanResult, error) {
	result := &ScanResult{}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			// Non-YAML sibling: silently skip without opening the file.
			return nil
		}

		class, meta, err := Classify(path)
		if err != nil {
			return fmt.Errorf("scanning manifest directory %q: %w", dir, err)
		}

		switch class {
		case ClassShrine:
			result.Shrine = append(result.Shrine, ShrineCandidate{
				Path:     path,
				TypeMeta: *meta,
			})
		case ClassForeign:
			result.Foreign = append(result.Foreign, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
