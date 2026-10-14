package dalgo2mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/dal-go/dalgo/ddl"
)

func TestCreateCollection_HappyPath(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "users")
	dropTable(t, db, tbl) // clean any residue from a prior run
	defer dropTable(t, db, tbl)

	c := dbschema.CollectionDef{
		Name: tbl,
		Fields: []dbschema.FieldDef{
			{Name: dal.FieldName("id"), Type: dbschema.String, Nullable: false},
			{Name: dal.FieldName("email"), Type: dbschema.String, Nullable: false},
			{Name: dal.FieldName("age"), Type: dbschema.Int, Nullable: true},
			{Name: dal.FieldName("score"), Type: dbschema.Float, Nullable: true},
			{Name: dal.FieldName("active"), Type: dbschema.Bool, Nullable: true},
			{Name: dal.FieldName("created_at"), Type: dbschema.Time, Nullable: true},
			{Name: dal.FieldName("balance"), Type: dbschema.Decimal, Nullable: true},
			{Name: dal.FieldName("payload"), Type: dbschema.Bytes, Nullable: true},
		},
		PrimaryKey: []dal.FieldName{"id"},
		Indexes: []dbschema.IndexDef{
			{Name: tbl + "_email_idx", Collection: tbl, Fields: []dal.FieldName{"email"}},
		},
	}

	if err := ddl.CreateCollection(ctx, db, c); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ref := dal.NewRootCollectionRef(tbl, "")
	got, err := db.DescribeCollection(ctx, &ref)
	if err != nil {
		t.Fatalf("DescribeCollection after Create: %v", err)
	}
	if len(got.Fields) != len(c.Fields) {
		t.Errorf("Fields len = %d, want %d", len(got.Fields), len(c.Fields))
	}
	if len(got.PrimaryKey) != 1 || string(got.PrimaryKey[0]) != "id" {
		t.Errorf("PrimaryKey = %v, want [id]", got.PrimaryKey)
	}
	if len(got.Indexes) != 1 {
		t.Errorf("Indexes len = %d, want 1; got %+v", len(got.Indexes), got.Indexes)
	}
}

func TestCreateCollection_IfNotExists(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "ine")
	dropTable(t, db, tbl)
	defer dropTable(t, db, tbl)

	c := dbschema.CollectionDef{
		Name:       tbl,
		Fields:     []dbschema.FieldDef{{Name: dal.FieldName("id"), Type: dbschema.String, Nullable: false}},
		PrimaryKey: []dal.FieldName{"id"},
	}
	if err := ddl.CreateCollection(ctx, db, c); err != nil {
		t.Fatalf("first CreateCollection: %v", err)
	}
	if err := ddl.CreateCollection(ctx, db, c, ddl.IfNotExists()); err != nil {
		t.Fatalf("CreateCollection with IfNotExists on existing table: %v", err)
	}
}

func TestDropCollection(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "drop")
	dropTable(t, db, tbl)
	// Don't defer drop — the test drops it explicitly.

	c := dbschema.CollectionDef{
		Name:       tbl,
		Fields:     []dbschema.FieldDef{{Name: dal.FieldName("id"), Type: dbschema.String, Nullable: false}},
		PrimaryKey: []dal.FieldName{"id"},
	}
	if err := ddl.CreateCollection(ctx, db, c); err != nil {
		t.Fatal(err)
	}
	if err := ddl.DropCollection(ctx, db, tbl); err != nil {
		t.Fatalf("DropCollection: %v", err)
	}
	tables, _ := db.ListCollections(ctx, nil)
	for _, t2 := range tables {
		if t2.Name() == tbl {
			t.Errorf("%q still listed after DropCollection", tbl)
		}
	}
	// IfExists tolerates absent table.
	if err := ddl.DropCollection(ctx, db, tbl+"_nonexistent", ddl.IfExists()); err != nil {
		t.Errorf("DropCollection IfExists on missing table: %v", err)
	}
}

