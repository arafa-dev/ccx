// Package scanner walks a profile's JSONL session files and emits parsed
// contracts.Events. It is defensive: unknown event types and malformed lines
// are logged and skipped, never panicked on. File-level concurrency is bounded
// by a worker pool sized to runtime.NumCPU(). Incremental scanning is driven
// by an injectable CursorStore so unit tests can use an in-memory map while
// Phase 2 backs it with internal/storage.
package scanner
