package cost

import "testing"

// TestPostgresStoreImplementsInterface verifies at compile time that
// PostgresStore satisfies the Store interface.
func TestPostgresStoreImplementsInterface(t *testing.T) {
	// This is a compile-time check — if PostgresStore doesn't implement Store,
	// this file won't compile.
	var _ Store = (*PostgresStore)(nil)
}
