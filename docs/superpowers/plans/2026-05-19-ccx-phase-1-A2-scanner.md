# ccx Phase 1 — A2 `internal/scanner/` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `internal/scanner/` — a defensive, concurrent JSONL walker that turns a profile's `<ConfigDir>/projects/*/<session-uuid>.jsonl` files into a stream of `contracts.Event`s. Incremental scanning is driven by an injectable `CursorStore` so unit tests can use an in-memory map while Phase 2 backs it with `internal/storage`.

**Architecture:** Stdlib only. The package exports a `Scanner` type that implements `contracts.Scanner`. Parsing is line-oriented (`bufio.Scanner` with a generous buffer), defensive (unknown event types and malformed lines are logged and skipped — never panic), and fuzz-tested. File-level concurrency is bounded by a worker pool sized to `runtime.NumCPU()`. Per-file cursors store byte offset + inode; inode mismatch forces a re-scan from offset 0.

**Tech Stack:** Go 1.22+, stdlib only (`encoding/json`, `bufio`, `os`, `io/fs`, `path/filepath`, `net/url`, `log/slog`, `context`, `sync`, `runtime`, `syscall`, `testing`, `testing/fstest`).

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 7.1 (Scanner).

**Worktree:** `feat/scanner` off `main`, created after Phase 0 is merged.

```bash
git worktree add ../ccx-scanner -b feat/scanner main
cd ../ccx-scanner
```

**Exit criteria:**
- `go build ./internal/scanner/...` succeeds
- `go test -race -count=1 ./internal/scanner/...` is green
- `go test -fuzz=FuzzParseLine -fuzztime=30s ./internal/scanner` runs without panic and finds no crashers
- `golangci-lint run ./internal/scanner/...` reports zero issues
- Package depends only on `github.com/arafa-dev/ccx/internal/contracts` and the Go stdlib (verified via `go list -deps`)
- A PR against `main` is opened and CI is green

---

## Pre-flight

Confirm the working directory is the worktree and that Phase 0 artifacts are present.

```bash
pwd                                         # → .../ccx-scanner
git rev-parse --abbrev-ref HEAD             # → feat/scanner
git log --oneline | head                    # → includes Phase 0 commits + phase-0 tag
ls internal/contracts                       # → types.go, errors.go, interfaces.go
go build ./...                              # → succeeds
go test ./internal/contracts/...            # → PASS
```

**Conventions for this plan (per `docs/conventions.md`):**
- Tabs for Go indentation; `gofumpt -w .` before every commit
- Test files use black-box `package scanner_test` unless they need to touch unexported helpers (then `package scanner`)
- Wrap errors with context: `fmt.Errorf("scanning %q: %w", path, err)`
- Logging via `log/slog`; never `fmt.Print*` in library code
- One commit per task with `type(scope): subject` messages, scope `scanner`
- `go test -race -count=1 ./internal/scanner/...` and `golangci-lint run ./internal/scanner/...` must pass before every commit

---

## Task 1: Package skeleton — Scanner struct and CursorStore interface

**Files:**
- Modify: `internal/scanner/doc.go` (extend the existing Phase 0 stub)
- Create: `internal/scanner/scanner.go`
- Create: `internal/scanner/cursor.go`
- Create: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/scanner/scanner_test.go`:

```go
package scanner_test

import (
	"context"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/scanner"
)

func TestNewScannerImplementsContractsScanner(t *testing.T) {
	var _ contracts.Scanner = scanner.NewScanner(scanner.NewMemoryCursorStore())
}

func TestScannerScanReturnsClosedChannelsForMissingDir(t *testing.T) {
	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	profile := contracts.Profile{Name: "p", ConfigDir: t.TempDir()}

	events, errs := s.Scan(context.Background(), profile)

	count := 0
	for range events {
		count++
	}
	for err := range errs {
		t.Errorf("unexpected error for empty profile: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 events for empty profile, got %d", count)
	}
}

