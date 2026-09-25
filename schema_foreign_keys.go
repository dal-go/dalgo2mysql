package dalgo2mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

func readForeignKeys(ctx context.Context, db *sql.DB, table string) ([]dbschema.ForeignKeyDef, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: foreign keys connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	var checks int
	if err := conn.QueryRowContext(ctx, `SELECT @@SESSION.foreign_key_checks`).Scan(&checks); err != nil {
		return nil, fmt.Errorf("dalgo2mysql: foreign key checks: %w", err)
	}
	state := dbschema.ForeignKeyEnforcementDisabled
	if checks != 0 {
		state = dbschema.ForeignKeyEnforcementEnabled
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT k.constraint_name, k.column_name,
		       CASE WHEN k.referenced_table_schema = DATABASE() THEN '' ELSE k.referenced_table_schema END,
		       k.referenced_table_name, k.referenced_column_name,
		       rc.update_rule, rc.delete_rule
		FROM information_schema.key_column_usage AS k
		JOIN information_schema.referential_constraints AS rc
		  ON rc.constraint_schema = k.constraint_schema
		 AND rc.constraint_name = k.constraint_name AND rc.table_name = k.table_name
		WHERE k.table_schema = DATABASE() AND k.table_name = ?
		  AND k.referenced_table_name IS NOT NULL
		ORDER BY k.constraint_name, k.ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: foreign keys for %q: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var keys []dbschema.ForeignKeyDef
	for rows.Next() {
		var name, from, targetNamespace, target, to, onUpdate, onDelete string
		if err := rows.Scan(&name, &from, &targetNamespace, &target, &to, &onUpdate, &onDelete); err != nil {
			return nil, fmt.Errorf("dalgo2mysql: foreign key scan: %w", err)
		}
		if len(keys) == 0 || keys[len(keys)-1].Name != name {
			keys = append(keys, dbschema.ForeignKeyDef{Name: name, ReferencedCollection: target, ReferencedNamespace: targetNamespace, Enforcement: state, OnUpdate: onUpdate, OnDelete: onDelete})
		}
		key := &keys[len(keys)-1]
		key.Fields = append(key.Fields, dal.FieldName(from))
		key.ReferencedFields = append(key.ReferencedFields, dal.FieldName(to))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dalgo2mysql: foreign key rows: %w", err)
	}
	return keys, nil
}
