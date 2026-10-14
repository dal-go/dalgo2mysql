package dalgo2mysql

import (
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/dal-go/dalgo/ddl"
)

func TestValidateAndQuoteIdent(t *testing.T) {
	t.Parallel()
	if err := validateIdent(""); err == nil {
		t.Error("expected error for empty identifier")
	}
	if err := validateIdent("bad-name"); err == nil {
		t.Error("expected error for invalid characters in identifier")
	}
	if err := validateIdent("valid_name_1"); err != nil {
		t.Errorf("unexpected error for valid identifier: %v", err)
	}
	if got := quoteIdent("a`b"); got != "`a``b`" {
		t.Errorf("quoteIdent = %q, want `a``b`", got)
	}
}

func TestBuildCreateTableSQL(t *testing.T) {
	t.Parallel()

	t.Run("simple_with_pk_and_if_not_exists", func(t *testing.T) {
		t.Parallel()
		c := dbschema.CollectionDef{
			Name: "users",
			Fields: []dbschema.FieldDef{
				{Name: dal.FieldName("id"), Type: dbschema.String, Nullable: false},
				{Name: dal.FieldName("age"), Type: dbschema.Int, Nullable: true},
			},
			PrimaryKey: []dal.FieldName{"id"},
		}
		got, err := buildCreateTableSQL(c, ddl.ResolveOptions(ddl.IfNotExists()))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "CREATE TABLE IF NOT EXISTS `users` (`id` VARCHAR(255) NOT NULL, `age` BIGINT, PRIMARY KEY (`id`))"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("invalid_table_name", func(t *testing.T) {
		t.Parallel()
		if _, err := buildCreateTableSQL(dbschema.CollectionDef{Name: ""}, ddl.Options{}); err == nil {
			t.Error("expected error for empty table name")
		}
	})

	t.Run("invalid_field_name", func(t *testing.T) {
		t.Parallel()
		c := dbschema.CollectionDef{
			Name:   "users",
			Fields: []dbschema.FieldDef{{Name: dal.FieldName("bad-col"), Type: dbschema.Int}},
		}
		if _, err := buildCreateTableSQL(c, ddl.Options{}); err == nil {
			t.Error("expected error for invalid field name")
		}
	})

	t.Run("invalid_field_type", func(t *testing.T) {
		t.Parallel()
		c := dbschema.CollectionDef{
			Name:   "users",
			Fields: []dbschema.FieldDef{{Name: dal.FieldName("col"), Type: dbschema.Null}},
		}
		if _, err := buildCreateTableSQL(c, ddl.Options{}); err == nil {
			t.Error("expected error for Null field type")
		}
	})
}

func TestBuildCreateIndexSQL(t *testing.T) {
	t.Parallel()

	t.Run("non_unique_and_unique", func(t *testing.T) {
		t.Parallel()
		idx := dbschema.IndexDef{
			Name:       "ix_users_email",
			Collection: "users",
			Fields:     []dal.FieldName{"email"},
		}
		got, err := buildCreateIndexSQL(idx, ddl.Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "CREATE INDEX `ix_users_email` ON `users` (`email`)" {
			t.Errorf("got %q", got)
		}

		idx.Unique = true
		gotUq, err := buildCreateIndexSQL(idx, ddl.Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotUq != "CREATE UNIQUE INDEX `ix_users_email` ON `users` (`email`)" {
			t.Errorf("got %q", gotUq)
		}
	})

	t.Run("validation_errors", func(t *testing.T) {
		t.Parallel()
		cases := []dbschema.IndexDef{
			{Name: "", Collection: "users", Fields: []dal.FieldName{"a"}},
			{Name: "ix", Collection: "", Fields: []dal.FieldName{"a"}},
			{Name: "ix", Collection: "users", Fields: nil},
			{Name: "bad-ix", Collection: "users", Fields: []dal.FieldName{"a"}},
			{Name: "ix", Collection: "bad-tbl", Fields: []dal.FieldName{"a"}},
		}
		for i, idx := range cases {
			if _, err := buildCreateIndexSQL(idx, ddl.Options{}); err == nil {
				t.Errorf("case %d (%+v): expected error, got nil", i, idx)
			}
		}
	})
}

func TestBuildDropAndAlterSQL(t *testing.T) {
	t.Parallel()

	if got := buildDropTableSQL("users", ddl.Options{}); got != "DROP TABLE `users`" {
		t.Errorf("got %q", got)
	}
	if got := buildDropTableSQL("users", ddl.ResolveOptions(ddl.IfExists())); got != "DROP TABLE IF EXISTS `users`" {
		t.Errorf("got %q", got)
	}
	if got := buildDropIndexSQL("ix_email", "users", ddl.Options{}); got != "DROP INDEX `ix_email` ON `users`" {
		t.Errorf("got %q", got)
	}

	addSQL, err := buildAlterTableAddColumnSQL("users", dbschema.FieldDef{Name: "age", Type: dbschema.Int, Nullable: true})
	if err != nil || !strings.Contains(addSQL, "ALTER TABLE `users` ADD COLUMN `age` BIGINT") {
		t.Errorf("buildAlterTableAddColumnSQL = (%q, %v)", addSQL, err)
	}
	if _, err := buildAlterTableAddColumnSQL("users", dbschema.FieldDef{Name: "bad", Type: dbschema.Null}); err == nil {
		t.Error("expected error for invalid column type in buildAlterTableAddColumnSQL")
	}

	if got := buildAlterTableDropColumnSQL("users", "age"); got != "ALTER TABLE `users` DROP COLUMN `age`" {
		t.Errorf("got %q", got)
	}
	if got := buildAlterTableRenameColumnSQL("users", "old_col", "new_col"); got != "ALTER TABLE `users` RENAME COLUMN `old_col` TO `new_col`" {
		t.Errorf("got %q", got)
	}
}
