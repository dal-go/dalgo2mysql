package dalgo2mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo2sql"
)

const envDSN = "DALGO2MYSQL_TEST_DSN"

// testDSN returns the DSN from the environment or skips the test.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv(envDSN)
	if dsn == "" {
		t.Skipf("skipping DB test: %s not set", envDSN)
	}
	return dsn
}

// openTestDB opens a live MySQL connection and registers cleanup.
func openTestDB(t *testing.T) *Database {
	t.Helper()
	db, err := NewDatabase(testDSN(t))
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// openTestDBWithOpts opens a live MySQL connection with per-table recordset options.
func openTestDBWithOpts(t *testing.T, opts dalgo2sql.DbOptions) *Database {
	t.Helper()
	db, err := NewDatabaseWithOptions(testDSN(t), dal.NewSchema(nil, nil), opts)
	if err != nil {
		t.Fatalf("NewDatabaseWithOptions: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// uniqueTable returns a test-unique table name so parallel tests don't clash.
// MySQL (lower_case_table_names=0) preserves case, so we keep the sanitized
// name as-is rather than folding to lower case.
func uniqueTable(t *testing.T, base string) string {
	name := fmt.Sprintf("test_%s_%s", base, sanitizeName(t.Name()))
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func sanitizeName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// dropTable drops a table unconditionally; used in test cleanup.
func dropTable(t *testing.T, db *Database, table string) {
	t.Helper()
	if _, err := db.sqlDB.ExecContext(context.Background(),
		`DROP TABLE IF EXISTS `+quoteIdent(table)); err != nil {
		t.Logf("dropTable %q: %v (non-fatal)", table, err)
	}
}

func TestNewDatabase_Connects(t *testing.T) {
	t.Parallel()
	dsn := testDSN(t)
	db, err := NewDatabase(dsn)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	defer func() { _ = db.Close() }()
	if db == nil {
		t.Fatal("NewDatabase returned nil")
	}
}

func TestNewDatabase_RejectsBadDSN(t *testing.T) {
	t.Parallel()
	db, err := NewDatabase("nobody:wrong@tcp(127.0.0.1:13306)/noexist_db_xyz?parseTime=true")
	if err == nil {
		_ = db.Close()
		t.Fatal("NewDatabase: expected error on bad DSN, got nil")
	}
	if db != nil {
		t.Errorf("expected nil db on error, got %T", db)
	}
}

// Not parallel: mutates package-level sqlOpen seam.
func TestNewDatabase_Unit(t *testing.T) {
	orig := sqlOpen
	t.Cleanup(func() { sqlOpen = orig })

	t.Run("success_and_close", func(t *testing.T) {
		sqlOpen = func(dsn string) (*sql.DB, error) {
			return sql.OpenDB(&mockConnector{}), nil
		}
		db, err := NewDatabase("mock-dsn")
		if err != nil {
			t.Fatalf("NewDatabase: %v", err)
		}
		if db == nil {
			t.Fatal("NewDatabase returned nil")
		}
		if err := db.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("open_error", func(t *testing.T) {
		sqlOpen = func(dsn string) (*sql.DB, error) {
			return nil, errors.New("open failed")
		}
		db, err := NewDatabaseWithOptions("bad", dal.NewSchema(nil, nil), dalgo2sql.DbOptions{})
		if err == nil {
			t.Fatal("expected error when sqlOpen fails")
		}
		if db != nil {
			t.Errorf("expected nil db, got %v", db)
		}
	})
}

func TestDatabase_CloseNil(t *testing.T) {
	t.Parallel()
	var d Database
	if err := d.Close(); err != nil {
		t.Fatalf("Close on nil sqlDB returned error: %v", err)
	}
}

func TestDatabase_SupportsConcurrentConnections_True(t *testing.T) {
	t.Parallel()
	_, db := newMockDB(t, nil)
	if !db.SupportsConcurrentConnections() {
		t.Error("expected SupportsConcurrentConnections() == true for MySQL, got false")
	}
}

func TestDatabase_SupportsTransactionalDDL_False(t *testing.T) {
	t.Parallel()
	_, db := newMockDB(t, nil)
	if db.SupportsTransactionalDDL() {
		t.Error("expected SupportsTransactionalDDL() == false for MySQL, got true")
	}
}

func TestDatabase_DelegatedMethods(t *testing.T) {
	t.Parallel()
	_, db := newMockDBWithOpts(t, nil, dalgo2sql.DbOptions{
		ID: "test-db-id",
	})
	if got := db.ID(); got != "test-db-id" {
		t.Errorf("ID() = %q, want %q", got, "test-db-id")
	}
	adapter := db.Adapter()
	if adapter.Name() != "dalgo2mysql" {
		t.Errorf("Adapter().Name() = %q, want dalgo2mysql", adapter.Name())
	}
	if adapter.Version() != Version {
		t.Errorf("Adapter().Version() = %q, want %q", adapter.Version(), Version)
	}
	if gotSchema := db.Schema(); gotSchema == nil {
		t.Error("Schema() returned nil")
	}
}
