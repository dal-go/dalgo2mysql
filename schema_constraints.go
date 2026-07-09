package dalgo2mysql

import (
	"context"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// ListConstraints returns a best-effort survey of constraints on the table via
// information_schema:
//   - PRIMARY KEY constraint (one row if any PK columns exist)
//   - UNIQUE constraints
//   - FOREIGN KEY constraints
//
// CHECK constraints are NOT enumerated here; read them from DescribeCollection
// if needed.
func (d *Database) ListConstraints(ctx context.Context, ref *dal.CollectionRef) ([]dbschema.ConstraintDef, error) {
	rows, err := d.sqlDB.QueryContext(ctx,
		`SELECT constraint_name, constraint_type
		 FROM information_schema.table_constraints
		 WHERE table_schema = DATABASE()
		   AND table_name   = ?
		   AND constraint_type IN ('PRIMARY KEY', 'UNIQUE', 'FOREIGN KEY')
		 ORDER BY constraint_type, constraint_name`,
		ref.Name(),
	)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: ListConstraints %q: %w", ref.Name(), err)
	}
	defer func() { _ = rows.Close() }()

	var out []dbschema.ConstraintDef
	for rows.Next() {
		var name, ctype string
		if scanErr := rows.Scan(&name, &ctype); scanErr != nil {
			return nil, fmt.Errorf("dalgo2mysql: ListConstraints scan: %w", scanErr)
		}
		var ct string
		switch ctype {
		case "PRIMARY KEY":
			ct = "primary-key"
		case "UNIQUE":
			ct = "unique"
		case "FOREIGN KEY":
			ct = "foreign-key"
		default:
			ct = ctype
		}
		out = append(out, dbschema.ConstraintDef{Name: name, Type: ct})
	}
	return out, rows.Err()
}

// ListReferrers returns the collections that reference ref via foreign keys.
// It queries information_schema.key_column_usage for rows whose
// referenced_table_name is the named table.
func (d *Database) ListReferrers(ctx context.Context, ref *dal.CollectionRef) ([]dbschema.Referrer, error) {
	rows, err := d.sqlDB.QueryContext(ctx,
		`SELECT DISTINCT table_name AS referrer_table, column_name AS referrer_col
		 FROM information_schema.key_column_usage
		 WHERE constraint_schema      = DATABASE()
		   AND referenced_table_name  = ?
		 ORDER BY referrer_table, referrer_col`,
		ref.Name(),
	)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: ListReferrers %q: %w", ref.Name(), err)
	}
	defer func() { _ = rows.Close() }()

	byTable := make(map[string][]dal.FieldName)
	var order []string
	for rows.Next() {
		var tbl, col string
		if scanErr := rows.Scan(&tbl, &col); scanErr != nil {
			return nil, fmt.Errorf("dalgo2mysql: ListReferrers scan: %w", scanErr)
		}
		if _, ok := byTable[tbl]; !ok {
			order = append(order, tbl)
		}
		byTable[tbl] = append(byTable[tbl], dal.FieldName(col))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]dbschema.Referrer, 0, len(order))
	for _, tbl := range order {
		out = append(out, dbschema.Referrer{
			Collection: dal.NewRootCollectionRef(tbl, ""),
			Fields:     byTable[tbl],
		})
	}
	return out, nil
}