func TestMemoryCursorStoreGetSet(t *testing.T) {
	ctx := context.Background()
	cs := scanner.NewMemoryCursorStore()

	got, err := cs.Get(ctx, "work", "/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (scanner.Cursor{}) {
		t.Errorf("expected zero cursor for missing key, got %+v", got)
	}

	want := scanner.Cursor{Offset: 42, Inode: 99}
	if err := cs.Set(ctx, "work", "/tmp/a.jsonl", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err = cs.Get(ctx, "work", "/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != want {
		t.Errorf("Get after Set: got %+v want %+v", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/scanner/...
```

Expected: FAIL — `scanner.NewScanner`, `scanner.Cursor`, `scanner.NewMemoryCursorStore` undefined.

- [ ] **Step 3: Write the minimal implementation**

Replace `internal/scanner/doc.go` with:

```go
// Package scanner walks a profile's JSONL session files and emits parsed
// contracts.Events. It is defensive: unknown event types and malformed lines
// are logged and skipped, never panicked on. File-level concurrency is bounded
// by a worker pool sized to runtime.NumCPU(). Incremental scanning is driven
// by an injectable CursorStore so unit tests can use an in-memory map while
// Phase 2 backs it with internal/storage.
package scanner
```

Create `internal/scanner/cursor.go`:

```go
package scanner

import (
	"context"
	"sync"
)

// Cursor is the per-file position checkpoint used for incremental scanning.
// Offset is the next byte to read; Inode is the underlying file inode at the
// time the offset was recorded. If the inode changes on a subsequent scan,
// the file is assumed to have been rotated or replaced and the scan restarts
// from offset 0.
type Cursor struct {
	Offset int64
	Inode  uint64
}

// CursorStore persists per-file scan checkpoints. In Phase 2 it is backed by
// internal/storage; unit tests use NewMemoryCursorStore.
type CursorStore interface {
	// Get returns the saved cursor for (profile, file). If absent, returns
	// the zero-value Cursor and a nil error.
	Get(ctx context.Context, profile, file string) (Cursor, error)
	// Set persists the cursor for (profile, file).
	Set(ctx context.Context, profile, file string, c Cursor) error
}

// memoryCursorStore is an in-memory CursorStore for tests.
type memoryCursorStore struct {
	mu   sync.Mutex
	data map[string]Cursor
}

// NewMemoryCursorStore returns a CursorStore backed by a sync-protected map.
// Suitable for unit tests and short-lived processes; not durable.
func NewMemoryCursorStore() CursorStore {
	return &memoryCursorStore{data: map[string]Cursor{}}
}

func (m *memoryCursorStore) key(profile, file string) string {
	return profile + "\x00" + file
}

func (m *memoryCursorStore) Get(_ context.Context, profile, file string) (Cursor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[m.key(profile, file)], nil
}

func (m *memoryCursorStore) Set(_ context.Context, profile, file string, c Cursor) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[m.key(profile, file)] = c
	return nil
}
```

Create `internal/scanner/scanner.go`:

```go
package scanner

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Scanner implements contracts.Scanner. Create with NewScanner.
type Scanner struct {
	cursors CursorStore
	workers int
	logger  *slog.Logger
}

// NewScanner constructs a Scanner using the given CursorStore. The worker
// pool size defaults to runtime.NumCPU() (minimum 1). The logger defaults to
// slog.Default().
func NewScanner(cs CursorStore) *Scanner {
	w := runtime.NumCPU()
	if w < 1 {
		w = 1
	}
	return &Scanner{cursors: cs, workers: w, logger: slog.Default()}
}

// Scan walks <profile.ConfigDir>/projects/*/<session-uuid>.jsonl and emits
// parsed Events on the returned channel. The events channel is closed when
// scanning completes or ctx is cancelled. The errs channel reports fatal
// errors (e.g., directory traversal failures); it is also closed when done.
// Per-line parse failures are logged and skipped, not reported on errs.
func (s *Scanner) Scan(ctx context.Context, profile contracts.Profile) (<-chan contracts.Event, <-chan error) {
	events := make(chan contracts.Event, 256)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		files, err := s.listJSONL(profile.ConfigDir)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				errs <- err
			}
			return
		}

		_ = files
		_ = ctx
		// File processing is added in later tasks.
	}()

	return events, errs
}

// listJSONL returns every <configDir>/projects/<project>/<session>.jsonl file.
// Missing configDir or projects dir returns fs.ErrNotExist.
func (s *Scanner) listJSONL(configDir string) ([]string, error) {
	projectsDir := filepath.Join(configDir, "projects")
	info, err := os.Stat(projectsDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fs.ErrNotExist
	}

	var out []string
	err = filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".jsonl" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/scanner
golangci-lint run ./internal/scanner/...
git add internal/scanner/
git commit -m "feat(scanner): add Scanner struct and in-memory CursorStore"
```

---

## Task 2: Defensive JSONL line parser (TDD)

**Files:**
- Create: `internal/scanner/parse.go`
- Create: `internal/scanner/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/scanner/parse_test.go`:

```go
package scanner

import (
	"testing"
	"time"
)

func TestParseLineAssistantUsage(t *testing.T) {
	line := []byte(`{"type":"assistant","uuid":"u-1","sessionId":"s-1","timestamp":"2026-05-19T12:00:01Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":200}}}`)

	ev, ok := parseLine(line, "my-project")
	if !ok {
		t.Fatalf("parseLine returned ok=false for valid assistant event")
	}
	if ev.Type != "assistant" {
		t.Errorf("Type = %q want %q", ev.Type, "assistant")
	}
	if ev.UUID != "u-1" {
		t.Errorf("UUID = %q want %q", ev.UUID, "u-1")
	}
	if ev.SessionID != "s-1" {
		t.Errorf("SessionID = %q want %q", ev.SessionID, "s-1")
	}
	if ev.Project != "my-project" {
		t.Errorf("Project = %q want %q", ev.Project, "my-project")
	}
	if ev.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q want %q", ev.Model, "claude-opus-4-7")
	}
	want := time.Date(2026, 5, 19, 12, 0, 1, 0, time.UTC)
	if !ev.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v want %v", ev.Timestamp, want)
	}
	if ev.Usage == nil {
		t.Fatalf("Usage is nil; want non-nil")
	}
	if ev.Usage.InputTokens != 100 || ev.Usage.OutputTokens != 50 ||
		ev.Usage.CacheCreateTokens != 10 || ev.Usage.CacheReadTokens != 200 {
		t.Errorf("Usage = %+v want {100 50 200 10}", *ev.Usage)
	}
}

