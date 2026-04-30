package manifest

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Class identifies the classification of a YAML file encountered during a directory scan.
type Class int

const (
	ClassShrine  Class = iota // apiVersion matches the shrine regex
	ClassForeign              // .yaml/.yml but apiVersion absent or non-matching
)

// shrineAPIVersionRe matches valid shrine apiVersion strings such as shrine/v1,
// shrine/v1beta1, shrine/v10alpha7.
var shrineAPIVersionRe = regexp.MustCompile(`^shrine/v\d+([a-z]+\d+)?$`)

// IsShrineAPIVersion reports whether s matches ^shrine/v\d+([a-z]+\d+)?$.
func IsShrineAPIVersion(s string) bool {
	return shrineAPIVersionRe.MatchString(s)
}

// Classify returns the Class of the YAML file at path.
// Note: yaml.Unmarshal probe duplicated from parser.go's probeKind to avoid coupling classify to the parse pipeline mid-refactor.
func Classify(path string) (Class, *TypeMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, fmt.Errorf("reading manifest %q: %w", path, err)
	}

	var meta TypeMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return 0, nil, fmt.Errorf("parsing manifest %q: %w", path, err)
	}

	if !IsShrineAPIVersion(meta.APIVersion) {
		return ClassForeign, nil, nil
	}

	return ClassShrine, &meta, nil
}
