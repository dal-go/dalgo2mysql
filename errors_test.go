package dalgo2mysql

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestIsAlreadyExists_DuplicateEntry proves the affirmative case:
// ER_DUP_ENTRY (1062), the standard "Duplicate entry '%s' for key %d/'%s'"
// error MySQL raises on a unique or primary-key violation, is classified as
// a duplicate key.
func TestIsAlreadyExists_DuplicateEntry(t *testing.T) {
	err := &mysql.MySQLError{Number: erDupEntry, Message: "Duplicate entry 'id1' for key 'PRIMARY'"}
	if !IsAlreadyExists(err) {
		t.Errorf("IsAlreadyExists(ER_DUP_ENTRY) = false, want true")
	}
}

// TestIsAlreadyExists_DuplicateEntryWithKeyName proves ER_DUP_ENTRY_WITH_KEY_NAME
// (1586) is classified identically to ER_DUP_ENTRY (1062): the server emits
// this variant instead of 1062 in call sites (bulk load, ALTER TABLE index
// rebuild) where only the violated key's name, not its internal index
// number, is available — it is the same duplicate-key condition, not a
// different one, so treating it any differently from 1062 would make
// dalgo2mysql's classification depend on which code path inside the server
// happened to raise the error rather than on what the error means.
func TestIsAlreadyExists_DuplicateEntryWithKeyName(t *testing.T) {
	err := &mysql.MySQLError{Number: erDupEntryWithKeyName, Message: "Duplicate entry 'id1' for key 'users.PRIMARY'"}
	if !IsAlreadyExists(err) {
		t.Errorf("IsAlreadyExists(ER_DUP_ENTRY_WITH_KEY_NAME) = false, want true")
	}
}

// TestIsAlreadyExists_ForeignKeyViolationNotClassified proves the negative
// case the task is most concerned about: a foreign-key violation
// (ER_NO_REFERENCED_ROW_2, 1452) is a different constraint (referential
// integrity, not uniqueness) and must NOT be reported as a duplicate key.
func TestIsAlreadyExists_ForeignKeyViolationNotClassified(t *testing.T) {
	err := &mysql.MySQLError{Number: erNoReferencedRow2, Message: "Cannot add or update a child row: a foreign key constraint fails"}
	if IsAlreadyExists(err) {
		t.Errorf("IsAlreadyExists(ER_NO_REFERENCED_ROW_2) = true, want false")
	}
}

// TestIsAlreadyExists_OtherErrorsNotClassified proves a handful of other
// error shapes are all left unclassified: a generic non-MySQL error, a MySQL
// error with an unrelated number, and nil.
func TestIsAlreadyExists_OtherErrorsNotClassified(t *testing.T) {
	cases := map[string]error{
		"generic error":         errors.New("connection reset by peer"),
		"unrelated MySQL error": &mysql.MySQLError{Number: 1146, Message: "Table 'db.widgets' doesn't exist"},
		"nil":                   nil,
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			if IsAlreadyExists(err) {
				t.Errorf("IsAlreadyExists(%v) = true, want false", err)
			}
		})
	}
}

// TestIsAlreadyExists_UnwrapsWrappedError proves errors.As-based detection
// reaches a *mysql.MySQLError buried inside a wrapped chain, the same shape
// database/sql and any intermediate wrapping in this package's own error
// paths would produce.
func TestIsAlreadyExists_UnwrapsWrappedError(t *testing.T) {
	driverErr := &mysql.MySQLError{Number: erDupEntry, Message: "Duplicate entry 'id1' for key 'PRIMARY'"}
	wrapped := fmt.Errorf("dalgo2mysql: exec insert: %w", driverErr)
	if !IsAlreadyExists(wrapped) {
		t.Errorf("IsAlreadyExists(wrapped ER_DUP_ENTRY) = false, want true")
	}
}