func TestParseLineUserNoUsage(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u-2","sessionId":"s-1","timestamp":"2026-05-19T12:00:00Z","message":{"content":[{"type":"text","text":"hi"}]}}`)

	ev, ok := parseLine(line, "proj")
	if !ok {
		t.Fatalf("parseLine returned ok=false for valid user event")
	}
	if ev.Type != "user" {
		t.Errorf("Type = %q want user", ev.Type)
	}
	if ev.Usage != nil {
		t.Errorf("Usage should be nil for user event, got %+v", *ev.Usage)
	}
	if ev.Model != "" {
		t.Errorf("Model should be empty for user event, got %q", ev.Model)
	}
}

func TestParseLineRejectsMalformed(t *testing.T) {
	cases := [][]byte{
		[]byte(``),
		[]byte(`   `),
		[]byte(`not json at all`),
		[]byte(`{`),
		[]byte(`{"type":"assistant"`),
		[]byte(`{"type":123}`),
	}
	for i, c := range cases {
		if _, ok := parseLine(c, "p"); ok {
			t.Errorf("case %d: parseLine returned ok=true for malformed input %q", i, c)
		}
	}
}

func TestParseLineRejectsMissingUUID(t *testing.T) {
	line := []byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`)
	if _, ok := parseLine(line, "p"); ok {
		t.Errorf("parseLine returned ok=true for event with no uuid")
	}
}

func TestParseLineRejectsBadTimestamp(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"not-a-time"}`)
	if _, ok := parseLine(line, "p"); ok {
		t.Errorf("parseLine returned ok=true for event with bad timestamp")
	}
}

func TestParseLineIgnoresUnknownFields(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"2026-05-19T12:00:00Z","cwd":"/x","gitBranch":"main","parentUuid":"p","extraFutureField":42}`)
	if _, ok := parseLine(line, "p"); !ok {
		t.Errorf("parseLine returned ok=false; unknown fields should be ignored")
	}
}

func TestProjectNameFromDirURLDecoded(t *testing.T) {
	got := projectNameFromDir("-Users-arafa-Developer-ccx")
	if got != "-Users-arafa-Developer-ccx" {
		t.Errorf("projectNameFromDir(plain) = %q want unchanged", got)
	}
	got = projectNameFromDir("home%2Fuser%2Fproj")
	if got != "home/user/proj" {
		t.Errorf("projectNameFromDir(encoded) = %q want decoded", got)
	}
	got = projectNameFromDir("bad%ZZencoding")
	if got != "bad%ZZencoding" {
		t.Errorf("projectNameFromDir(bad encoding) = %q want raw fallback", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: FAIL — `parseLine`, `projectNameFromDir` undefined.

- [ ] **Step 3: Write the parser**

Create `internal/scanner/parse.go`:

```go
package scanner

import (
	"bytes"
	"encoding/json"
	"net/url"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// rawLine is the on-disk JSONL shape we care about. Fields we don't use are
// intentionally absent so encoding/json skips them. All fields are pointers
// or omitempty-friendly so we can distinguish "missing" from "zero."
type rawLine struct {
	Type      string  `json:"type"`
	UUID      string  `json:"uuid"`
	SessionID string  `json:"sessionId"`
	Timestamp string  `json:"timestamp"`
	Message   *rawMsg `json:"message,omitempty"`
}

type rawMsg struct {
	Model string    `json:"model,omitempty"`
	Usage *rawUsage `json:"usage,omitempty"`
}

type rawUsage struct {
	InputTokens             int `json:"input_tokens"`
	OutputTokens            int `json:"output_tokens"`
	CacheCreationInputToks  int `json:"cache_creation_input_tokens"`
	CacheReadInputToks      int `json:"cache_read_input_tokens"`
}

// parseLine parses one JSONL line into a contracts.Event. It returns ok=false
// for malformed input, missing required fields, or unparsable timestamps.
// Unknown fields are ignored. The project parameter is the URL-decoded
// parent directory name and is assigned to Event.Project.
func parseLine(b []byte, project string) (contracts.Event, bool) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return contracts.Event{}, false
	}

	var r rawLine
	if err := json.Unmarshal(b, &r); err != nil {
		return contracts.Event{}, false
	}
	if r.Type == "" || r.UUID == "" {
		return contracts.Event{}, false
	}

	ts, err := time.Parse(time.RFC3339Nano, r.Timestamp)
	if err != nil {
		return contracts.Event{}, false
	}

	ev := contracts.Event{
		UUID:      r.UUID,
		SessionID: r.SessionID,
		Timestamp: ts.UTC(),
		Type:      r.Type,
		Project:   project,
	}
	if r.Message != nil {
		ev.Model = r.Message.Model
		if r.Message.Usage != nil {
			ev.Usage = &contracts.Usage{
				InputTokens:       r.Message.Usage.InputTokens,
				OutputTokens:      r.Message.Usage.OutputTokens,
				CacheReadTokens:   r.Message.Usage.CacheReadInputToks,
				CacheCreateTokens: r.Message.Usage.CacheCreationInputToks,
			}
		}
	}
	return ev, true
}

