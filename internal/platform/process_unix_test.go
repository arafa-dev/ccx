//go:build darwin || linux

package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProcessAliveTreatsZombieAsDead(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start short-lived child: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Wait()
	})
	pid := cmd.Process.Pid
	eventuallyProcessTest(t, func() bool {
		zombie, err := processZombieStateForTest(pid)
		return err == nil && zombie
	})

	if ProcessAlive(pid) {
		t.Fatalf("zombie pid %d reported alive", pid)
	}
}

func processZombieStateForTest(pid int) (bool, error) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat") //nolint:gosec // Test-owned pid from child process.
		if err != nil {
			return false, err
		}
		fields := strings.Fields(string(data))
		return len(fields) > 2 && fields[2] == "Z", nil
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // Command is constant and pid is test-owned.
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(strings.TrimSpace(string(out)), "Z"), nil
}

func eventuallyProcessTest(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok() {
		t.Fatal("condition not met before deadline")
	}
}
