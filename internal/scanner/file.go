package scanner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
// the current inode. Per-line parse failures are logged via slog and skipped.
func readFile(ctx context.Context, path, project string, cursor Cursor, out chan<- contracts.Event) (end int64, inode uint64, err error) {
	return readFileWithEmit(ctx, path, project, cursor, func(ev contracts.Event) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- ev:
			return nil
		}
	})
}

func readFileWithEmit(ctx context.Context, path, project string, cursor Cursor, emit func(contracts.Event) error) (end int64, inode uint64, err error) {
	f, err := os.Open(path) // #nosec G304 -- scanner intentionally reads profile JSONL paths.
	if err != nil {
		return 0, 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	info, err := f.Stat()
	if err != nil {
		return 0, 0, fmt.Errorf("stat %q: %w", path, err)
	}
	inode = fileInode(info)
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

	if _, err := f.Seek(start, io.SeekStart); err != nil {
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
			if errors.Is(err, io.EOF) {
				return pos, inode, nil
			}
			slog.Warn("scanner: line read error", "file", base, "line", lineNum+1, "err", err)
			return pos, inode, nil
		}
		lineNum++
		pos += int64(len(line))

		ev, outcome := parseLine(line, project)
		switch outcome {
		case parseEvent:
			if emitErr := emit(ev); emitErr != nil {
				return pos, inode, emitErr
			}
		case parseMalformed:
			slog.Warn("scanner: skipped malformed line", "file", base, "line", lineNum)
		case parseIgnore:
			// Valid JSON, but not a usage event ccx tracks. Skip quietly;
			// surfaced only under --verbose.
			slog.Debug("scanner: skipped non-event line", "file", base, "line", lineNum)
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
func readOneLine(r *bufio.Reader, maxBytes int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := r.ReadSlice('\n')
		out = append(out, chunk...)
		if err == nil {
			return out, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) && len(out) < maxBytes {
			continue
		}
		return out, err
	}
}