// projectNameFromDir returns the human-readable project name for the given
// directory basename. Claude Code stores project directories with URL-encoded
// paths; if decoding fails, the raw name is returned unchanged.
func projectNameFromDir(base string) string {
	decoded, err := url.QueryUnescape(base)
	if err != nil {
		return base
	}
	return decoded
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/scanner
golangci-lint run ./internal/scanner/...
git add internal/scanner/
git commit -m "feat(scanner): add defensive JSONL line parser"
```

---

## Task 3: Per-file streaming reader with cursor (TDD)

**Files:**
- Create: `internal/scanner/file.go`
- Create: `internal/scanner/file_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/scanner/file_test.go`:

```go
package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var buf []byte
	for _, l := range lines {
		buf = append(buf, l...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestReadFileStreamsFromOffsetZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s-1","timestamp":"2026-05-19T12:00:00Z"}`,
		`{"type":"assistant","uuid":"u-2","sessionId":"s-1","timestamp":"2026-05-19T12:00:01Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":3,"cache_read_input_tokens":4}}}`,
	)

	out := make(chan contracts.Event, 4)
	end, _, err := readFile(context.Background(), path, "proj", Cursor{}, out)
	close(out)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if end == 0 {
		t.Errorf("expected non-zero end offset")
	}

	var got []contracts.Event
	for ev := range out {
		got = append(got, ev)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].UUID != "u-1" || got[1].UUID != "u-2" {
		t.Errorf("event order wrong: %+v", got)
	}
}

func TestReadFileSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`,
		`garbage line not json`,
		``,
		`{"type":"future-type","uuid":"u-2","sessionId":"s","timestamp":"2026-05-19T12:00:01Z"}`,
		`{"type":"user","uuid":"u-3","sessionId":"s","timestamp":"2026-05-19T12:00:02Z"}`,
	)

	out := make(chan contracts.Event, 8)
	if _, _, err := readFile(context.Background(), path, "proj", Cursor{}, out); err != nil {
		t.Fatalf("readFile: %v", err)
	}
	close(out)

	var uuids []string
	for ev := range out {
		uuids = append(uuids, ev.UUID)
	}
	want := []string{"u-1", "u-2", "u-3"}
	if len(uuids) != len(want) {
		t.Fatalf("got uuids %v want %v", uuids, want)
	}
	for i := range want {
		if uuids[i] != want[i] {
			t.Fatalf("uuids[%d] = %q want %q", i, uuids[i], want[i])
		}
	}
}

func TestReadFileResumesFromCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`,
		`{"type":"user","uuid":"u-2","sessionId":"s","timestamp":"2026-05-19T12:00:01Z"}`,
	)

	out := make(chan contracts.Event, 4)
	end, inode, err := readFile(context.Background(), path, "proj", Cursor{}, out)
	close(out)
	if err != nil {
		t.Fatalf("readFile 1: %v", err)
	}
	drainCount := 0
	for range out {
		drainCount++
	}
	if drainCount != 2 {
		t.Fatalf("first pass got %d events, want 2", drainCount)
	}

	out2 := make(chan contracts.Event, 4)
	end2, _, err := readFile(context.Background(), path, "proj", Cursor{Offset: end, Inode: inode}, out2)
	close(out2)
	if err != nil {
		t.Fatalf("readFile 2: %v", err)
	}
	if end2 != end {
		t.Errorf("end after no-op resume = %d, want %d", end2, end)
	}
	for ev := range out2 {
		t.Errorf("unexpected event on second pass: %+v", ev)
	}
}

func TestReadFileResetsOnInodeChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`,
	)

	out := make(chan contracts.Event, 4)
	if _, _, err := readFile(context.Background(), path, "proj", Cursor{Offset: 999999, Inode: 1}, out); err != nil {
		t.Fatalf("readFile: %v", err)
	}
	close(out)

	count := 0
	for range out {
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 event after inode-mismatch reset, got %d", count)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: FAIL — `readFile` undefined.

- [ ] **Step 3: Implement the file reader**

Create `internal/scanner/file.go`:

```go
package scanner

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// maxLineBytes is the upper bound for a single JSONL line. Claude Code rarely
// emits anything close to this, but a generous buffer prevents bufio errors
// on outlier sessions with large tool-use payloads.
const maxLineBytes = 16 * 1024 * 1024 // 16 MiB

// readFile streams events from one JSONL file into out. It starts at
// cursor.Offset unless the current inode differs from cursor.Inode, in which
// case it restarts from offset 0. It returns the new end-of-file offset and
// the current inode. Per-line parse failures are logged via slog at debug
// (unknown event types) or warn (malformed bytes) and skipped.
func readFile(ctx context.Context, path, project string, cursor Cursor, out chan<- contracts.Event) (int64, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, 0, fmt.Errorf("stat %q: %w", path, err)
	}
	inode := fileInode(info)
	size := info.Size()

	start := cursor.Offset
	if cursor.Inode != 0 && cursor.Inode != inode {
		slog.Debug("scanner: inode changed, restarting from offset 0", "path", path)
		start = 0
	}
	if start > size {
		slog.Debug("scanner: cursor past EOF, restarting from offset 0", "path", path, "cursor", start, "size", size)
		start = 0
	}

	if _, err := f.Seek(start, 0); err != nil {
		return 0, inode, fmt.Errorf("seek %q to %d: %w", path, start, err)
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	pos := start
	lineNum := 0
	base := filepath.Base(path)

	for {
		select {
		case <-ctx.Done():
			return pos, inode, ctx.Err()
		default:
		}

		line, err := readOneLine(reader, maxLineBytes)
		if len(line) == 0 && err != nil {
			if err.Error() == "EOF" {
				return pos, inode, nil
			}
			slog.Warn("scanner: line read error", "file", base, "line", lineNum+1, "err", err)
			return pos, inode, nil
		}
		lineNum++
		pos += int64(len(line))

		ev, ok := parseLine(line, project)
		if !ok {
			slog.Warn("scanner: skipped malformed line", "file", base, "line", lineNum)
			if err != nil {
				return pos, inode, nil
			}
			continue
		}

		select {
		case <-ctx.Done():
			return pos, inode, ctx.Err()
		case out <- ev:
		}

		if err != nil {
			return pos, inode, nil
		}
	}
}

