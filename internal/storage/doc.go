// Package storage provides a SQLite-backed implementation of contracts.Store.
//
// The schema is defined in schema.sql and embedded at build time. Migrations
// (when added in a later release) live alongside this file and are run by
// (*Store).Migrate.
package storage
