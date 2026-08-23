package dalgo2mysql

import (
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// newCollectionNotFoundError formats the standard "collection not
// found" error. The contract is content-based per the Feature spec:
// the message MUST contain the substring "not found" and the collection name.
func newCollectionNotFoundError(name string) error {
	return fmt.Errorf("dalgo2mysql: collection %q not found", name)
}

// MySQL server error numbers relevant to duplicate-key detection. See
// https://dev.mysql.com/doc/mysql-errors/en/server-error-reference.html.
const (
	// erDupEntry is ER_DUP_ENTRY: "Duplicate entry '%s' for key %d" (or,
	// on servers new enough to name the key, "for key '%s'"). This is the
	// standard unique/primary-key violation raised on INSERT.
	erDupEntry = 1062

	// erDupEntryWithKeyName is ER_DUP_ENTRY_WITH_KEY_NAME: the identical
	// duplicate-entry condition as erDupEntry, differing only in how the
	// server reports which key was violated (by name instead of by
	// internal index number) — the same code path in some storage-engine
	// call sites (notably ALTER TABLE / bulk load) surfaces it this way
	// because only the key's name, not its number, is available at that
	// point. It is not a different failure mode, so it MUST be classified
	// identically to erDupEntry.
	erDupEntryWithKeyName = 1586

	// erNoReferencedRow2 is ER_NO_REFERENCED_ROW_2, a foreign-key
	// violation on INSERT/UPDATE. Listed here only as a negative
	// reference: it is a different constraint (referential integrity, not
	// uniqueness) and must NOT be classified as a duplicate key.
	erNoReferencedRow2 = 1452
)

// IsAlreadyExists is a github.com/dal-go/dalgo2sql DbOptions.IsAlreadyExists
// classifier for github.com/go-sql-driver/mysql: it reports whether err is a
// MySQL duplicate-key violation — ER_DUP_ENTRY (1062) or its key-name
// variant ER_DUP_ENTRY_WITH_KEY_NAME (1586) — as opposed to any other insert
// failure. In particular a foreign-key violation such as ER_NO_REFERENCED_ROW_2
// (1452, see erNoReferencedRow2) is a different constraint and is
// deliberately NOT classified as a duplicate key.
//
// NewDatabaseWithOptions wires this in automatically whenever the caller's
// DbOptions.IsAlreadyExists is nil (see database.go), so most callers never
// need to reference it directly. It is exported so a caller constructing a
// dalgo2sql.DbOptions independently of this package's constructors can reuse
// the same detection logic:
//
//	dalgo2sql.DbOptions{IsAlreadyExists: dalgo2mysql.IsAlreadyExists}
func IsAlreadyExists(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	switch mysqlErr.Number {
	case erDupEntry, erDupEntryWithKeyName:
		return true
	default:
		return false
	}
}
