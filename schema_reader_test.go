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

func TestListCollections_Basic(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()

	tbl1 := uniqueTable(t, "lc1")
	tbl2 := uniqueTable(t, "lc2")
	dropTable(t, db, tbl1)
	dropTable(t, db, tbl2)
	defer dropTable(t, db, tbl1)
	defer dropTable(t, db, tbl2)

	for _, stmt := range []string{
		`CREATE TABLE ` + quoteIdent(tbl1) + " (`id` VARCHAR(255) NOT NULL PRIMARY KEY)",
		`CREATE TABLE ` + quoteIdent(tbl2) + " (`id` VARCHAR(255) NOT NULL PRIMARY KEY)",
	} {
		if _, err := db.sqlDB.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.ListCollections(ctx, nil)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	found := make(map[string]bool)
	for _, ref := range got {
		found[ref.Name()] = true
	}
	for _, want := range []string{tbl1, tbl2} {
		if !found[want] {
			t.Errorf("expected %q in ListCollections, not found; got %+v", want, got)
		}
	}
}

func TestDescribeCollection_NotFound(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	ref := dal.NewRootCollectionRef("no_such_table_xyz_123", "")
	got, err := db.DescribeCollection(ctx, &ref)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil CollectionDef on error, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error message, got: %s", err.Error())
	}
}

func TestDescribeCollection_BasicRoundTrip(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "desc")
	dropTable(t, db, tbl)
	defer dropTable(t, db, tbl)

	stmt := `CREATE TABLE ` + quoteIdent(tbl) + ` (
		` + "`id`" + `         VARCHAR(255) NOT NULL,
		` + "`email`" + `      VARCHAR(255) NOT NULL,
		` + "`age`" + `        BIGINT,
		` + "`score`" + `      DOUBLE,
		` + "`active`" + `     BOOLEAN,
		` + "`created_at`" + ` DATETIME,
		` + "`balance`" + `    DECIMAL(10,2),
		` + "`payload`" + `    BLOB,
		PRIMARY KEY (` + "`id`" + `)
	)`
	if _, err := db.sqlDB.ExecContext(ctx, stmt); err != nil {
		t.Fatal(err)
	}

	ref := dal.NewRootCollectionRef(tbl, "")
	got, err := db.DescribeCollection(ctx, &ref)
	if err != nil {
		t.Fatalf("DescribeCollection: %v", err)
	}
	if got.Name != tbl {
		t.Errorf("Name = %q, want %q", got.Name, tbl)
	}
	if len(got.Fields) != 8 {
		t.Fatalf("Fields len = %d, want 8; fields = %+v", len(got.Fields), got.Fields)
	}

	byName := make(map[string]dbschema.FieldDef)
	for _, f := range got.Fields {
		byName[string(f.Name)] = f
	}

	check := func(name string, want dbschema.Type, wantNullable bool) {
		t.Helper()
		f, ok := byName[name]
		if !ok {
			t.Errorf("field %q missing from DescribeCollection result", name)
			return
		}
		if f.Type != want {
			t.Errorf("field %q: Type = %v, want %v", name, f.Type, want)
		}
		if f.Nullable != wantNullable {
			t.Errorf("field %q: Nullable = %v, want %v", name, f.Nullable, wantNullable)
		}
	}
	check("id", dbschema.String, false)
	check("email", dbschema.String, false)
	check("age", dbschema.Int, true)
	check("score", dbschema.Float, true)
	check("active", dbschema.Bool, true)
	check("created_at", dbschema.Time, true)
	check("balance", dbschema.Decimal, true)
	check("payload", dbschema.Bytes, true)

	bal := byName["balance"]
	if bal.Precision == nil || bal.Precision.Total != 10 || bal.Precision.Scale != 2 {
		t.Errorf("balance Precision = %+v, want (10,2)", bal.Precision)
	}

	if len(got.PrimaryKey) != 1 || string(got.PrimaryKey[0]) != "id" {
		t.Errorf("PrimaryKey = %v, want [id]", got.PrimaryKey)
	}
}

