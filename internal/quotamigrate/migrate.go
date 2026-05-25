package quotamigrate

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Action is what a Step does.
type Action int

const (
	// ActionMoveContents copies a profile's projects subtree into the shared
	// directory without overwriting existing destination files, then removes
	// the source tree.
	ActionMoveContents Action = iota + 1
	// ActionCreateSymlink replaces or creates the profile's projects entry
	// with a symlink to the shared directory.
	ActionCreateSymlink
)

// Step is one filesystem operation in a migration plan.
type Step struct {
	Profile string
	Action  Action
	From    string
	To      string
}

// String renders a step in a human-readable form for dry-run output.
func (s Step) String() string {
	switch s.Action {
	case ActionMoveContents:
		return fmt.Sprintf("[%s] move contents of %s -> %s", s.Profile, s.From, s.To)
	case ActionCreateSymlink:
		return fmt.Sprintf("[%s] symlink %s -> %s", s.Profile, s.From, s.To)
	default:
		return fmt.Sprintf("[%s] unknown action", s.Profile)
	}
}

// Plan inspects the disk state for each profile and returns the migration
// steps needed to bring it into the shared-projects layout. It returns an empty
// slice when everything is already migrated.
func Plan(ccxHome string, profiles []contracts.Profile) ([]Step, error) {
	shared := SharedProjectsPath(ccxHome)
	steps := make([]Step, 0, len(profiles))

	for i := range profiles {
		profile := &profiles[i]
		projects := ProfileProjectsPath(profile.ConfigDir)
		info, err := os.Lstat(projects)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("lstat %q: %w", projects, err)
		}

		if info != nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(projects)
			if err != nil {
				return nil, fmt.Errorf("readlink %q: %w", projects, err)
			}
			if target == shared {
				continue
			}
			return nil, fmt.Errorf("[%s] %q is a symlink to unexpected target %q; refusing to overwrite", profile.Name, projects, target)
		}

		if info != nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("[%s] %q exists and is not a directory; refusing to overwrite", profile.Name, projects)
			}
			steps = append(steps, Step{
				Profile: profile.Name,
				Action:  ActionMoveContents,
				From:    projects,
				To:      shared,
			})
		}

		steps = append(steps, Step{
			Profile: profile.Name,
			Action:  ActionCreateSymlink,
			From:    projects,
			To:      shared,
		})
	}

	return steps, nil
}

// Apply executes the plan. It is safe to re-run idempotent steps. It stops at
// the first error and returns it; partial state may be left on disk.
func Apply(steps []Step) error {
	if err := preflightSymlinkSteps(steps); err != nil {
		return err
	}
	for _, step := range steps {
		switch step.Action {
		case ActionMoveContents:
			if err := applyMoveContents(step.From, step.To); err != nil {
				return fmt.Errorf("apply move %q -> %q: %w", step.From, step.To, err)
			}
		case ActionCreateSymlink:
			if err := applyCreateSymlink(step.From, step.To); err != nil {
				return fmt.Errorf("apply symlink %q -> %q: %w", step.From, step.To, err)
			}
		default:
			return fmt.Errorf("apply: unknown action %v", step.Action)
		}
	}

	return nil
}

func preflightSymlinkSteps(steps []Step) error {
	for i, step := range steps {
		if step.Action != ActionCreateSymlink {
			continue
		}
		link, err := preflightSymlinkPath(filepath.Dir(step.From), i)
		if err != nil {
			return err
		}
		if err := applyCreateSymlink(link, step.To); err != nil {
			return fmt.Errorf("preflight symlink %q -> %q: %w", link, step.To, err)
		}
		if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove preflight symlink %q: %w", link, err)
		}
	}

	return nil
}

func preflightSymlinkPath(parent string, stepIndex int) (string, error) {
	for attempt := range 10 {
		path := filepath.Join(parent, fmt.Sprintf(".ccx-symlink-preflight-%d-%d-%d", os.Getpid(), stepIndex, attempt))
		_, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return path, nil
		case err != nil:
			return "", fmt.Errorf("lstat preflight symlink path %q: %w", path, err)
		}
	}

	return "", fmt.Errorf("could not choose unused preflight symlink path in %q", parent)
}

func applyMoveContents(src, dst string) error {
	info, err := os.Lstat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lstat source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("readlink source: %w", err)
		}
		if target == dst {
			return nil
		}
		return fmt.Errorf("source is a symlink to unexpected target %q", target)
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory")
	}
	entries, err := planMergeEntries(src, dst)
	if err != nil {
		return err
	}
	if err := copyMergeEntries(entries); err != nil {
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("remove source tree: %w", err)
	}

	return nil
}

