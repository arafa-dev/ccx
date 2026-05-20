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

func TestCCXHomeCreatesDirIfMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := platform.CCXHome()
	if err != nil {
		t.Fatalf("CCXHome: %v", err)
	}

	want := filepath.Join(tmp, ".ccx")
	if got != want {
		t.Errorf("CCXHome = %q, want %q", got, want)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat %q: %v", got, err)
	}
	if !info.IsDir() {
		t.Errorf("CCXHome %q is not a directory", got)
	}
	// Permissions check is Unix-only; Windows uses ACLs and reports 0777-ish.
	if runtimeIsUnix() {
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("CCXHome perm = %o, want 0700", perm)
		}
	}
}

func TestCCXHomeIdempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	first, err := platform.CCXHome()
	if err != nil {
		t.Fatalf("first CCXHome: %v", err)
	}
	second, err := platform.CCXHome()
	if err != nil {
		t.Fatalf("second CCXHome: %v", err)
	}
	if first != second {
		t.Errorf("CCXHome not idempotent: %q vs %q", first, second)
	}
}

func runtimeIsUnix() bool {
	return os.PathSeparator == '/'
}

func TestDefaultConfigDirReturnsAbsolutePath(t *testing.T) {
	// Clear the override so we exercise the OS default branch.
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := platform.DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	if got == "" {
		t.Fatal("DefaultConfigDir returned empty")
	}
	if !filepath.IsAbs(got) {
		t.Errorf("DefaultConfigDir = %q is not absolute", got)
	}
	// It should be located under the (fake) home.
	if !strings.HasPrefix(got, tmp) {
		t.Errorf("DefaultConfigDir = %q should be under HOME %q", got, tmp)
	}
}

func TestDefaultConfigDirRespectsEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	override := filepath.Join(tmp, "custom-claude")
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("CLAUDE_CONFIG_DIR", override)

	got, err := platform.DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	if got != override {
		t.Errorf("DefaultConfigDir = %q, want override %q", got, override)
	}
}

func TestDefaultConfigDirExpandsTildeInOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("CLAUDE_CONFIG_DIR", "~/elsewhere")

	got, err := platform.DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	want := filepath.Join(tmp, "elsewhere")
	if got != want {
		t.Errorf("DefaultConfigDir = %q, want %q", got, want)
	}
}