func TestListIndexes_ExcludesPK(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "lidx")
	dropTable(t, db, tbl)
	defer dropTable(t, db, tbl)

	for _, stmt := range []string{
		`CREATE TABLE ` + quoteIdent(tbl) + " (`id` VARCHAR(255) NOT NULL, `email` VARCHAR(255), PRIMARY KEY (`id`))",
		`CREATE INDEX ` + quoteIdent(tbl+"_email_idx") + ` ON ` + quoteIdent(tbl) + " (`email`)",
		`CREATE UNIQUE INDEX ` + quoteIdent(tbl+"_email_uq") + ` ON ` + quoteIdent(tbl) + " (`email`)",
	} {
		if _, err := db.sqlDB.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	ref := dal.NewRootCollectionRef(tbl, "")
	got, err := db.ListIndexes(ctx, &ref)
	if err != nil {
		t.Fatalf("ListIndexes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 user-defined indexes (PK excluded), got %d: %+v", len(got), got)
	}
	wantNames := map[string]bool{
		tbl + "_email_idx": false,
		tbl + "_email_uq":  true,
	}
	for _, idx := range got {
		wantUnique, known := wantNames[idx.Name]
		if !known {
			t.Errorf("unexpected index %q", idx.Name)
			continue
		}
		if idx.Unique != wantUnique {
			t.Errorf("index %q: Unique = %v, want %v", idx.Name, idx.Unique, wantUnique)
		}
	}
}

func TestSchemaReader_ListCollections_Unit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"table_name"},
					rows: [][]driver.Value{{"orders"}, {"users"}},
				}, nil, true
			},
		})
		got, err := db.ListCollections(ctx, nil)
		if err != nil {
			t.Fatalf("ListCollections: %v", err)
		}
		if len(got) != 2 || got[0].Name() != "orders" || got[1].Name() != "users" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return nil, errors.New("query failed"), true
			},
		})
		if _, err := db.ListCollections(ctx, nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("scan_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"table_name"},
					rows: [][]driver.Value{{nil}},
				}, nil, true
			},
		})
		if _, err := db.ListCollections(ctx, nil); err == nil {
			t.Fatal("expected scan error")
		}
	})
}

