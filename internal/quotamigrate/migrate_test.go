package quotamigrate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/quotamigrate"
)

func TestPlanLinksMissingProjects(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()

	profile := contracts.Profile{Name: "work", ConfigDir: profileDir}
	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{profile})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("steps: got %d, want 1", len(steps))
	}
	if steps[0].Action != quotamigrate.ActionCreateSymlink {
		t.Errorf("action: got %v, want CreateSymlink", steps[0].Action)
	}
}

func TestPlanMovesAndLinksExistingDir(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(profileDir, "projects/foo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "projects/foo/sess.jsonl"), []byte(`{"type":"user"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	profile := contracts.Profile{Name: "work", ConfigDir: profileDir}
	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{profile})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("steps: got %d, want 2 (move + symlink)", len(steps))
	}
	if steps[0].Action != quotamigrate.ActionMoveContents {
		t.Errorf("step 0: got %v", steps[0].Action)
	}
	if steps[1].Action != quotamigrate.ActionCreateSymlink {
		t.Errorf("step 1: got %v", steps[1].Action)
	}
}

func TestPlanSkipsAlreadyLinked(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	shared := quotamigrate.SharedProjectsPath(ccxHome)
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	requireSymlink(t, shared, filepath.Join(profileDir, "projects"))

	profile := contracts.Profile{Name: "work", ConfigDir: profileDir}
	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{profile})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("already-linked should plan zero steps; got %+v", steps)
	}
}

func TestApplyExecutesPlan(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(profileDir, "projects/foo"), 0o700); err != nil {
		t.Fatal(err)
	}
	migratedFile := filepath.Join(profileDir, "projects/foo/sess.jsonl")
	if err := os.WriteFile(migratedFile, []byte(`x`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(migratedFile, 0o640); err != nil { // #nosec G302 -- the test verifies mode preservation.
		t.Fatal(err)
	}

	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{{Name: "work", ConfigDir: profileDir}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if err := quotamigrate.Apply(steps); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	target, err := os.Readlink(filepath.Join(profileDir, "projects"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != quotamigrate.SharedProjectsPath(ccxHome) {
		t.Errorf("symlink target: got %q", target)
	}

	movedPath := filepath.Join(quotamigrate.SharedProjectsPath(ccxHome), "foo/sess.jsonl")
	info, err := os.Stat(movedPath)
	if err != nil {
		t.Fatalf("expected moved file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("moved file mode: got %v, want %v", got, os.FileMode(0o640))
	}
}

func TestApplyRefusesDestinationCollisionBeforeCopying(t *testing.T) {
	src := filepath.Join(t.TempDir(), "projects")
	dst := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.jsonl"), []byte("new-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "b.jsonl"), []byte("new-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "b.jsonl"), []byte("old-b"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := quotamigrate.Apply([]quotamigrate.Step{{
		Action: quotamigrate.ActionMoveContents,
		From:   src,
		To:     dst,
	}})
	if err == nil {
		t.Fatal("Apply should refuse destination collision")
	}
	if _, statErr := os.Stat(filepath.Join(dst, "a.jsonl")); !os.IsNotExist(statErr) {
		t.Fatalf("preflight should avoid partial copy; stat err=%v", statErr)
	}
}

func TestApplyMoveContentsAllowsIdenticalCopiedFileOnRerun(t *testing.T) {
	src := filepath.Join(t.TempDir(), "projects")
	dst := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.jsonl"), []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "a.jsonl"), []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := quotamigrate.Apply([]quotamigrate.Step{{
		Action: quotamigrate.ActionMoveContents,
		From:   src,
		To:     dst,
	}}); err != nil {
		t.Fatalf("Apply should allow identical existing file: %v", err)
	}
	if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
		t.Fatalf("source tree should be removed after successful resumable merge; stat err=%v", statErr)
	}
}

func TestApplyRefusesDestinationSymlinkAncestor(t *testing.T) {
	src := filepath.Join(t.TempDir(), "projects")
	dst := filepath.Join(t.TempDir(), "shared")
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(filepath.Join(src, "foo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	requireSymlink(t, outside, filepath.Join(dst, "foo"))
	if err := os.WriteFile(filepath.Join(src, "foo/bar.jsonl"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := quotamigrate.Apply([]quotamigrate.Step{{
		Action: quotamigrate.ActionMoveContents,
		From:   src,
		To:     dst,
	}})
	if err == nil {
		t.Fatal("Apply should refuse destination symlink ancestor")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "bar.jsonl")); !os.IsNotExist(statErr) {
		t.Fatalf("migration escaped shared directory; stat err=%v", statErr)
	}
}

func TestApplyRefusesSourceSymlink(t *testing.T) {
	src := filepath.Join(t.TempDir(), "projects")
	dst := filepath.Join(t.TempDir(), "shared")
	outsideFile := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	requireSymlink(t, outsideFile, filepath.Join(src, "linked.jsonl"))

	err := quotamigrate.Apply([]quotamigrate.Step{{
		Action: quotamigrate.ActionMoveContents,
		From:   src,
		To:     dst,
	}})
	if err == nil {
		t.Fatal("Apply should refuse source symlink")
	}
	if _, statErr := os.Stat(filepath.Join(dst, "linked.jsonl")); !os.IsNotExist(statErr) {
		t.Fatalf("source symlink should not be copied; stat err=%v", statErr)
	}
}

func TestApplyCreateSymlinkIsIdempotent(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	shared := quotamigrate.SharedProjectsPath(ccxHome)
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(profileDir, "projects")
	requireSymlink(t, shared, projects)

	if err := quotamigrate.Apply([]quotamigrate.Step{{
		Action: quotamigrate.ActionCreateSymlink,
		From:   projects,
		To:     shared,
	}}); err != nil {
		t.Fatalf("Apply idempotent symlink: %v", err)
	}
}

func requireSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
}
