package pricing_test

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestYAMLDepAvailable verifies the yaml.v3 module is wired into go.mod.
// It does no real work -- it just forces the dependency to be linkable from
// this package, which is the only place in ccx allowed to import yaml.v3.
func TestYAMLDepAvailable(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("hello: world\n"), &node); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if node.Kind == 0 {
		t.Fatalf("expected non-zero Kind after Unmarshal")
	}
}