func TestSchemaReader_DescribeCollection_Unit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ref := dal.NewRootCollectionRef("users", "")

	t.Run("success_full", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				switch {
				case strings.Contains(query, "FROM information_schema.tables"):
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.table_constraints"):
					return &mockRows{cols: []string{"column_name"}, rows: [][]driver.Value{{"id"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.columns"):
					return &mockRows{
						cols: []string{"column_name", "data_type", "character_maximum_length", "numeric_precision", "numeric_scale", "is_nullable"},
						rows: [][]driver.Value{
							{"id", "varchar", int64(64), nil, nil, "NO"},
							{"balance", "decimal", nil, int64(10), int64(2), "YES"},
						},
					}, nil, true
				case strings.Contains(query, "FROM information_schema.statistics"):
					return &mockRows{
						cols: []string{"index_name", "non_unique", "seq_in_index", "column_name"},
						rows: [][]driver.Value{
							{"ix_comp", int64(1), int64(1), "id"},
							{"ix_comp", int64(1), int64(2), "balance"},
						},
					}, nil, true
				case strings.Contains(query, "foreign_key_checks"):
					return &mockRows{cols: []string{"checks"}, rows: [][]driver.Value{{int64(1)}}}, nil, true
				case strings.Contains(query, "referential_constraints"):
					return &mockRows{cols: []string{"a", "b", "c", "d", "e", "f", "g"}}, nil, true
				}
				return nil, nil, false
			},
		})
		got, err := db.DescribeCollection(ctx, &ref)
		if err != nil {
			t.Fatalf("DescribeCollection: %v", err)
		}
		if got.Name != "users" || len(got.Fields) != 2 || len(got.PrimaryKey) != 1 || len(got.Indexes) != 1 {
			t.Fatalf("unexpected CollectionDef: %+v", got)
		}
		if got.Fields[0].Length == nil || *got.Fields[0].Length != 64 {
			t.Errorf("expected length 64, got %v", got.Fields[0].Length)
		}
		if got.Fields[1].Precision == nil || got.Fields[1].Precision.Total != 10 || got.Fields[1].Precision.Scale != 2 || !got.Fields[1].Nullable {
			t.Errorf("unexpected balance field: %+v", got.Fields[1])
		}
		if len(got.Indexes[0].Fields) != 2 {
			t.Errorf("expected 2 fields in composite index, got %+v", got.Indexes[0])
		}
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{cols: []string{"table_name"}}, nil, true
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("probe_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return nil, errors.New("probe boom"), true
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("pk_query_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "FROM information_schema.tables") {
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				}
				return nil, errors.New("pk query boom"), true
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("pk_scan_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				if strings.Contains(query, "FROM information_schema.tables") {
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				}
				return &mockRows{cols: []string{"column_name"}, rows: [][]driver.Value{{nil}}}, nil, true
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("columns_query_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				switch {
				case strings.Contains(query, "FROM information_schema.tables"):
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.table_constraints"):
					return &mockRows{cols: []string{"column_name"}}, nil, true
				default:
					return nil, errors.New("columns boom"), true
				}
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("columns_scan_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				switch {
				case strings.Contains(query, "FROM information_schema.tables"):
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.table_constraints"):
					return &mockRows{cols: []string{"column_name"}}, nil, true
				default:
					return &mockRows{
						cols: []string{"a", "b", "c", "d", "e", "f"},
						rows: [][]driver.Value{{nil, "varchar", nil, nil, nil, "YES"}},
					}, nil, true
				}
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("columns_unsupported_type", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				switch {
				case strings.Contains(query, "FROM information_schema.tables"):
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.table_constraints"):
					return &mockRows{cols: []string{"column_name"}}, nil, true
				default:
					return &mockRows{
						cols: []string{"a", "b", "c", "d", "e", "f"},
						rows: [][]driver.Value{{"geo", "geometry", nil, nil, nil, "YES"}},
					}, nil, true
				}
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error for unsupported MySQL type")
		}
	})

	t.Run("columns_rows_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				switch {
				case strings.Contains(query, "FROM information_schema.tables"):
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.table_constraints"):
					return &mockRows{cols: []string{"column_name"}}, nil, true
				default:
					return &mockRows{
						cols:    []string{"a", "b", "c", "d", "e", "f"},
						nextErr: errors.New("rows err"),
					}, nil, true
				}
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("indexes_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				switch {
				case strings.Contains(query, "FROM information_schema.tables"):
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.table_constraints"):
					return &mockRows{cols: []string{"column_name"}}, nil, true
				case strings.Contains(query, "FROM information_schema.columns"):
					return &mockRows{cols: []string{"a", "b", "c", "d", "e", "f"}}, nil, true
				default:
					return nil, errors.New("indexes boom"), true
				}
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("foreign_keys_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				switch {
				case strings.Contains(query, "FROM information_schema.tables"):
					return &mockRows{cols: []string{"table_name"}, rows: [][]driver.Value{{"users"}}}, nil, true
				case strings.Contains(query, "FROM information_schema.table_constraints"):
					return &mockRows{cols: []string{"column_name"}}, nil, true
				case strings.Contains(query, "FROM information_schema.columns"):
					return &mockRows{cols: []string{"a", "b", "c", "d", "e", "f"}}, nil, true
				case strings.Contains(query, "FROM information_schema.statistics"):
					return &mockRows{cols: []string{"a", "b", "c", "d"}}, nil, true
				default:
					return nil, errors.New("fk boom"), true
				}
			},
		})
		if _, err := db.DescribeCollection(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestSchemaReader_ListIndexes_Unit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ref := dal.NewRootCollectionRef("users", "")

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"index_name", "non_unique", "seq_in_index", "column_name"},
					rows: [][]driver.Value{
						{"uq_email", int64(0), int64(1), "email"},
					},
				}, nil, true
			},
		})
		got, err := db.ListIndexes(ctx, &ref)
		if err != nil {
			t.Fatalf("ListIndexes: %v", err)
		}
		if len(got) != 1 || !got[0].Unique || got[0].Name != "uq_email" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("scan_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"index_name", "non_unique", "seq_in_index", "column_name"},
					rows: [][]driver.Value{{nil, int64(0), int64(1), "email"}},
				}, nil, true
			},
		})
		if _, err := db.ListIndexes(ctx, &ref); err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("rows_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols:    []string{"index_name", "non_unique", "seq_in_index", "column_name"},
					nextErr: errors.New("rows err"),
				}, nil, true
			},
		})
		if _, err := db.ListIndexes(ctx, &ref); err == nil {
			t.Fatal("expected rows error")
		}
	})
}