// readOneLine reads up to and including the next '\n' (or EOF). The returned
// slice includes the trailing newline so the caller can track byte offsets
// accurately. If a line exceeds max bytes, it is truncated and the rest of
// the line is skipped.
func readOneLine(r *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := r.ReadSlice('\n')
		out = append(out, chunk...)
		if err == nil {
			return out, nil
		}
		if err == bufio.ErrBufferFull && len(out) < max {
			continue
		}
		return out, err
	}
}
```

Create `internal/scanner/inode_unix.go` (covers darwin, linux, freebsd, etc.):

```go
//go:build !windows

package scanner

import (
	"os"
	"syscall"
)

func fileInode(info os.FileInfo) uint64 {
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(sys.Ino)
	}
	return 0
}
```

Create `internal/scanner/inode_windows.go`:

```go
//go:build windows

package scanner

import "os"

// fileInode returns a stable identifier for the file. Windows does not expose
// a true inode through os.FileInfo, so we approximate using size + modtime —
// any modification rotates the value.
func fileInode(info os.FileInfo) uint64 {
	return uint64(info.Size()) ^ uint64(info.ModTime().UnixNano())
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/scanner
golangci-lint run ./internal/scanner/...
git add internal/scanner/
git commit -m "feat(scanner): add per-file streaming reader with cursor"
```

---

## Task 4: Sample JSONL fixtures

**Files:**
- Create: `internal/scanner/testdata/fixtures/sample-session.jsonl`
- Create: `internal/scanner/testdata/fixtures/malformed-session.jsonl`
- Create: `internal/scanner/testdata/fixtures/empty-session.jsonl`

- [ ] **Step 1: Write `sample-session.jsonl`**

Create `internal/scanner/testdata/fixtures/sample-session.jsonl` with exactly these 5 lines (each line is a complete JSON object terminated by `\n`):

```
{"type":"user","uuid":"01H7Z8AAAA","sessionId":"sess-001","timestamp":"2026-05-19T12:00:00Z","cwd":"/home/u/proj","message":{"content":[{"type":"text","text":"hi"}]}}
{"type":"assistant","uuid":"01H7Z8AAAB","sessionId":"sess-001","timestamp":"2026-05-19T12:00:01Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":200}}}
{"type":"user","uuid":"01H7Z8AAAC","sessionId":"sess-001","timestamp":"2026-05-19T12:01:00Z","message":{"content":[{"type":"text","text":"more"}]}}
{"type":"assistant","uuid":"01H7Z8AAAD","sessionId":"sess-001","timestamp":"2026-05-19T12:01:02Z","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":200,"output_tokens":80,"cache_creation_input_tokens":0,"cache_read_input_tokens":500}}}
{"type":"tool_result","uuid":"01H7Z8AAAE","sessionId":"sess-001","timestamp":"2026-05-19T12:01:03Z"}
```

- [ ] **Step 2: Write `malformed-session.jsonl`**

Create `internal/scanner/testdata/fixtures/malformed-session.jsonl`:

```
{"type":"user","uuid":"01H7Z8BBBA","sessionId":"sess-002","timestamp":"2026-05-19T12:00:00Z"}
this line is not valid json at all
{"type":"unknown-future-event","uuid":"01H7Z8BBBB","sessionId":"sess-002","timestamp":"2026-05-19T12:00:01Z"}

