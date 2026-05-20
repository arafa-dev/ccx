package storage_test

import (
	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

// Compile-time assertion that *storage.Store satisfies contracts.Store. If a
// future contract method is added, this file will fail to build until Store
// implements it.
var _ contracts.Store = (*storage.Store)(nil)
