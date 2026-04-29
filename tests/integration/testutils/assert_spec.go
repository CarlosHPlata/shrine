//go:build integration

package testutils

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func (tc *TestCase) AssertSpecHas(filePath, key, expectedValue string) *TestCase {
	tc.t.Helper()
	data, err := os.ReadFile(filePath)
	if err != nil {
		tc.t.Fatalf("failed to read file %s: %v", filePath, err)
	}
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		tc.t.Fatalf("failed to parse YAML at %s: %v", filePath, err)
	}
	value, ok := navigateDotPath(parsed, key)
	if !ok {
		tc.t.Fatalf("key %q not found in %s", key, filePath)
	}
	actual := fmt.Sprintf("%v", value)
	if actual != expectedValue {
		tc.t.Fatalf("key %q: expected %q, got %q", key, expectedValue, actual)
	}
	return tc
}

func navigateDotPath(data map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.SplitN(path, ".", 2)
	val, ok := data[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return val, true
	}
	nested, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return navigateDotPath(nested, parts[1])
}