{"type":"assistant","uuid":"01H7Z8BBBC","sessionId":"sess-002","timestamp":"2026-05-19T12:00:02Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"user","uuid":"","sessionId":"sess-002","timestamp":"2026-05-19T12:00:03Z"}
{"type":"user","uuid":"01H7Z8BBBD","sessionId":"sess-002","timestamp":"not-a-timestamp"}
{"type":"user","uuid":"01H7Z8BBBE","sessionId":"sess-002","timestamp":"2026-05-19T12:00:05Z"}
```

- [ ] **Step 3: Write `empty-session.jsonl`**

Create `internal/scanner/testdata/fixtures/empty-session.jsonl` containing exactly one byte: a single newline `\n` (the file is intentionally near-empty to exercise zero-event handling).

- [ ] **Step 4: Verify fixtures are readable**

```bash
wc -l internal/scanner/testdata/fixtures/*.jsonl
```

Expected: `sample-session.jsonl` has 5 lines, `malformed-session.jsonl` has 7 lines (including the empty middle line), `empty-session.jsonl` has 1 line.

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/testdata/
git commit -m "test(scanner): add sample JSONL fixtures"
```

---

## Task 5: Golden test against `sample-session.jsonl` (TDD)

**Files:**
- Create: `internal/scanner/golden_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/scanner/golden_test.go`:

```go
package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/scanner"
)

// stageProfile copies fixture files into a temporary <ConfigDir>/projects/<proj>/ tree
// and returns the staged profile.
func stageProfile(t *testing.T, fixtureToFilename map[string]string) contracts.Profile {
	t.Helper()
	dir := t.TempDir()
	for fixture, rel := range fixtureToFilename {
		src := filepath.Join("testdata", "fixtures", fixture)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %q: %v", fixture, err)
		}
		dst := filepath.Join(dir, "projects", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	return contracts.Profile{Name: "test", ConfigDir: dir}
}

func TestScannerGoldenSampleSession(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"sample-session.jsonl": filepath.Join("home%2Fu%2Fproj", "sess-001.jsonl"),
	})

	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	events, errs := s.Scan(context.Background(), profile)

	var got []contracts.Event
	for ev := range events {
		got = append(got, ev)
	}
	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}

	want := []contracts.Event{
		{
			UUID: "01H7Z8AAAA", SessionID: "sess-001", Type: "user", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		},
		{
			UUID: "01H7Z8AAAB", SessionID: "sess-001", Type: "assistant", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 0, 1, 0, time.UTC),
			Model:     "claude-opus-4-7",
			Usage:     &contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 10},
		},
		{
			UUID: "01H7Z8AAAC", SessionID: "sess-001", Type: "user", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 1, 0, 0, time.UTC),
		},
		{
			UUID: "01H7Z8AAAD", SessionID: "sess-001", Type: "assistant", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 1, 2, 0, time.UTC),
			Model:     "claude-sonnet-4-6",
			Usage:     &contracts.Usage{InputTokens: 200, OutputTokens: 80, CacheReadTokens: 500, CacheCreateTokens: 0},
		},
		{
			UUID: "01H7Z8AAAE", SessionID: "sess-001", Type: "tool_result", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 1, 3, 0, time.UTC),
		},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if !eventEqual(got[i], want[i]) {
			t.Errorf("event[%d] mismatch:\n got  %+v usage=%v\n want %+v usage=%v",
				i, got[i], usageDeref(got[i].Usage), want[i], usageDeref(want[i].Usage))
		}
	}
}

func TestScannerSkipsMalformedAndUnknown(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"malformed-session.jsonl": filepath.Join("proj", "sess-002.jsonl"),
	})

	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	events, errs := s.Scan(context.Background(), profile)

	var uuids []string
	for ev := range events {
		uuids = append(uuids, ev.UUID)
	}
	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}

	want := []string{"01H7Z8BBBA", "01H7Z8BBBB", "01H7Z8BBBC", "01H7Z8BBBE"}
	if len(uuids) != len(want) {
		t.Fatalf("got uuids %v, want %v", uuids, want)
	}
	for i := range want {
		if uuids[i] != want[i] {
			t.Errorf("uuid[%d] = %q want %q", i, uuids[i], want[i])
		}
	}
}

func eventEqual(a, b contracts.Event) bool {
	if a.UUID != b.UUID || a.SessionID != b.SessionID || a.Type != b.Type ||
		a.Project != b.Project || a.Model != b.Model || !a.Timestamp.Equal(b.Timestamp) {
		return false
	}
	if (a.Usage == nil) != (b.Usage == nil) {
		return false
	}
	if a.Usage != nil && *a.Usage != *b.Usage {
		return false
	}
	return true
}

func usageDeref(u *contracts.Usage) contracts.Usage {
	if u == nil {
		return contracts.Usage{}
	}
	return *u
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: FAIL — the Scanner's `Scan` method currently does not stream events from files (Task 1 left the worker loop as a placeholder). The golden test forces the wiring.

- [ ] **Step 3: Wire the worker pool in `scanner.go`**

Replace the body of `Scan` and add the worker-loop logic in `internal/scanner/scanner.go`. Replace the file from the `func (s *Scanner) Scan(...)` line through the end of the existing `func (s *Scanner) listJSONL(...)` definition with:

```go
// Scan walks <profile.ConfigDir>/projects/*/<session-uuid>.jsonl and emits
// parsed Events on the returned channel. The events channel is closed when
// scanning completes or ctx is cancelled. The errs channel reports fatal
// errors (e.g., directory traversal failures); it is also closed when done.
// Per-line parse failures are logged and skipped, not reported on errs.
func (s *Scanner) Scan(ctx context.Context, profile contracts.Profile) (<-chan contracts.Event, <-chan error) {
	events := make(chan contracts.Event, 256)
	errs := make(chan error, 1)

	go s.run(ctx, profile, events, errs)

	return events, errs
}

func (s *Scanner) run(ctx context.Context, profile contracts.Profile, events chan<- contracts.Event, errs chan<- error) {
	defer close(events)
	defer close(errs)

	files, err := s.listJSONL(profile.ConfigDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			errs <- err
		}
		return
	}
	if len(files) == 0 {
		return
	}

	jobs := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				s.processFile(ctx, profile.Name, path, events)
			}
		}()
	}

	for _, p := range files {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			errs <- ctx.Err()
			return
		case jobs <- p:
		}
	}
	close(jobs)
	wg.Wait()
}

func (s *Scanner) processFile(ctx context.Context, profileName, path string, out chan<- contracts.Event) {
	cur, err := s.cursors.Get(ctx, profileName, path)
	if err != nil {
		s.logger.Warn("scanner: cursor get failed", "path", path, "err", err)
		return
	}

	project := projectNameFromDir(filepath.Base(filepath.Dir(path)))
	end, inode, err := readFile(ctx, path, project, cur, out)
	if err != nil {
		s.logger.Warn("scanner: read failed", "path", path, "err", err)
		return
	}

	if err := s.cursors.Set(ctx, profileName, path, Cursor{Offset: end, Inode: inode}); err != nil {
		s.logger.Warn("scanner: cursor set failed", "path", path, "err", err)
	}
}

