//go:build integration

package integration_test

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCCXProfileAddListUseDashboardFlow(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}

	env := append(
		os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"SHELL=/bin/zsh",
	)

	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		return out.String(), err
	}

	if out, err := run("profile", "add", "work", "--config-dir", cfgDir); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}

	out, err := run("profile", "list")
	if err != nil || !strings.Contains(out, "work") {
		t.Fatalf("list: %v\n%s", err, out)
	}

	out, err = run("use", "work")
	if err != nil || !strings.Contains(out, "CLAUDE_CONFIG_DIR") {
		t.Fatalf("use: %v\n%s", err, out)
	}

	var dashOut bytes.Buffer
	port := freeTCPPort(t)
	dashCmd := exec.Command(bin, "dashboard", "--no-open", "--port", port)
	dashCmd.Env = env
	dashCmd.Stdout = &dashOut
	dashCmd.Stderr = &dashOut
	if err := dashCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if dashCmd.Process != nil {
			_ = dashCmd.Process.Kill()
			_, _ = dashCmd.Process.Wait()
		}
	}()

	res, err := waitForHealth("http://127.0.0.1:" + port + "/api/health")
	if err != nil {
		t.Fatalf("health: %v\n%s", err, dashOut.String())
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health code=%d\n%s", res.StatusCode, dashOut.String())
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	name := "ccx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ccx")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr %T", ln.Addr())
	}
	return strconv.Itoa(addr.Port)
}

func waitForHealth(url string) (*http.Response, error) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		res, err := client.Get(url)
		if err == nil {
			return res, nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for %s: %w", url, lastErr)
}
