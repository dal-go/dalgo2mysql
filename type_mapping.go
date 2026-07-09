package dalgo2mysql

import (
	"fmt"
	"strings"

	"github.com/dal-go/dalgo/dbschema"
)

// defaultVarcharLen is the VARCHAR length used for String columns without an
// explicit Length. 255 keeps utf8mb4 indexes within InnoDB limits on MySQL 8.
const defaultVarcharLen = 255

// mysqlTypeFor returns the MySQL column-type keyword for the given
// dbschema.FieldDef.  Length and Precision annotations are honoured:
//
//   - String   → VARCHAR(255) (default) or VARCHAR(n) when Length > 0
//   - Int      → BIGINT
//   - Float    → DOUBLE
//   - Bool     → BOOLEAN (stored as tinyint(1))
//   - Time     → DATETIME
//   - Decimal  → DECIMAL(10,0) (default) or DECIMAL(total, scale) when Precision != nil
//   - Bytes    → BLOB
//
// String columns always map to VARCHAR rather than TEXT: a VARCHAR can serve
// as a PRIMARY KEY or index member, whereas TEXT/BLOB columns cannot be
// indexed without an explicit prefix length. VARCHAR(255) is safely indexable
// under utf8mb4 on InnoDB.
func mysqlTypeFor(f dbschema.FieldDef) (string, error) {
	switch f.Type {
	case dbschema.String:
		if f.Length != nil && *f.Length > 0 {
			return fmt.Sprintf("VARCHAR(%d)", *f.Length), nil
		}
		return fmt.Sprintf("VARCHAR(%d)", defaultVarcharLen), nil
	case dbschema.Int:
		return "BIGINT", nil
	case dbschema.Float:
		return "DOUBLE", nil
	case dbschema.Bool:
		return "BOOLEAN", nil
	case dbschema.Time:
		return "DATETIME", nil
	case dbschema.Decimal:
		if f.Precision != nil {
			return fmt.Sprintf("DECIMAL(%d, %d)", f.Precision.Total, f.Precision.Scale), nil
		}
		return "DECIMAL", nil
	case dbschema.Bytes:
		return "BLOB", nil
	case dbschema.Null:
		return "", fmt.Errorf("dalgo2mysql: dbschema.Null is not a valid column type")
	default:
		return "", fmt.Errorf("dalgo2mysql: unknown dbschema.Type %v", f.Type)
	}
}

// dbschemaTypeFromMySQL maps information_schema.columns.data_type to a
// dbschema.Type.  MySQL returns the base type name in data_type (e.g.
// "varchar", "bigint", "double", "tinyint", "datetime", "decimal", "blob"),
// with length/precision reported separately in
// character_maximum_length / numeric_precision / numeric_scale.  The mapping
// is lossy by design: the round-trip is sufficient for schema inspection and
// migration, not for exact DDL replay.
//
// Note: MySQL BOOLEAN is a synonym for tinyint(1), so data_type comes back as
// "tinyint" and is mapped to dbschema.Bool.
func dbschemaTypeFromMySQL(dataType string) (dbschema.Type, bool) {
	switch strings.ToLower(strings.TrimSpace(dataType)) {
	case "varchar", "char", "text", "tinytext", "mediumtext", "longtext", "enum", "set":
		return dbschema.String, true
	case "bigint", "int", "integer", "smallint", "mediumint", "year":
		return dbschema.Int, true
	case "tinyint":
		// MySQL BOOLEAN is stored as tinyint(1).
		return dbschema.Bool, true
	case "double", "float", "real":
		return dbschema.Float, true
	case "bool", "boolean":
		return dbschema.Bool, true
	case "datetime", "timestamp", "date", "time":
		return dbschema.Time, true
	case "decimal", "numeric", "dec", "fixed":
		return dbschema.Decimal, true
	case "blob", "tinyblob", "mediumblob", "longblob", "binary", "varbinary":
		return dbschema.Bytes, true
	}
	return dbschema.Null, false
}
