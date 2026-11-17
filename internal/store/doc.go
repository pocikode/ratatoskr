// Package store opens the Ratatoskr SQLite databases (instance.db and
// project.db), applies embedded forward-only migrations and exposes a
// serialized write handle plus a read pool (docs/03-architecture.md §5,
// ADR-0002).
package store