// listJSONL returns every <configDir>/projects/<project>/<session>.jsonl file.
// Missing configDir or projects dir returns fs.ErrNotExist.
func (s *Scanner) listJSONL(configDir string) ([]string, error) {
	projectsDir := filepath.Join(configDir, "projects")
	info, err := os.Stat(projectsDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fs.ErrNotExist
	}

	var out []string
	err = filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".jsonl" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
```

Also update the imports block at the top of `scanner.go` to include `sync`:

```go
import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/arafa-dev/ccx/internal/contracts"
)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: PASS on all tests (golden, malformed, parse, file, cursor).

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/scanner
golangci-lint run ./internal/scanner/...
git add internal/scanner/
git commit -m "feat(scanner): wire worker pool and add golden + skip tests"
```

---

## Task 6: Incremental and inode-change end-to-end tests (TDD)

**Files:**
- Modify: `internal/scanner/golden_test.go` (append two tests)

- [ ] **Step 1: Append the failing tests**

Append to `internal/scanner/golden_test.go`:

```go
func TestScannerIncrementalScanReturnsNothingNewOnSecondPass(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"sample-session.jsonl": filepath.Join("home%2Fu%2Fproj", "sess-001.jsonl"),
	})

	cs := scanner.NewMemoryCursorStore()
	s := scanner.NewScanner(cs)

	events1, errs1 := s.Scan(context.Background(), profile)
	first := 0
	for range events1 {
		first++
	}
	for err := range errs1 {
		t.Fatalf("first pass error: %v", err)
	}
	if first != 5 {
		t.Fatalf("first pass got %d events, want 5", first)
	}

	events2, errs2 := s.Scan(context.Background(), profile)
	second := 0
	for ev := range events2 {
		second++
		t.Errorf("unexpected event on second pass: %+v", ev)
	}
	for err := range errs2 {
		t.Fatalf("second pass error: %v", err)
	}
	if second != 0 {
		t.Fatalf("second pass got %d events, want 0", second)
	}
}

func TestScannerReScansWhenInodeChanges(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"sample-session.jsonl": filepath.Join("home%2Fu%2Fproj", "sess-001.jsonl"),
	})

	cs := scanner.NewMemoryCursorStore()
	// Pre-poison the cursor with a non-zero inode that won't match the real file.
	jsonlPath := filepath.Join(profile.ConfigDir, "projects", "home%2Fu%2Fproj", "sess-001.jsonl")
	if err := cs.Set(context.Background(), profile.Name, jsonlPath, scanner.Cursor{Offset: 999999, Inode: 1}); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}

	s := scanner.NewScanner(cs)
	events, errs := s.Scan(context.Background(), profile)

	count := 0
	for range events {
		count++
	}
	for err := range errs {
		t.Fatalf("scan error: %v", err)
	}
	if count != 5 {
		t.Errorf("inode-change rescan got %d events, want 5", count)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: PASS (the Task 5 wiring already supports both behaviors via `readFile`).

If either test fails, that is a defect in earlier tasks — fix the implementation, not the test.

- [ ] **Step 3: Commit**

```bash
gofumpt -w internal/scanner
golangci-lint run ./internal/scanner/...
git add internal/scanner/golden_test.go
git commit -m "test(scanner): cover incremental and inode-change paths"
```

---

## Task 7: Concurrent scan timing test (TDD)

**Files:**
- Create: `internal/scanner/concurrency_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/scanner/concurrency_test.go`:

```go
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

	src, err := os.ReadFile(filepath.Join("testdata", "fixtures", "sample-session.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	for i := 0; i < 10; i++ {
		path := filepath.Join(dir, "projects", fmt.Sprintf("proj-%d", i), "sess-001.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, src, 0o644); err != nil {
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
		t.Errorf("got %d events, want 50 (10 files × 5)", count)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("scan of 10 files took %v, want <100ms", elapsed)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test -race -count=1 ./internal/scanner/...
```

Expected: PASS. If timing fails on a slow CI runner, this test is the canary — investigate, do not weaken the bound.

- [ ] **Step 3: Commit**

```bash
gofumpt -w internal/scanner
golangci-lint run ./internal/scanner/...
git add internal/scanner/concurrency_test.go
git commit -m "test(scanner): verify concurrent scan of 10 files under 100ms"
```

---

## Task 8: Fuzz test for the line parser

**Files:**
- Create: `internal/scanner/fuzz_test.go`

- [ ] **Step 1: Write the fuzz test**

Create `internal/scanner/fuzz_test.go`:

```go
package scanner

import "testing"

func FuzzParseLine(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`),
		[]byte(`{"type":"assistant","uuid":"u","sessionId":"s","timestamp":"2026-05-19T12:00:01Z","message":{"model":"m","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`),
		[]byte(``),
		[]byte(`{`),
		[]byte(`{"type":"x"}`),
		[]byte(`{"type":"user","uuid":"","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`),
		[]byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"bad"}`),
		[]byte("\x00\x01\x02"),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic on arbitrary bytes.
		_, _ = parseLine(data, "fuzz-project")
	})
}
```

