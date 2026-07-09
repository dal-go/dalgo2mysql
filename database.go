// Package dalgo2mysql is the MySQL-specific DALgo driver.
//
// It composes [github.com/dal-go/dalgo2sql] for the [dal.DB] read/write
// surface (transactions, recordset reader, Get/Set/Insert/Delete) and
// adds MySQL-native implementations of:
//
//   - [dbschema.SchemaReader] for schema introspection via information_schema
//     scoped to the connected database
//   - [ddl.SchemaModifier] for MySQL-flavored CREATE / DROP / ALTER
//   - [dal.ConcurrencyAware] returning true (MySQL supports concurrent
//     connections from multiple goroutines and processes)
package dalgo2mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo2sql"

	_ "github.com/go-sql-driver/mysql" // register the "mysql" driver (pure Go, CGO_ENABLED=0)
)

// Database is the dalgo2mysql driver instance. It implements
// [dal.DB] by delegating to an inner [dal.DB] obtained from
// [dalgo2sql.NewDatabase], and adds MySQL-specific dbschema, ddl,
// and concurrency surfaces.
//
// Construct via [NewDatabase]. Database values are safe for concurrent
// use — the underlying MySQL server and go-sql-driver connection pool both
// support concurrent connections from multiple goroutines.
type Database struct {
	dal.ConcurrencyAvailable // SupportsConcurrentConnections() = true

	innerDB dal.DB  // delegate for the dal.DB surface
	sqlDB   *sql.DB // direct handle for DDL + introspection queries
	dsn     string  // remembered for diagnostics
}

// NewDatabase opens a connection to the MySQL server identified by dsn
// using github.com/go-sql-driver/mysql (pure Go, CGO_ENABLED=0), pings to
// surface connectivity errors at construction time, wraps the *sql.DB via
// dalgo2sql.NewDatabase for the dal.DB surface, and returns a *Database
// that satisfies dal.DB + dal.ConcurrencyAware.
//
// The dsn is in go-sql-driver/mysql format, e.g.
// "user:pass@tcp(127.0.0.1:3306)/dbname?parseTime=true".
//
// Use [NewDatabaseWithOptions] when you need to supply per-collection
// primary-key metadata (required for Insert/Get/Delete with map[string]any data).
func NewDatabase(dsn string) (*Database, error) {
	return NewDatabaseWithOptions(dsn, dal.NewSchema(nil, nil), dalgo2sql.DbOptions{})
}

// NewDatabaseWithOptions is like [NewDatabase] but accepts a dal.Schema and
// dalgo2sql.DbOptions so callers can configure per-collection primary-key
// mappings required by Insert/Get/Delete operations.
//
// Example — open a DB whose "widgets" table has "id" as its primary key:
//
//	db, err := dalgo2mysql.NewDatabaseWithOptions(dsn, dal.NewSchema(nil, nil),
//	    dalgo2sql.DbOptions{
//	        Recordsets: map[string]*dalgo2sql.Recordset{
//	            "widgets": dalgo2sql.NewRecordset("widgets", dalgo2sql.Table,
//	                []dal.FieldRef{dal.Field("id")}),
//	        },
//	    })
func NewDatabaseWithOptions(dsn string, schema dal.Schema, opts dalgo2sql.DbOptions) (*Database, error) {
	// MySQL uses "?" positional placeholders, which is dalgo2sql's default
	// (PlaceholderQuestion / zero value). We intentionally do NOT override
	// opts.Placeholder. MySQL with lower_case_table_names=0 preserves
	// identifier case and does not fold, so the bare identifiers that
	// dalgo2sql's DML and dal's structured-query rendering emit match the
	// backtick-quoted DDL this package produces without any case folding.

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: sql.Open(%q): %w", dsn, err)
	}
	if pingErr := sqlDB.PingContext(context.Background()); pingErr != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("dalgo2mysql: PingContext(%q): %w", dsn, pingErr)
	}
	innerDB := dalgo2sql.NewDatabase(sqlDB, schema, opts)
	return &Database{
		innerDB: innerDB,
		sqlDB:   sqlDB,
		dsn:     dsn,
	}, nil
}

// Close closes the underlying *sql.DB. After Close the Database value
// is unusable; further method calls will fail with an error from
// database/sql.
func (d *Database) Close() error {
	if d.sqlDB == nil {
		return nil
	}
	return d.sqlDB.Close()
}

// ID returns the driver-issued database ID (delegated to dalgo2sql).
func (d *Database) ID() string { return d.innerDB.ID() }

// Adapter returns the driver/version identifier.
func (d *Database) Adapter() dal.Adapter {
	return dal.NewAdapter("dalgo2mysql", Version)
}

// Schema returns the dal-level Schema (delegated to dalgo2sql).
func (d *Database) Schema() dal.Schema { return d.innerDB.Schema() }

// Version is the dalgo2mysql package version. Updated by hand on
// each release; consumed by Adapter.Version().
const Version = "0.1.0"
