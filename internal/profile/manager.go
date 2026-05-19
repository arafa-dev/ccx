package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// fileName is the canonical name of the registry file inside the ccx root.
const fileName = "profiles.toml"

// Manager owns the profile registry at <root>/profiles.toml. All mutating
// methods rewrite the whole file atomically. The zero Manager is not usable;
// always construct via NewManager.
type Manager struct {
	root string
	mu   sync.Mutex
}

// NewManager returns a Manager rooted at the given directory (typically
// ~/.ccx). The directory does not need to exist yet; it is created lazily by
// the first mutating call.
func NewManager(root string) (*Manager, error) {
	if root == "" {
		return nil, errors.New("profile: root path is empty")
	}
	return &Manager{root: root}, nil
}

// Root returns the registry root directory.
func (m *Manager) Root() string {
	return m.root
}

// Path returns the absolute path to the registry file.
func (m *Manager) Path() string {
	return filepath.Join(m.root, fileName)
}

// Add registers a new profile. Behavior:
//   - Validates the profile shape (name, absolute ConfigDir).
//   - Rejects with contracts.ErrProfileAlreadyExists if another profile has
//     the same name.
//   - Ensures ConfigDir exists (creating it with mode 0700 if missing).
//   - Sets CreatedAt/LastUsedAt to time.Now().UTC() when the caller leaves
//     them zero, so callers can pass a bare Profile{Name, ConfigDir}.
//   - Writes the full registry atomically.
func (m *Manager) Add(ctx context.Context, p contracts.Profile) error { //nolint:gocritic // Profile is a value-style contract type.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateProfile(p); err != nil {
		return err
	}

	if err := ensureConfigDir(p.ConfigDir); err != nil {
		return fmt.Errorf("preparing config dir %q: %w", p.ConfigDir, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return err
	}

	for _, existing := range reg.Profiles {
		if existing.Name == p.Name {
			return fmt.Errorf("profile %q: %w", p.Name, contracts.ErrProfileAlreadyExists)
		}
		if existing.ConfigDir == p.ConfigDir {
			return fmt.Errorf("config_dir %q already used by profile %q: %w", p.ConfigDir, existing.Name, contracts.ErrConfigDirConflict)
		}
	}

	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.LastUsedAt.IsZero() {
		p.LastUsedAt = now
	}

	reg.Profiles = append(reg.Profiles, p)
	if err := atomicWriteRegistry(m.Path(), reg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}
	return nil
}

// ensureConfigDir guarantees that path exists and is a directory. If path
// does not exist it is created with mode 0700. If path exists but is not a
// directory the call returns contracts.ErrInvalidConfigDir.
func ensureConfigDir(path string) error {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("path %q is not a directory: %w", path, contracts.ErrInvalidConfigDir)
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		if mkErr := os.MkdirAll(path, 0o700); mkErr != nil {
			return fmt.Errorf("creating %q: %w", path, contracts.ErrInvalidConfigDir)
		}
		return nil
	default:
		return fmt.Errorf("stat %q: %w", path, err)
	}
}

// Get returns the profile with the given name. If no such profile exists,
// the returned error wraps contracts.ErrProfileNotFound.
func (m *Manager) Get(ctx context.Context, name string) (contracts.Profile, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Profile{}, err
	}
	if name == "" {
		return contracts.Profile{}, fmt.Errorf("profile: name is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return contracts.Profile{}, err
	}
	for _, p := range reg.Profiles {
		if p.Name == name {
			return p, nil
		}
	}
	return contracts.Profile{}, fmt.Errorf("profile %q: %w", name, contracts.ErrProfileNotFound)
}

// List returns all profiles, sorted by Name. The returned slice is a fresh
// copy; mutating it does not affect the on-disk registry.
func (m *Manager) List(ctx context.Context) ([]contracts.Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return nil, err
	}
	out := make([]contracts.Profile, len(reg.Profiles))
	copy(out, reg.Profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Remove deletes the profile with the given name. If no such profile exists,
// the returned error wraps contracts.ErrProfileNotFound. The file is rewritten
// atomically only if the registry actually changed.
func (m *Manager) Remove(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("profile: name is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return err
	}

	idx := -1
	for i, p := range reg.Profiles {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile %q: %w", name, contracts.ErrProfileNotFound)
	}

	reg.Profiles = append(reg.Profiles[:idx], reg.Profiles[idx+1:]...)
	if err := atomicWriteRegistry(m.Path(), reg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}
	return nil
}
