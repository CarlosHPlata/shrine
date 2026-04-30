package config

import (
	"strings"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	const fakeHome = "/home/test-user"

	cases := []struct {
		name    string
		home    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty path is left empty", home: fakeHome, input: "", want: ""},
		{name: "absolute path is unchanged", home: fakeHome, input: "/etc/shrine", want: "/etc/shrine"},
		{name: "plain relative path is unchanged", home: fakeHome, input: "foo/bar", want: "foo/bar"},
		{name: "single tilde resolves to HOME", home: fakeHome, input: "~", want: fakeHome},
		{name: "tilde-slash prefix expands to HOME", home: fakeHome, input: "~/foo", want: fakeHome + "/foo"},
		{name: "tilde-slash prefix expands deep paths", home: fakeHome, input: "~/foo/bar/baz", want: fakeHome + "/foo/bar/baz"},
		{name: "tilde-username form is unchanged", home: fakeHome, input: "~alice", want: "~alice"},
		{name: "tilde-username with subpath is unchanged", home: fakeHome, input: "~alice/foo", want: "~alice/foo"},
		{name: "embedded tilde is unchanged", home: fakeHome, input: "/path/~/middle", want: "/path/~/middle"},
		{name: "idempotent on already-expanded path", home: fakeHome, input: fakeHome + "/foo", want: fakeHome + "/foo"},
		{name: "HOME unset returns error for bare tilde", home: "", input: "~", wantErr: true},
		{name: "HOME unset returns error for tilde-slash", home: "", input: "~/foo", wantErr: true},
		{name: "HOME unset is fine for absolute path (no expansion needed)", home: "", input: "/abs", want: "/abs"},
		{name: "HOME unset is fine for plain relative (no expansion needed)", home: "", input: "rel", want: "rel"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			got, err := expandTilde(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expandTilde(%q) succeeded with %q, want error", tc.input, got)
				}
				if !strings.Contains(err.Error(), "expanding ~") {
					t.Errorf("error %q should mention 'expanding ~'", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("expandTilde(%q) returned unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("expandTilde(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
