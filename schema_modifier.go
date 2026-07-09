package dalgo2mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/dal-go/dalgo/ddl"
)

// execer is the subset of *sql.DB / *sql.Tx used to run DDL. MySQL implicitly
// commits before and after each DDL statement, so schema changes are NOT
// rolled back on error (see SupportsTransactionalDDL). Statements are executed
// directly against the *sql.DB; on partial failure earlier statements remain
// applied.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// CreateCollection creates a table and its inline indexes.
//
// Because MySQL DDL is auto-committing (non-transactional), a failure while
// creating an index leaves the table — and any previously created indexes —
// in place. Callers that need clean rollback should drop the table on error.
func (d *Database) CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...ddl.Option) error {
	o := ddl.ResolveOptions(opts...)
	createSQL, err := buildCreateTableSQL(c, o)
	if err != nil {
		return err
	}
	indexSQLs := make([]string, 0, len(c.Indexes))
	for _, idx := range c.Indexes {
		if idx.Collection == "" {
			idx.Collection = c.Name
		}
		s, ierr := buildCreateIndexSQL(idx, o)
		if ierr != nil {
			return ierr
		}
		indexSQLs = append(indexSQLs, s)
	}

	if _, err := d.sqlDB.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("dalgo2mysql: CreateCollection exec %q: %w", createSQL, err)
	}
	for _, s := range indexSQLs {
		if _, err := d.sqlDB.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("dalgo2mysql: CreateCollection index exec %q: %w", s, err)
		}
	}
	return nil
}

// DropCollection drops the table and all its indexes (MySQL drops table-owned
// indexes automatically).
func (d *Database) DropCollection(ctx context.Context, name string, opts ...ddl.Option) error {
	o := ddl.ResolveOptions(opts...)
	sqlStmt := buildDropTableSQL(name, o)
	if _, err := d.sqlDB.ExecContext(ctx, sqlStmt); err != nil {
		return fmt.Errorf("dalgo2mysql: DropCollection exec: %w", err)
	}
	return nil
}

// AlterCollection applies ops in order. Because MySQL DDL auto-commits, ops are
// NOT applied atomically: a failure midway leaves earlier ops applied.
func (d *Database) AlterCollection(ctx context.Context, name string, ops ...ddl.AlterOp) error {
	a := &mysqlAlterApplier{ctx: ctx, exec: d.sqlDB, table: name}
	for _, op := range ops {
		if err := op.ApplyTo(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

// mysqlAlterApplier implements ddl.Applier for an AlterCollection call. One
// instance per call. Statements run directly against the *sql.DB; MySQL
// auto-commits each DDL statement, so there is no batch rollback.
type mysqlAlterApplier struct {
	ctx   context.Context
	exec  execer
	table string
}

func (a *mysqlAlterApplier) ApplyAddField(ctx context.Context, f dbschema.FieldDef, opts ddl.Options) error {
	sqlStmt, err := buildAlterTableAddColumnSQL(a.table, f)
	if err != nil {
		return err
	}
	if _, err := a.exec.ExecContext(ctx, sqlStmt); err != nil {
		return fmt.Errorf("dalgo2mysql: ApplyAddField %q: %w", f.Name, err)
	}
	return nil
}

func (a *mysqlAlterApplier) ApplyDropField(ctx context.Context, name dal.FieldName, opts ddl.Options) error {
	sqlStmt := buildAlterTableDropColumnSQL(a.table, name)
	if _, err := a.exec.ExecContext(ctx, sqlStmt); err != nil {
		return fmt.Errorf("dalgo2mysql: ApplyDropField %q: %w", name, err)
	}
	return nil
}

func (a *mysqlAlterApplier) ApplyRenameField(ctx context.Context, oldName, newName dal.FieldName, opts ddl.Options) error {
	sqlStmt := buildAlterTableRenameColumnSQL(a.table, oldName, newName)
	if _, err := a.exec.ExecContext(ctx, sqlStmt); err != nil {
		return fmt.Errorf("dalgo2mysql: ApplyRenameField %q->%q: %w", oldName, newName, err)
	}
	return nil
}

func (a *mysqlAlterApplier) ApplyAddIndex(ctx context.Context, idx dbschema.IndexDef, opts ddl.Options) error {
	if idx.Collection == "" {
		idx.Collection = a.table
	}
	sqlStmt, err := buildCreateIndexSQL(idx, opts)
	if err != nil {
		return err
	}
	if _, err := a.exec.ExecContext(ctx, sqlStmt); err != nil {
		return fmt.Errorf("dalgo2mysql: ApplyAddIndex %q: %w", idx.Name, err)
	}
	return nil
}

func (a *mysqlAlterApplier) ApplyDropIndex(ctx context.Context, name string, opts ddl.Options) error {
	sqlStmt := buildDropIndexSQL(name, a.table, opts)
	if _, err := a.exec.ExecContext(ctx, sqlStmt); err != nil {
		return fmt.Errorf("dalgo2mysql: ApplyDropIndex %q: %w", name, err)
	}
	return nil
}

// ApplyModifyField alters a column's type in MySQL using MODIFY COLUMN. The
// full new column definition (type + nullability) is respecified, as MySQL's
// MODIFY replaces the whole column definition.
func (a *mysqlAlterApplier) ApplyModifyField(ctx context.Context, name dal.FieldName, newDef dbschema.FieldDef, opts ddl.Options) error {
	if err := validateIdent(a.table); err != nil {
		return err
	}
	if err := validateIdent(string(name)); err != nil {
		return err
	}
	sqlType, err := mysqlTypeFor(newDef)
	if err != nil {
		return fmt.Errorf("dalgo2mysql: ApplyModifyField %q: %w", name, err)
	}
	nullClause := "NULL"
	if !newDef.Nullable {
		nullClause = "NOT NULL"
	}
	stmt := fmt.Sprintf(
		"ALTER TABLE %s MODIFY COLUMN %s %s %s",
		quoteIdent(a.table), quoteIdent(string(name)), sqlType, nullClause,
	)
	if _, err := a.exec.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("dalgo2mysql: ApplyModifyField %q: %w", name, err)
	}
	return nil
}
