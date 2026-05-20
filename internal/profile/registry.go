package profile

import (
	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/pelletier/go-toml/v2"
)

// registry is the on-disk shape of profiles.toml. The TOML form is a single
// table-array under the key "profile":
//
//	[[profile]]
//	name        = "work"
//	config_dir  = "/home/u/.claude-profiles/work"
//	...
type registry struct {
	Profiles []contracts.Profile `toml:"profile"`
}

// encodeRegistry serializes r to TOML bytes.
func encodeRegistry(r registry) ([]byte, error) {
	return toml.Marshal(r)
}

// decodeRegistry parses TOML bytes into a registry. An empty or nil input
// produces an empty registry without error so that a freshly created file
// (or no file at all) round-trips cleanly.
func decodeRegistry(data []byte) (registry, error) {
	var r registry
	if len(data) == 0 {
		return r, nil
	}
	if err := toml.Unmarshal(data, &r); err != nil {
		return registry{}, err
	}
	return r, nil
}
