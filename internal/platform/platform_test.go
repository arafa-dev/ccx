package platform_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/platform"
)

func TestExpandPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Windows respects USERPROFILE; set both so the test is OS-portable.
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("CCX_TEST_VAR", "expanded")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde alone", "~", tmp},
		{"tilde with subpath", "~/foo/bar", filepath.Join(tmp, "foo", "bar")},
		{"env var", "$CCX_TEST_VAR/x", filepath.Join(cwd, "expanded", "x")},
		{"already absolute", filepath.Join(tmp, "abs"), filepath.Join(tmp, "abs")},
		{"relative", "rel/path", filepath.Join(cwd, "rel", "path")},
		{"double slash cleaned", "~//foo", filepath.Join(tmp, "foo")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := platform.ExpandPath(tc.in)
			if err != nil {
				t.Fatalf("ExpandPath(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("ExpandPath(%q) = %q is not absolute", tc.in, got)
			}
		})
	}
}

func TestExpandPathEmptyInputErrors(t *testing.T) {
	if _, err := platform.ExpandPath(""); err == nil {
		t.Fatal("ExpandPath(\"\") should return an error")
	} else if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error %q should mention empty input", err.Error())
	}
}
