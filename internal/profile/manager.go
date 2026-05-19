package profile

import (
	"errors"
	"path/filepath"
)

// fileName is the canonical name of the registry file inside the ccx root.
const fileName = "profiles.toml"

// Manager owns the profile registry at <root>/profiles.toml. All mutating
// methods rewrite the whole file atomically. The zero Manager is not usable;
// always construct via NewManager.
type Manager struct {
	root string
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
