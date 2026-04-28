//go:build integration

package integration_test

import (
	"path/filepath"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func TestGenerateTeam(t *testing.T) {
	s := NewSuite(t)

	s.Test("should generate a new team manifest", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "team", "my-team", "--path", dir).
			AssertSuccess().
			AssertFileExists(tc.Path("my-team.yml")).
			AssertSpecHas(tc.Path("my-team.yml"), "kind", "Team").
			AssertSpecHas(tc.Path("my-team.yml"), "metadata.name", "my-team")
	})

	s.Test("should fail when a manifest with the same name already exists", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "team", "my-team", "--path", dir).
			AssertSuccess()
		tc.Run("generate", "team", "my-team", "--path", dir).
			AssertFailure().
			AssertStderrContains("already exists")
	})

	s.Test("should write the manifest to the directory specified by --path", func(tc *TestCase) {
		customDir := filepath.Join(tc.TempDir(), "nested", "custom")
		tc.Run("generate", "team", "my-team", "--path", customDir).
			AssertSuccess().
			AssertFileExists(filepath.Join(customDir, "my-team.yml"))
	})
}

func TestGenerateApplication(t *testing.T) {
	s := NewSuite(t)

	s.Test("should generate a new application manifest with defaults", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "application", "my-app", "--path", dir).
			AssertSuccess().
			AssertFileExists(tc.Path("my-app.yml")).
			AssertSpecHas(tc.Path("my-app.yml"), "kind", "Application").
			AssertSpecHas(tc.Path("my-app.yml"), "metadata.name", "my-app")
	})

	s.Test("should populate manifest fields from flags", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "application", "backend", "--path", dir, "--team", "platform", "--port", "3000", "--replicas", "3", "--domain", "backend.example.com", "--pathprefix", "/api", "--expose", "--image", "my-registry/backend:v1").
			AssertSuccess().
			AssertFileExists(tc.Path("backend.yml")).
			AssertSpecHas(tc.Path("backend.yml"), "metadata.owner", "platform").
			AssertSpecHas(tc.Path("backend.yml"), "spec.image", "my-registry/backend:v1").
			AssertSpecHas(tc.Path("backend.yml"), "spec.port", "3000").
			AssertSpecHas(tc.Path("backend.yml"), "spec.replicas", "3").
			AssertSpecHas(tc.Path("backend.yml"), "spec.routing.domain", "backend.example.com").
			AssertSpecHas(tc.Path("backend.yml"), "spec.routing.pathPrefix", "/api").
			AssertSpecHas(tc.Path("backend.yml"), "spec.networking.exposeToPlatform", "true")
	})

	s.Test("should fail when a manifest with the same name already exists", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "application", "my-app", "--path", dir).
			AssertSuccess()
		tc.Run("generate", "application", "my-app", "--path", dir).
			AssertFailure().
			AssertStderrContains("already exists")
	})

	s.Test("should write the manifest to the directory specified by --path", func(tc *TestCase) {
		customDir := filepath.Join(tc.TempDir(), "apps", "v2")
		tc.Run("generate", "application", "my-app", "--path", customDir).
			AssertSuccess().
			AssertFileExists(filepath.Join(customDir, "my-app.yml"))
	})
}

func TestGenerateResource(t *testing.T) {
	s := NewSuite(t)

	s.Test("should generate a new resource manifest with defaults", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "resource", "my-db", "--path", dir).
			AssertSuccess().
			AssertFileExists(tc.Path("my-db.yml")).
			AssertSpecHas(tc.Path("my-db.yml"), "kind", "Resource").
			AssertSpecHas(tc.Path("my-db.yml"), "metadata.name", "my-db")
	})

	s.Test("should populate manifest fields from flags", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "resource", "cache", "--path", dir, "--team", "platform", "--type", "redis", "--version", "7", "--expose").
			AssertSuccess().
			AssertFileExists(tc.Path("cache.yml")).
			AssertSpecHas(tc.Path("cache.yml"), "metadata.owner", "platform").
			AssertSpecHas(tc.Path("cache.yml"), "spec.type", "redis").
			AssertSpecHas(tc.Path("cache.yml"), "spec.version", "7").
			AssertSpecHas(tc.Path("cache.yml"), "spec.networking.exposeToPlatform", "true")
	})

	s.Test("should fail when a manifest with the same name already exists", func(tc *TestCase) {
		dir := tc.TempDir()
		tc.Run("generate", "resource", "my-db", "--path", dir).
			AssertSuccess()
		tc.Run("generate", "resource", "my-db", "--path", dir).
			AssertFailure().
			AssertStderrContains("already exists")
	})

	s.Test("should write the manifest to the directory specified by --path", func(tc *TestCase) {
		customDir := filepath.Join(tc.TempDir(), "resources", "databases")
		tc.Run("generate", "resource", "my-db", "--path", customDir).
			AssertSuccess().
			AssertFileExists(filepath.Join(customDir, "my-db.yml"))
	})
}
