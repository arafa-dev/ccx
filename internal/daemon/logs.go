package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// WriteLogs writes daemon.log to w and optionally follows new bytes until ctx
// is cancelled.
func (c *Controller) WriteLogs(ctx context.Context, w io.Writer, follow bool) error {
	root, err := c.root()
	if err != nil {
		return err
	}
	path := RuntimePaths(root).LogPath
	file, err := os.Open(path) //nolint:gosec // path is controlled by ccx home.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(w, file); err != nil {
		return err
	}
	if !follow {
		return nil
	}
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			info, err := file.Stat()
			if err != nil {
				return err
			}
			if info.Size() < offset {
				offset = 0
			}
			if info.Size() == offset {
				continue
			}
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				return err
			}
			n, err := io.Copy(w, file)
			offset += n
			if err != nil {
				return err
			}
		}
	}
}
