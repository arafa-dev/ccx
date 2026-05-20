package scanner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/scanner"
)

func TestScannerConcurrentScanOfTenFilesCompletesQuickly(t *testing.T) {
	dir := t.TempDir()

	src, err := os.ReadFile(filepath.Join("testdata", "fixtures", "sample-session.jsonl")) // #nosec G304 -- fixed test fixture path.
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	for i := 0; i < 10; i++ {
		path := filepath.Join(dir, "projects", fmt.Sprintf("proj-%d", i), "sess-001.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, src, 0o600); err != nil { // #nosec G703 -- path is under t.TempDir.
			t.Fatalf("write: %v", err)
		}
	}

	profile := contracts.Profile{Name: "p", ConfigDir: dir}
	s := scanner.NewScanner(scanner.NewMemoryCursorStore())

	start := time.Now()
	events, errs := s.Scan(context.Background(), profile)

	count := 0
	for range events {
		count++
	}
	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if count != 50 {
		t.Errorf("got %d events, want 50 (10 files x 5)", count)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("scan of 10 files took %v, want <100ms", elapsed)
	}
}