func applyCreateSymlink(from, to string) error {
	info, err := os.Lstat(from)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return fmt.Errorf("readlink existing symlink: %w", err)
		}
		if target == to {
			return nil
		}
		return fmt.Errorf("existing symlink points to unexpected target %q", target)
	case err == nil:
		if err := os.Remove(from); err != nil {
			return fmt.Errorf("remove existing path: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("lstat existing path: %w", err)
	}

	if err := mkdirAllNoSymlink(to, 0o700); err != nil {
		return fmt.Errorf("mkdir shared: %w", err)
	}
	if err := mkdirAllNoSymlink(filepath.Dir(from), 0o700); err != nil {
		return fmt.Errorf("mkdir profile config dir: %w", err)
	}
	if err := createProjectsLinkFunc(to, from); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}

	return nil
}

type mergeEntry struct {
	source string
	target string
	mode   fs.FileMode
	dir    bool
}

func planMergeEntries(src, dst string) ([]mergeEntry, error) {
	if err := ensureNoSymlinkInPath(dst, dst); err != nil {
		return nil, err
	}

	entries := make([]mergeEntry, 0)
	if err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == ".." || relHasDotDotPrefix(rel) {
			return fmt.Errorf("refusing path outside source tree %q", path)
		}
		target := filepath.Join(dst, rel)

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to migrate symlink %q", path)
		}
		if entry.IsDir() {
			if err := ensureNoSymlinkInPath(dst, target); err != nil {
				return err
			}
			if existing, err := os.Lstat(target); err == nil {
				if existing.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("refusing to traverse destination symlink %q", target)
				}
				if !existing.IsDir() {
					return fmt.Errorf("refusing to overwrite non-directory %q", target)
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			entries = append(entries, mergeEntry{source: path, target: target, mode: info.Mode().Perm(), dir: true})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to migrate non-regular file %q", path)
		}
		if err := ensureNoSymlinkInPath(dst, filepath.Dir(target)); err != nil {
			return err
		}
		if existing, err := os.Lstat(target); err == nil {
			if existing.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to overwrite destination symlink %q", target)
			}
			if existing.IsDir() {
				return fmt.Errorf("refusing to overwrite directory %q", target)
			}
			same, err := sameRegularFile(path, target, info, existing)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("refusing to overwrite existing file %q", target)
			}
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		entries = append(entries, mergeEntry{source: path, target: target, mode: info.Mode().Perm()})
		return nil
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func copyMergeEntries(entries []mergeEntry) error {
	for _, entry := range entries {
		if entry.dir {
			if err := mkdirAllNoSymlink(entry.target, entry.mode); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Lstat(entry.target); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := copyFile(entry.source, entry.target, entry.mode); err != nil {
			return err
		}
	}
	return nil
}

func mkdirAllNoSymlink(path string, mode fs.FileMode) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to traverse symlink %q", path)
		}
		if !info.IsDir() {
			return fmt.Errorf("path %q is not a directory", path)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := mkdirAllNoSymlink(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.Mkdir(path, mode)
}

func ensureNoSymlinkInPath(root, path string) error {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == ".." || relHasDotDotPrefix(rel) {
		return fmt.Errorf("refusing destination outside shared tree %q", path)
	}
	current := root
	if err := rejectExistingSymlink(current); err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	for _, part := range splitPath(rel) {
		current = filepath.Join(current, part)
		if err := rejectExistingSymlink(current); err != nil {
			return err
		}
	}
	return nil
}

func rejectExistingSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to traverse symlink %q", path)
	}
	return nil
}

func relHasDotDotPrefix(rel string) bool {
	return len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)
}

func splitPath(path string) []string {
	clean := filepath.Clean(path)
	if clean == "." {
		return nil
	}
	return strings.Split(clean, string(filepath.Separator))
}

func sameRegularFile(left, right string, leftInfo, rightInfo fs.FileInfo) (bool, error) {
	if !rightInfo.Mode().IsRegular() {
		return false, nil
	}
	if leftInfo.Size() != rightInfo.Size() || leftInfo.Mode().Perm() != rightInfo.Mode().Perm() {
		return false, nil
	}
	leftHash, err := fileHash(left)
	if err != nil {
		return false, err
	}
	rightHash, err := fileHash(right)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func fileHash(path string) ([sha256.Size]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- migration compares files from planned source/destination paths.
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [sha256.Size]byte{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	dstDir := filepath.Dir(dst)
	if err := mkdirAllNoSymlink(dstDir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to copy symlink %q", src)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to copy non-regular file %q", src)
	}
	in, err := os.Open(src) // #nosec G304 -- migration copies files from user-selected profile dirs.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp, err := os.CreateTemp(dstDir, ".ccx-migrate-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	linked := false
	defer func() {
		if !linked {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpPath, dst); err != nil {
		return err
	}
	if err := os.Remove(tmpPath); err != nil {
		return err
	}
	linked = true
	return nil
}
