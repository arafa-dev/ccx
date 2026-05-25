package quotamigrate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPreflightsSymlinkBeforeMovingContents(t *testing.T) {
	src := filepath.Join(t.TempDir(), "projects")
	shared := filepath.Join(t.TempDir(), "shared-projects")
	session := filepath.Join(src, "proj", "sess.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte(`{"type":"assistant"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	oldCreateProjectsLink := createProjectsLinkFunc
	createProjectsLinkFunc = func(_, _ string) error {
		return errors.New("symlink unavailable")
	}
	t.Cleanup(func() {
		createProjectsLinkFunc = oldCreateProjectsLink
	})

	err := Apply([]Step{
		{Profile: "work", Action: ActionMoveContents, From: src, To: shared},
		{Profile: "work", Action: ActionCreateSymlink, From: src, To: shared},
	})
	if err == nil {
		t.Fatal("expected Apply to fail")
	}
	if !strings.Contains(err.Error(), "preflight symlink") {
		t.Fatalf("error = %v, want preflight symlink failure", err)
	}
	if _, err := os.Stat(session); err != nil {
		t.Fatalf("source session should remain before failed symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shared, "proj", "sess.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("shared session should not be copied before failed symlink; stat err=%v", err)
	}
}
