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

// IsShrineAPIVersion reports whether s matches the shrine apiVersion pattern
// ^shrine/v\d+([a-z]+\d+)?$. This is a pure function over the string value —
// it does not read any files.
func IsShrineAPIVersion(s string) bool {
	return shrineAPIVersionRe.MatchString(s)
}

// Classify reads the file at path, unmarshals its top-level YAML to extract
// apiVersion and kind, and returns the file's Class.
//
// Returns (ClassForeign, nil, nil) when apiVersion is absent or does not match
// the shrine regex — this covers foreign YAML, empty files, and comment-only files.
//
// Returns (ClassShrine, &meta, nil) when apiVersion matches; meta carries the
// probed APIVersion and Kind (kind is NOT validated here — that is the caller's
// responsibility via manifest.Parse / manifest.Validate).
//
// Returns (0, nil, error) when the file cannot be parsed as YAML at all. The
// error wraps the file path so operators can identify the offending file.
//
// Note: the yaml.Unmarshal probe is duplicated from parser.go's probeKind to
// avoid coupling classify.go to the parsing pipeline mid-refactor (lower churn).
func Classify(path string) (Class, *TypeMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, fmt.Errorf("parsing manifest %q: %w", path, err)
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