- [ ] **Step 2: Run the fuzz test for the corpus**

```bash
go test -run=FuzzParseLine ./internal/scanner
```

Expected: PASS (this runs only the seed corpus, not the fuzzing loop).

- [ ] **Step 3: Run an actual fuzz burst**

```bash
go test -run='^$' -fuzz=FuzzParseLine -fuzztime=30s ./internal/scanner
```

Expected: completes with `fuzz: elapsed: 30s, completed N, ...` and no crashers reported. If a crasher is found, the seed is saved under `testdata/fuzz/FuzzParseLine/` — open it, identify the panic in `parseLine`, fix, re-run, and commit the seed alongside the fix.

- [ ] **Step 4: Commit**

```bash
gofumpt -w internal/scanner
golangci-lint run ./internal/scanner/...
git add internal/scanner/fuzz_test.go
git commit -m "test(scanner): add FuzzParseLine"
```

---

## Task 9: Final verification and PR

- [ ] **Step 1: Confirm package dependency surface**

```bash
go list -deps ./internal/scanner | grep -v '^github.com/arafa-dev/ccx' | grep -v '^golang.org/x/' | sort -u
```

Expected output contains only stdlib packages (paths without a domain dot in the first component, e.g., `bufio`, `bytes`, `context`, `encoding/json`, `errors`, `fmt`, `io`, `io/fs`, `log/slog`, `net/url`, `os`, `path/filepath`, `runtime`, `sync`, `syscall`, `time`). The single internal import is `github.com/arafa-dev/ccx/internal/contracts`.

If any third-party module appears, remove the offending import.

- [ ] **Step 2: Run the full local gate**

```bash
gofumpt -l internal/scanner
go vet ./internal/scanner/...
golangci-lint run ./internal/scanner/...
go test -race -count=1 ./internal/scanner/...
go test -run='^$' -fuzz=FuzzParseLine -fuzztime=30s ./internal/scanner
```

Expected: every command exits 0 and `gofumpt -l` prints nothing.

- [ ] **Step 3: Push the branch and open a PR**

```bash
git push -u origin feat/scanner
gh pr create --title "feat(scanner): JSONL parser, dir walker, fuzz tests (A2)" --body "$(cat <<'EOF'
## What

Implements `internal/scanner/` per plan A2:

- `Scanner` type implementing `contracts.Scanner`
- `CursorStore` interface + in-memory implementation
- Defensive line parser (rejects malformed JSON, missing UUID, bad timestamps; ignores unknown fields)
- Per-file streaming reader with byte-offset + inode cursor (inode mismatch forces re-scan from 0)
- Bounded worker pool sized to `runtime.NumCPU()`
- Sample JSONL fixtures under `testdata/fixtures/`
- Golden, defensive-skip, incremental, inode-change, and 10-file concurrency tests
- `FuzzParseLine` — verified for 30s with no crashers

## Why

Phase 1 plan A2. Unlocks Phase 2 wiring of usage queries.

## Contract impact

- [x] This PR does NOT modify `internal/contracts/`, `api/openapi.yaml`, `internal/storage/schema.sql`, or `docs/conventions.md`
- [ ] If it does, this is a contract-amendment PR

## Checklist

- [x] Tests added and pass locally (`go test -race -count=1 ./internal/scanner/...`)
- [x] Lint clean (`golangci-lint run ./internal/scanner/...`)
- [x] No new dependencies (stdlib + `internal/contracts` only)

## Phase 1 worktree

- Package: `internal/scanner`
- Plan: `docs/superpowers/plans/2026-05-19-ccx-phase-1-A2-scanner.md`
EOF
)"
```

- [ ] **Step 4: Watch CI and merge**

```bash
gh pr checks --watch
```

Expected: lint + test (×3 OSes) + build (×3 OSes) all green. After review, merge with a linear-history merge.

---

## Phase 1 A2 done definition

All of the following are true:

- [ ] `go build ./internal/scanner/...` succeeds
- [ ] `go test -race -count=1 ./internal/scanner/...` is green
- [ ] `go test -run='^$' -fuzz=FuzzParseLine -fuzztime=30s ./internal/scanner` exits 0 with no crashers
- [ ] `golangci-lint run ./internal/scanner/...` reports zero issues
- [ ] `gofumpt -l internal/scanner` prints nothing
- [ ] `go list -deps ./internal/scanner` shows only stdlib + `internal/contracts`
- [ ] All files from this plan exist and are committed:
  - `internal/scanner/doc.go` (extended)
  - `internal/scanner/scanner.go`
  - `internal/scanner/cursor.go`
  - `internal/scanner/parse.go`
  - `internal/scanner/file.go`
  - `internal/scanner/inode_unix.go`
  - `internal/scanner/inode_windows.go`
  - `internal/scanner/scanner_test.go`
  - `internal/scanner/parse_test.go`
  - `internal/scanner/file_test.go`
  - `internal/scanner/golden_test.go`
  - `internal/scanner/concurrency_test.go`
  - `internal/scanner/fuzz_test.go`
  - `internal/scanner/testdata/fixtures/sample-session.jsonl`
  - `internal/scanner/testdata/fixtures/malformed-session.jsonl`
  - `internal/scanner/testdata/fixtures/empty-session.jsonl`
- [ ] PR merged to `main` with green CI
