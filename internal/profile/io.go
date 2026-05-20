package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// tmpSuffix is the suffix used by atomic writes. The temp file is renamed
// over fileName on successful flush.
const tmpSuffix = ".tmp"

// loadRegistry reads and parses the registry file at path. A missing file is
// treated as an empty registry (not an error) so that the first run of ccx
// works without `ccx profile add` having been called yet.
func loadRegistry(path string) (registry, error) {
	// The path is the manager-owned registry location, not user shell input.
	//nolint:gosec // G304: registry path is controlled by Manager construction.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return registry{}, nil
		}
		return registry{}, fmt.Errorf("reading registry %q: %w", path, err)
	}
	r, err := decodeRegistry(data)
	if err != nil {
		return registry{}, fmt.Errorf("parsing registry %q: %w", path, err)
	}
	return r, nil
}

// atomicWriteRegistry serializes r to TOML and writes it to path via a
// rename-from-tmp dance. The parent directory is created with 0700 if it
// does not exist. The final file mode is 0600.
//
// On error the .tmp file is removed if possible; the original path is left
// untouched.
func atomicWriteRegistry(path string, r registry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating registry dir: %w", err)
	}

	data, err := encodeRegistry(r)
	if err != nil {
		return fmt.Errorf("encoding registry: %w", err)
	}

	tmp := path + tmpSuffix
	// The path is the manager-owned registry temp file, not user shell input.
	//nolint:gosec // G304: registry path is controlled by Manager construction.
	tmpFile, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("opening tmp registry: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing tmp registry: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("syncing tmp registry: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("closing tmp registry: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming tmp registry: %w", err)
	}
	if err := syncParentDir(path); err != nil {
		return fmt.Errorf("syncing registry dir: %w", err)
	}
	return nil
}

func syncParentDir(path string) error {
	// The path is the manager-owned registry directory, not user shell input.
	//nolint:gosec // G304: registry path is controlled by Manager construction.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}

	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil && !isIgnorableDirSyncError(syncErr) {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func isIgnorableDirSyncError(err error) bool {
	return errors.Is(err, syscall.EINVAL) || runtime.GOOS == "windows"
}
