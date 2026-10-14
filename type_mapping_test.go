package dalgo2mysql

import (
	"testing"

	"github.com/dal-go/dalgo/dbschema"
)

func TestMySQLTypeFor(t *testing.T) {
	t.Parallel()
	len50 := 50
	cases := []struct {
		name string
		in   dbschema.FieldDef
		want string
	}{
		{"string_default", dbschema.FieldDef{Type: dbschema.String}, "VARCHAR(255)"},
		{"string_custom_len", dbschema.FieldDef{Type: dbschema.String, Length: &len50}, "VARCHAR(50)"},
		{"int", dbschema.FieldDef{Type: dbschema.Int}, "BIGINT"},
		{"float", dbschema.FieldDef{Type: dbschema.Float}, "DOUBLE"},
		{"bool", dbschema.FieldDef{Type: dbschema.Bool}, "BOOLEAN"},
		{"time", dbschema.FieldDef{Type: dbschema.Time}, "DATETIME"},
		{"decimal_default", dbschema.FieldDef{Type: dbschema.Decimal}, "DECIMAL"},
		{"decimal_precision", dbschema.FieldDef{Type: dbschema.Decimal, Precision: &dbschema.Precision{Total: 12, Scale: 4}}, "DECIMAL(12, 4)"},
		{"bytes", dbschema.FieldDef{Type: dbschema.Bytes}, "BLOB"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := mysqlTypeFor(c.in)
			if err != nil {
				t.Fatalf("mysqlTypeFor(%+v): unexpected error: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("mysqlTypeFor(%+v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMySQLTypeFor_Errors(t *testing.T) {
	t.Parallel()
	if _, err := mysqlTypeFor(dbschema.FieldDef{Type: dbschema.Null}); err == nil {
		t.Error("expected error for dbschema.Null")
	}
	if _, err := mysqlTypeFor(dbschema.FieldDef{Type: dbschema.Type(127)}); err == nil {
		t.Error("expected error for unknown dbschema.Type")
	}
}

func TestDbschemaTypeFromMySQL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want dbschema.Type
		ok   bool
	}{
		{"varchar", dbschema.String, true},
		{"char", dbschema.String, true},
		{"text", dbschema.String, true},
		{"tinytext", dbschema.String, true},
		{"mediumtext", dbschema.String, true},
		{"longtext", dbschema.String, true},
		{"enum", dbschema.String, true},
		{"set", dbschema.String, true},
		{"bigint", dbschema.Int, true},
		{"int", dbschema.Int, true},
		{"integer", dbschema.Int, true},
		{"smallint", dbschema.Int, true},
		{"mediumint", dbschema.Int, true},
		{"year", dbschema.Int, true},
		{"tinyint", dbschema.Bool, true},
		{"double", dbschema.Float, true},
		{"float", dbschema.Float, true},
		{"real", dbschema.Float, true},
		{"bool", dbschema.Bool, true},
		{"boolean", dbschema.Bool, true},
		{"datetime", dbschema.Time, true},
		{"timestamp", dbschema.Time, true},
		{"date", dbschema.Time, true},
		{"time", dbschema.Time, true},
		{"decimal", dbschema.Decimal, true},
		{"numeric", dbschema.Decimal, true},
		{"dec", dbschema.Decimal, true},
		{"fixed", dbschema.Decimal, true},
		{"blob", dbschema.Bytes, true},
		{"tinyblob", dbschema.Bytes, true},
		{"mediumblob", dbschema.Bytes, true},
		{"longblob", dbschema.Bytes, true},
		{"binary", dbschema.Bytes, true},
		{"varbinary", dbschema.Bytes, true},
		{"geometry", dbschema.Null, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got, ok := dbschemaTypeFromMySQL(c.in)
			if ok != c.ok || got != c.want {
				t.Errorf("dbschemaTypeFromMySQL(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}
