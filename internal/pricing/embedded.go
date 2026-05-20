package pricing

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// embeddedModelsYAML is the v0.1 baseline pricing table.
//
//go:embed models.yaml
var embeddedModelsYAML []byte

// NewTable constructs a Table by parsing the embedded baseline YAML and, if
// present, layering the user override at ~/.ccx/pricing.yaml on top.
func NewTable() (*Table, error) {
	override, err := readUserOverride()
	if err != nil {
		return nil, err
	}
	return NewTableFromBytes(embeddedModelsYAML, override)
}

// readUserOverride reads ~/.ccx/pricing.yaml. Returns (nil, nil) if absent.
func readUserOverride() ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving user home dir: %w", err)
	}
	path := filepath.Join(home, ".ccx", "pricing.yaml")

	// #nosec G304 -- pricing overrides are intentionally read from this fixed
	// per-user config path.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}