func TestAlterCollection_AddDropRenameField(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "alter")
	dropTable(t, db, tbl)
	defer dropTable(t, db, tbl)

	c := dbschema.CollectionDef{
		Name: tbl,
		Fields: []dbschema.FieldDef{
			{Name: dal.FieldName("id"), Type: dbschema.String, Nullable: false},
			{Name: dal.FieldName("email"), Type: dbschema.String, Nullable: true},
		},
		PrimaryKey: []dal.FieldName{"id"},
	}
	if err := ddl.CreateCollection(ctx, db, c); err != nil {
		t.Fatal(err)
	}

	err := ddl.AlterCollection(ctx, db, tbl,
		ddl.AddField(dbschema.FieldDef{Name: dal.FieldName("age"), Type: dbschema.Int, Nullable: true}),
		ddl.RenameField(dal.FieldName("email"), dal.FieldName("email_address")),
		ddl.DropField(dal.FieldName("age")),
	)
	if err != nil {
		t.Fatalf("AlterCollection: %v", err)
	}

	ref := dal.NewRootCollectionRef(tbl, "")
	got, err := db.DescribeCollection(ctx, &ref)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, f := range got.Fields {
		names[string(f.Name)] = true
	}
	if !names["id"] || !names["email_address"] {
		t.Errorf("expected fields id + email_address, got %+v", names)
	}
	if names["email"] || names["age"] {
		t.Errorf("unexpected residual fields after alter; got %+v", names)
	}
}

func TestAlterCollection_ModifyField(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "mod")
	dropTable(t, db, tbl)
	defer dropTable(t, db, tbl)

	c := dbschema.CollectionDef{
		Name: tbl,
		Fields: []dbschema.FieldDef{
			{Name: dal.FieldName("id"), Type: dbschema.String, Nullable: false},
			{Name: dal.FieldName("qty"), Type: dbschema.Int, Nullable: true},
		},
		PrimaryKey: []dal.FieldName{"id"},
	}
	if err := ddl.CreateCollection(ctx, db, c); err != nil {
		t.Fatal(err)
	}

	// Change qty from Int (BIGINT) to Float (DOUBLE).
	err := ddl.AlterCollection(ctx, db, tbl,
		ddl.ModifyField(dal.FieldName("qty"),
			dbschema.FieldDef{Name: dal.FieldName("qty"), Type: dbschema.Float, Nullable: true}),
	)
	if err != nil {
		t.Fatalf("AlterCollection ModifyField: %v", err)
	}

	ref := dal.NewRootCollectionRef(tbl, "")
	got, err := db.DescribeCollection(ctx, &ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got.Fields {
		if f.Name == dal.FieldName("qty") && f.Type != dbschema.Float {
			t.Errorf("qty Type = %v after modify, want Float", f.Type)
		}
	}
}

func TestAlterCollection_AddDropIndex(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	tbl := uniqueTable(t, "aidx")
	dropTable(t, db, tbl)
	defer dropTable(t, db, tbl)

	c := dbschema.CollectionDef{
		Name: tbl,
		Fields: []dbschema.FieldDef{
			{Name: dal.FieldName("id"), Type: dbschema.String, Nullable: false},
			{Name: dal.FieldName("email"), Type: dbschema.String, Nullable: true},
		},
		PrimaryKey: []dal.FieldName{"id"},
	}
	if err := ddl.CreateCollection(ctx, db, c); err != nil {
		t.Fatal(err)
	}

	idxName := tbl + "_email_idx"
	addIdx := dbschema.IndexDef{Name: idxName, Collection: tbl, Fields: []dal.FieldName{"email"}}
	if err := ddl.AlterCollection(ctx, db, tbl, ddl.AddIndex(addIdx), ddl.DropIndex(idxName)); err != nil {
		t.Fatalf("AlterCollection add+drop index: %v", err)
	}

	ref := dal.NewRootCollectionRef(tbl, "")
	idxs, _ := db.ListIndexes(ctx, &ref)
	for _, idx := range idxs {
		if idx.Name == idxName {
			t.Errorf("index %q still present after drop", idxName)
		}
	}
}

