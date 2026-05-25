package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/headroom"
)

func suggestHeadroomStore(deps *Deps) (headroom.Store, error) {
	store, ok := deps.Store.(headroom.Store)
	if !ok {
		return nil, fmt.Errorf("suggest requires headroom.Store, got %T", deps.Store)
	}
	return store, nil
}
