package daemon

import (
	"context"
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

	lock, err := acquireDaemonLock(&paths, time.Minute, "owner-a", testProcessIdentity(os.Getpid()), func(*daemonLockRecord) bool { return false })
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

func TestLockReleaseDoesNotDeleteSameTokenDifferentPID(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)

	lock, err := acquireDaemonLock(&paths, time.Minute, "shared-token", testProcessIdentity(os.Getpid()), func(*daemonLockRecord) bool { return false })
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "shared-token", PID: 99, CreatedAt: time.Now().UTC()})

	lock.release()

	record := readLockRecordForTest(t, &paths)
	if record.Token != "shared-token" || record.PID != 99 {
		t.Fatalf("lock after release = %+v, want same token pid 99", record)
	}
}

func TestLockReleaseDoesNotDeleteReplacedLockAfterRead(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)

	lock, err := acquireDaemonLock(&paths, time.Minute, "owner-a", testProcessIdentity(os.Getpid()), func(*daemonLockRecord) bool { return false })
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	restoreHook := setBeforeObservedLockRemoveHookForTest(func() {
		writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "owner-b", PID: 22, CreatedAt: time.Now().UTC()})
	})
	defer restoreHook()

	lock.release()

	record := readLockRecordForTest(t, &paths)
	if record.Token != "owner-b" || record.PID != 22 {
		t.Fatalf("lock after release race = %+v, want owner-b pid 22", record)
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

	lock, err := acquireDaemonLock(&paths, time.Millisecond, "requester", testProcessIdentity(os.Getpid()), func(*daemonLockRecord) bool { return false })
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

func TestStaleLockRecoveryKeepsLiveOwner(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)
	old := time.Now().Add(-2 * time.Hour)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "live-owner", PID: 44, CreatedAt: old})

	lock, err := acquireDaemonLock(&paths, time.Millisecond, "requester", testProcessIdentity(os.Getpid()), func(record *daemonLockRecord) bool {
		return record != nil && record.PID == 44 && record.ProcessIdentity == testProcessIdentity(44)
	})
	if err == nil {
		lock.release()
		t.Fatal("expected live stale owner to block acquire")
	}
	if !errors.Is(err, ErrStartInProgress) {
		t.Fatalf("error = %v, want ErrStartInProgress", err)
	}
	record := readLockRecordForTest(t, &paths)
	if record.Token != "live-owner" || record.PID != 44 {
		t.Fatalf("lock after blocked acquire = %+v, want live owner", record)
	}
}

func TestChildAdoptFailsWhenTokenMismatches(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "parent-token", PID: 11, ChildPID: 22, CreatedAt: time.Now().UTC()})

	lock, err := adoptDaemonLock(&paths, "child-token", 11, 22, testProcessIdentity(22))
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

func TestChildAdoptFailsWhenLockMissing(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)

	lock, err := adoptDaemonLock(&paths, "parent-token", 11, 22, testProcessIdentity(22))
	if err == nil {
		lock.release()
		t.Fatal("expected missing lock adopt to fail")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestChildAdoptDoesNotClobberReplacedLock(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "parent-token", PID: 11, ChildPID: 22, CreatedAt: time.Now().UTC()})
	restoreHook := setBeforeLockAdoptWriteHookForTest(func() {
		writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "replacement-token", PID: 33, CreatedAt: time.Now().UTC()})
	})
	defer restoreHook()

	lock, err := adoptDaemonLock(&paths, "parent-token", 11, 22, testProcessIdentity(22))
	if err == nil {
		lock.release()
		t.Fatal("expected replaced lock adopt to fail")
	}
	record := readLockRecordForTest(t, &paths)
	if record.Token != "replacement-token" || record.PID != 33 {
		t.Fatalf("lock after failed adopt = %+v, want replacement", record)
	}
}

func TestChildAdoptWithRetryWaitsForParentChildPIDClaim(t *testing.T) {
	root := t.TempDir()
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "parent-token", PID: 11, CreatedAt: time.Now().UTC()})

	go func() {
		time.Sleep(25 * time.Millisecond)
		writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "parent-token", PID: 11, ChildPID: 22, CreatedAt: time.Now().UTC()})
	}()

	lock, err := adoptDaemonLockWithRetry(context.Background(), &paths, "parent-token", 11, 22, testProcessIdentity(22), time.Second)
	if err != nil {
		t.Fatalf("adopt with retry: %v", err)
	}
	defer lock.release()
	record := readLockRecordForTest(t, &paths)
	if record.Token != "parent-token" || record.PID != 22 || record.ChildPID != 0 {
		t.Fatalf("lock after retry adopt = %+v, want child ownership", record)
	}
}

func writeLockRecordForTest(t *testing.T, paths *Paths, record daemonLockRecord) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(paths.LockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if record.ProcessIdentity == "" && record.PID > 0 {
		record.ProcessIdentity = testProcessIdentity(record.PID)
	}
	if record.ChildProcessIdentity == "" && record.ChildPID > 0 {
		record.ChildProcessIdentity = testProcessIdentity(record.ChildPID)
	}
	if err := writeLockRecord(paths.LockPath, &record); err != nil {
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
