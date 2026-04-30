package manifest

import (
	"os"
	"strings"
	"testing"
)

func TestIsShrineAPIVersion(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect bool
	}{
		{name: "strict shrine v1", input: "shrine/v1", expect: true},
		{name: "strict shrine v1beta1", input: "shrine/v1beta1", expect: true},
		{name: "strict shrine v10alpha7", input: "shrine/v10alpha7", expect: true},
		{name: "capital S typo", input: "Shrine/v1", expect: false},
		{name: "plural typo", input: "shrines/v1", expect: false},
		{name: "no version suffix", input: "shrine/dev", expect: false},
		{name: "trailing space", input: "shrine/v1 ", expect: false},
		{name: "empty apiVersion", input: "", expect: false},
		{name: "foreign apiVersion", input: "traefik.containo.us/v1alpha1", expect: false},
		{name: "missing apiVersion (empty)", input: "", expect: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsShrineAPIVersion(tc.input)
			if got != tc.expect {
				t.Errorf("IsShrineAPIVersion(%q) = %v, want %v", tc.input, got, tc.expect)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name          string
		content       []byte
		expectClass   Class
		expectErr     bool
		expectAPIVer  string
		expectKind    string
		checkTypeMeta bool
	}{
		{
			name:          "strict shrine v1",
			content:       []byte("apiVersion: shrine/v1\nkind: Application\n"),
			expectClass:   ClassShrine,
			expectAPIVer:  "shrine/v1",
			expectKind:    "Application",
			checkTypeMeta: true,
		},
		{
			name:        "strict shrine v1beta1",
			content:     []byte("apiVersion: shrine/v1beta1\nkind: Resource\n"),
			expectClass: ClassShrine,
		},
		{
			name:        "strict shrine v10alpha7",
			content:     []byte("apiVersion: shrine/v10alpha7\nkind: Team\n"),
			expectClass: ClassShrine,
		},
		{
			name:        "capital S typo",
			content:     []byte("apiVersion: Shrine/v1\nkind: Application\n"),
			expectClass: ClassForeign,
		},
		{
			name:        "plural typo",
			content:     []byte("apiVersion: shrines/v1\nkind: Application\n"),
			expectClass: ClassForeign,
		},
		{
			name:        "no version suffix",
			content:     []byte("apiVersion: shrine/dev\nkind: Application\n"),
			expectClass: ClassForeign,
		},
		{
			name:        "trailing space quoted scalar",
			content:     []byte("apiVersion: \"shrine/v1 \"\nkind: Application\n"),
			expectClass: ClassForeign,
		},
		{
			name:        "empty apiVersion",
			content:     []byte("apiVersion: \"\"\nkind: Application\n"),
			expectClass: ClassForeign,
		},
		{
			name:        "missing apiVersion",
			content:     []byte("kind: Application\n"),
			expectClass: ClassForeign,
		},
		{
			name:        "foreign apiVersion",
			content:     []byte("apiVersion: traefik.containo.us/v1alpha1\nkind: IngressRoute\n"),
			expectClass: ClassForeign,
		},
		{
			name:        "empty file",
			content:     []byte(""),
			expectClass: ClassForeign,
		},
		{
			name:        "comments only",
			content:     []byte("# nothing here\n"),
			expectClass: ClassForeign,
		},
		{
			name:      "malformed YAML",
			content:   []byte("apiVersion: shrine/v1\nkind: [unclosed"),
			expectErr: true,
		},
		{
			name:        "shrine v1 with bogus kind",
			content:     []byte("apiVersion: shrine/v1\nkind: Aplication\n"),
			expectClass: ClassShrine,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := dir + "/manifest.yaml"
			if err := os.WriteFile(path, tc.content, 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			class, meta, err := Classify(path)

			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), path) {
					t.Errorf("error %q does not contain file path %q", err.Error(), path)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if class != tc.expectClass {
				t.Errorf("class = %v, want %v", class, tc.expectClass)
			}

			if tc.checkTypeMeta {
				if meta == nil {
					t.Fatalf("expected non-nil TypeMeta for ClassShrine, got nil")
				}
				if meta.APIVersion != tc.expectAPIVer {
					t.Errorf("APIVersion = %q, want %q", meta.APIVersion, tc.expectAPIVer)
				}
				if meta.Kind != tc.expectKind {
					t.Errorf("Kind = %q, want %q", meta.Kind, tc.expectKind)
				}
			}

			if class == ClassShrine && meta == nil {
				t.Errorf("ClassShrine result must have non-nil TypeMeta")
			}

			if class == ClassForeign && meta != nil {
				t.Errorf("ClassForeign result must have nil TypeMeta, got %+v", meta)
			}
		})
	}
}
