package dalgo2mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

func TestDescribeCollection_ForeignKeys(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	parent := uniqueTable(t, "fk_parent")
	child := uniqueTable(t, "fk_child")
	dropTable(t, db, child)
	dropTable(t, db, parent)
	defer dropTable(t, db, parent)
	defer dropTable(t, db, child)
	for _, statement := range []string{
		`CREATE TABLE ` + quoteIdent(parent) + ` (a INT NOT NULL, b INT NOT NULL, PRIMARY KEY (a,b))`,
		`CREATE TABLE ` + quoteIdent(child) + ` (id INT PRIMARY KEY, a INT, b INT,
		 FOREIGN KEY (a,b) REFERENCES ` + quoteIdent(parent) + `(a,b)
		 ON DELETE CASCADE ON UPDATE RESTRICT)`,
	} {
		if _, err := db.sqlDB.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	ref := dal.NewRootCollectionRef(child, "")
	def, err := db.DescribeCollection(ctx, &ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(def.ForeignKeys) != 1 {
		t.Fatalf("foreign keys: %+v", def.ForeignKeys)
	}
	fk := def.ForeignKeys[0]
	if fk.ReferencedCollection != parent || fk.ReferencedNamespace != "" ||
		len(fk.Fields) != 2 || fk.Fields[0] != "a" || fk.Fields[1] != "b" ||
		len(fk.ReferencedFields) != 2 || fk.ReferencedFields[0] != "a" || fk.ReferencedFields[1] != "b" ||
		fk.OnDelete != "CASCADE" || fk.OnUpdate != "RESTRICT" ||
		fk.Enforcement != dbschema.ForeignKeyEnforcementEnabled {
		t.Fatalf("foreign key metadata: %+v", fk)
	}
}

func TestReadForeignKeys_Unit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success_enabled_and_composite", func(t *testing.T) {
		t.Parallel()
		sqlDB, _ := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "foreign_key_checks") {
					return &mockRows{cols: []string{"checks"}, rows: [][]driver.Value{{int64(1)}}}, nil, true
				}
				return &mockRows{
					cols: []string{"constraint_name", "column_name", "target_ns", "referenced_table_name", "referenced_column_name", "update_rule", "delete_rule"},
					rows: [][]driver.Value{
						{"fk_comp", "a", "", "parent", "a", "RESTRICT", "CASCADE"},
						{"fk_comp", "b", "", "parent", "b", "RESTRICT", "CASCADE"},
					},
				}, nil, true
			},
		})
		keys, err := readForeignKeys(ctx, sqlDB, "child")
		if err != nil {
			t.Fatalf("readForeignKeys: %v", err)
		}
		if len(keys) != 1 {
			t.Fatalf("len(keys) = %d, want 1", len(keys))
		}
		fk := keys[0]
		if fk.Name != "fk_comp" || fk.Enforcement != dbschema.ForeignKeyEnforcementEnabled || len(fk.Fields) != 2 || len(fk.ReferencedFields) != 2 {
			t.Errorf("unexpected fk: %+v", fk)
		}
	})

	t.Run("success_disabled_checks", func(t *testing.T) {
		t.Parallel()
		sqlDB, _ := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "foreign_key_checks") {
					return &mockRows{cols: []string{"checks"}, rows: [][]driver.Value{{int64(0)}}}, nil, true
				}
				return &mockRows{
					cols: []string{"constraint_name", "column_name", "target_ns", "referenced_table_name", "referenced_column_name", "update_rule", "delete_rule"},
					rows: [][]driver.Value{
						{"fk_1", "user_id", "other_db", "users", "id", "NO ACTION", "NO ACTION"},
					},
				}, nil, true
			},
		})
		keys, err := readForeignKeys(ctx, sqlDB, "orders")
		if err != nil {
			t.Fatalf("readForeignKeys: %v", err)
		}
		if len(keys) != 1 || keys[0].Enforcement != dbschema.ForeignKeyEnforcementDisabled {
			t.Errorf("expected disabled enforcement, got %+v", keys)
		}
	})

	t.Run("conn_error", func(t *testing.T) {
		t.Parallel()
		sqlDB, _ := newMockDB(t, &driverHooks{
			connectErr: errors.New("conn failed"),
		})
		if _, err := readForeignKeys(ctx, sqlDB, "child"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("checks_query_error", func(t *testing.T) {
		t.Parallel()
		sqlDB, _ := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "foreign_key_checks") {
					return nil, errors.New("checks failed"), true
				}
				return nil, nil, false
			},
		})
		if _, err := readForeignKeys(ctx, sqlDB, "child"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("fk_query_error", func(t *testing.T) {
		t.Parallel()
		sqlDB, _ := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "foreign_key_checks") {
					return &mockRows{cols: []string{"checks"}, rows: [][]driver.Value{{int64(1)}}}, nil, true
				}
				return nil, errors.New("fk query failed"), true
			},
		})
		if _, err := readForeignKeys(ctx, sqlDB, "child"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("fk_scan_error", func(t *testing.T) {
		t.Parallel()
		sqlDB, _ := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "foreign_key_checks") {
					return &mockRows{cols: []string{"checks"}, rows: [][]driver.Value{{int64(1)}}}, nil, true
				}
				return &mockRows{
					cols: []string{"a", "b", "c", "d", "e", "f", "g"},
					rows: [][]driver.Value{{nil, "a", "", "p", "a", "RESTRICT", "CASCADE"}},
				}, nil, true
			},
		})
		if _, err := readForeignKeys(ctx, sqlDB, "child"); err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("fk_rows_error", func(t *testing.T) {
		t.Parallel()
		sqlDB, _ := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "foreign_key_checks") {
					return &mockRows{cols: []string{"checks"}, rows: [][]driver.Value{{int64(1)}}}, nil, true
				}
				return &mockRows{
					cols:    []string{"a", "b", "c", "d", "e", "f", "g"},
					nextErr: errors.New("rows err"),
				}, nil, true
			},
		})
		if _, err := readForeignKeys(ctx, sqlDB, "child"); err == nil {
			t.Fatal("expected rows error")
		}
	})
}
