package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockReleaseDoesNotDeleteAnotherOwner(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)

	lock, err := acquireDaemonLock(&paths, time.Minute, "owner-a")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "owner-b", PID: 22, CreatedAt: time.Now().UTC()})

	lock.release()

	record := readLockRecordForTest(t, &paths)
	if record.Token != "owner-b" {
		t.Fatalf("lock token after release = %q, want owner-b", record.Token)
	}
}

func TestStaleLockRecoveryDoesNotRemoveReplacedOwner(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)
	old := time.Now().Add(-2 * time.Hour)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "stale-owner", PID: 11, CreatedAt: old})
	if err := os.Chtimes(paths.LockPath, old, old); err != nil {
		t.Fatal(err)
	}
	restoreHook := setBeforeObservedLockRemoveHookForTest(func() {
		writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "new-owner", PID: 33, CreatedAt: time.Now().UTC()})
	})
	defer restoreHook()

	lock, err := acquireDaemonLock(&paths, time.Millisecond, "requester")
	if err == nil {
		lock.release()
		t.Fatal("expected replaced lock to remain busy")
	}
	if !errors.Is(err, ErrStartInProgress) {
		t.Fatalf("error = %v, want ErrStartInProgress", err)
	}
	record := readLockRecordForTest(t, &paths)
	if record.Token != "new-owner" {
		t.Fatalf("lock token after failed stale recovery = %q, want new-owner", record.Token)
	}
}

func TestChildAdoptFailsWhenTokenMismatches(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "parent-token", PID: 11, CreatedAt: time.Now().UTC()})

	lock, err := adoptDaemonLock(&paths, "child-token")
	if err == nil {
		lock.release()
		t.Fatal("expected token mismatch to fail")
	}
	if !strings.Contains(err.Error(), "token mismatch") {
		t.Fatalf("error = %v", err)
	}
	record := readLockRecordForTest(t, &paths)
	if record.Token != "parent-token" {
		t.Fatalf("lock token after failed adopt = %q, want parent-token", record.Token)
	}
}

func writeLockRecordForTest(t *testing.T, paths *Paths, record daemonLockRecord) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(paths.LockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeLockRecord(paths.LockPath, record); err != nil {
		t.Fatal(err)
	}
}

func readLockRecordForTest(t *testing.T, paths *Paths) daemonLockRecord {
	t.Helper()
	record, _, err := readLockRecord(paths.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