func TestSchemaModifier_Unit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("create_collection_success_and_default_idx_collection", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, nil)
		c := dbschema.CollectionDef{
			Name:       "users",
			Fields:     []dbschema.FieldDef{{Name: "id", Type: dbschema.String}},
			PrimaryKey: []dal.FieldName{"id"},
			Indexes: []dbschema.IndexDef{
				{Name: "ix_id", Collection: "", Fields: []dal.FieldName{"id"}},
			},
		}
		if err := db.CreateCollection(ctx, c, ddl.IfNotExists()); err != nil {
			t.Fatalf("CreateCollection: %v", err)
		}
	})

	t.Run("create_collection_invalid_table", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, nil)
		if err := db.CreateCollection(ctx, dbschema.CollectionDef{Name: ""}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("create_collection_invalid_index", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, nil)
		c := dbschema.CollectionDef{
			Name:    "users",
			Fields:  []dbschema.FieldDef{{Name: "id", Type: dbschema.String}},
			Indexes: []dbschema.IndexDef{{Name: "", Fields: []dal.FieldName{"id"}}},
		}
		if err := db.CreateCollection(ctx, c); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("create_collection_exec_table_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			execHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error, bool) {
				return nil, errors.New("create table failed"), true
			},
		})
		c := dbschema.CollectionDef{
			Name:   "users",
			Fields: []dbschema.FieldDef{{Name: "id", Type: dbschema.String}},
		}
		if err := db.CreateCollection(ctx, c); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("create_collection_exec_index_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			execHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error, bool) {
				if strings.HasPrefix(query, "CREATE INDEX") {
					return nil, errors.New("create index failed"), true
				}
				return driver.RowsAffected(0), nil, true
			},
		})
		c := dbschema.CollectionDef{
			Name:    "users",
			Fields:  []dbschema.FieldDef{{Name: "id", Type: dbschema.String}},
			Indexes: []dbschema.IndexDef{{Name: "ix_id", Fields: []dal.FieldName{"id"}}},
		}
		if err := db.CreateCollection(ctx, c); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("drop_collection_success_and_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, nil)
		if err := db.DropCollection(ctx, "users", ddl.IfExists()); err != nil {
			t.Fatalf("DropCollection: %v", err)
		}

		_, errDB := newMockDB(t, &driverHooks{
			execHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error, bool) {
				return nil, errors.New("drop failed"), true
			},
		})
		if err := errDB.DropCollection(ctx, "users"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("alter_collection_happy_path_all_ops", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, nil)
		err := db.AlterCollection(ctx, "users",
			ddl.AddField(dbschema.FieldDef{Name: "age", Type: dbschema.Int, Nullable: true}),
			ddl.RenameField("age", "years"),
			ddl.DropField("years"),
			ddl.AddIndex(dbschema.IndexDef{Name: "ix_email", Collection: "", Fields: []dal.FieldName{"email"}}),
			ddl.DropIndex("ix_email"),
			ddl.ModifyField("email", dbschema.FieldDef{Name: "email", Type: dbschema.String, Nullable: false}),
			ddl.ModifyField("email", dbschema.FieldDef{Name: "email", Type: dbschema.String, Nullable: true}),
		)
		if err != nil {
			t.Fatalf("AlterCollection: %v", err)
		}
	})

	t.Run("alter_collection_op_errors", func(t *testing.T) {
		t.Parallel()
		_, okDB := newMockDB(t, nil)
		// AddField invalid type
		if err := okDB.AlterCollection(ctx, "users", ddl.AddField(dbschema.FieldDef{Name: "bad", Type: dbschema.Null})); err == nil {
			t.Fatal("expected error")
		}
		// AddIndex invalid index
		if err := okDB.AlterCollection(ctx, "users", ddl.AddIndex(dbschema.IndexDef{Name: ""})); err == nil {
			t.Fatal("expected error")
		}
		// ModifyField invalid table name
		if err := okDB.AlterCollection(ctx, "bad-table", ddl.ModifyField("col", dbschema.FieldDef{Type: dbschema.Int})); err == nil {
			t.Fatal("expected error")
		}
		// ModifyField invalid field name
		if err := okDB.AlterCollection(ctx, "users", ddl.ModifyField("bad-col", dbschema.FieldDef{Type: dbschema.Int})); err == nil {
			t.Fatal("expected error")
		}
		// ModifyField invalid type
		if err := okDB.AlterCollection(ctx, "users", ddl.ModifyField("col", dbschema.FieldDef{Type: dbschema.Null})); err == nil {
			t.Fatal("expected error")
		}

		// Exec errors for each op
		_, failDB := newMockDB(t, &driverHooks{
			execHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error, bool) {
				return nil, errors.New("exec failed"), true
			},
		})
		ops := []ddl.AlterOp{
			ddl.AddField(dbschema.FieldDef{Name: "age", Type: dbschema.Int}),
			ddl.DropField("age"),
			ddl.RenameField("age", "years"),
			ddl.AddIndex(dbschema.IndexDef{Name: "ix_age", Collection: "users", Fields: []dal.FieldName{"age"}}),
			ddl.DropIndex("ix_age"),
			ddl.ModifyField("age", dbschema.FieldDef{Name: "age", Type: dbschema.Int}),
		}
		for i, op := range ops {
			if err := failDB.AlterCollection(ctx, "users", op); err == nil {
				t.Errorf("op %d: expected exec error, got nil", i)
			}
		}
	})
}
