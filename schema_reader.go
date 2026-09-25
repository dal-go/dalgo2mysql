package dalgo2mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	dalrecord "github.com/dal-go/record"
)

// ListCollections returns user-defined base tables in the connected database
// (scoped to DATABASE()) in alphabetical order. The parent *dalrecord.Key is
// ignored — MySQL has a flat table namespace within a schema.
func (d *Database) ListCollections(ctx context.Context, parent *dalrecord.Key) ([]dal.CollectionRef, error) {
	_ = parent // ignored
	rows, err := d.sqlDB.QueryContext(ctx,
		`SELECT table_name
		 FROM information_schema.tables
		 WHERE table_schema = DATABASE()
		   AND table_type = 'BASE TABLE'
		 ORDER BY table_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: ListCollections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []dal.CollectionRef
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return nil, fmt.Errorf("dalgo2mysql: ListCollections scan: %w", scanErr)
		}
		out = append(out, dal.NewRootCollectionRef(name, ""))
	}
	return out, rows.Err()
}

// DescribeCollection returns the full schema definition for the named table.
// It queries information_schema for columns and primary-key membership.
func (d *Database) DescribeCollection(ctx context.Context, ref *dal.CollectionRef) (*dbschema.CollectionDef, error) {
	return describeCollectionImpl(ctx, d.sqlDB, ref.Name())
}

// ListIndexes returns the non-primary-key indexes on the named table via
// information_schema.statistics.
func (d *Database) ListIndexes(ctx context.Context, ref *dal.CollectionRef) ([]dbschema.IndexDef, error) {
	return listIndexesImpl(ctx, d.sqlDB, ref.Name())
}

// ---- DescribeCollection impl ----

// describeCollectionImpl is the inner reader, factored so tests can reuse it.
func describeCollectionImpl(ctx context.Context, db *sql.DB, name string) (*dbschema.CollectionDef, error) {
	// 1. Confirm the table exists.
	var found string
	probeErr := db.QueryRowContext(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' AND table_name = ?`,
		name,
	).Scan(&found)
	if probeErr == sql.ErrNoRows {
		return nil, newCollectionNotFoundError(name)
	}
	if probeErr != nil {
		return nil, fmt.Errorf("dalgo2mysql: DescribeCollection probe %q: %w", name, probeErr)
	}

	// 2. Enumerate primary key columns.
	pkCols, err := listPrimaryKeyColumns(ctx, db, name)
	if err != nil {
		return nil, err
	}
	pkSet := make(map[string]bool, len(pkCols))
	for _, c := range pkCols {
		pkSet[c] = true
	}

	// 3. Enumerate columns from information_schema (including numeric precision/scale
	//    and character max length for proper type round-trip).
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, data_type,
		        character_maximum_length, numeric_precision, numeric_scale,
		        is_nullable
		 FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = ?
		 ORDER BY ordinal_position`,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: DescribeCollection columns %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	var fields []dbschema.FieldDef
	for rows.Next() {
		var (
			colName    string
			dataType   string
			charMaxLen sql.NullInt64
			numPrec    sql.NullInt64
			numScale   sql.NullInt64
			isNullable string
		)
		if scanErr := rows.Scan(&colName, &dataType, &charMaxLen, &numPrec, &numScale, &isNullable); scanErr != nil {
			return nil, fmt.Errorf("dalgo2mysql: DescribeCollection column scan: %w", scanErr)
		}

		t, ok := dbschemaTypeFromMySQL(dataType)
		if !ok {
			return nil, &dbschema.NotSupportedError{
				Op:      "DescribeCollection",
				Backend: "dalgo2mysql",
				Reason:  fmt.Sprintf("column %q has unrecognized MySQL type %q", colName, dataType),
			}
		}

		// Derive precision from numeric_precision/scale columns for DECIMAL types.
		var precision *dbschema.Precision
		if numPrec.Valid && numScale.Valid && t == dbschema.Decimal {
			precision = &dbschema.Precision{
				Total: int(numPrec.Int64),
				Scale: int(numScale.Int64),
			}
		}

		var fieldLength *int
		if t == dbschema.String && charMaxLen.Valid {
			n := int(charMaxLen.Int64)
			fieldLength = &n
		}

		nullable := isNullable == "YES" && !pkSet[colName]
		f := dbschema.FieldDef{
			Name:      dal.FieldName(colName),
			Type:      t,
			Precision: precision,
			Length:    fieldLength,
			Nullable:  nullable,
		}
		fields = append(fields, f)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("dalgo2mysql: DescribeCollection rows: %w", rowsErr)
	}

	// 4. Build PrimaryKey slice in ordinal order.
	pk := make([]dal.FieldName, len(pkCols))
	for i, c := range pkCols {
		pk[i] = dal.FieldName(c)
	}

	indexes, err := listIndexesImpl(ctx, db, name)
	if err != nil {
		return nil, err
	}
	foreignKeys, err := readForeignKeys(ctx, db, name)
	if err != nil {
		return nil, err
	}

	return &dbschema.CollectionDef{
		Name:        name,
		Fields:      fields,
		PrimaryKey:  pk,
		Indexes:     indexes,
		ForeignKeys: foreignKeys,
	}, nil
}

// listPrimaryKeyColumns returns the primary key column names in key ordinal order.
func listPrimaryKeyColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT kcu.column_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		   ON tc.constraint_name = kcu.constraint_name
		  AND tc.table_schema    = kcu.table_schema
		  AND tc.table_name      = kcu.table_name
		 WHERE tc.constraint_type = 'PRIMARY KEY'
		   AND tc.table_schema    = DATABASE()
		   AND tc.table_name      = ?
		 ORDER BY kcu.ordinal_position`,
		table,
	)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: listPrimaryKeyColumns %q: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var col string
		if scanErr := rows.Scan(&col); scanErr != nil {
			return nil, fmt.Errorf("dalgo2mysql: listPrimaryKeyColumns scan: %w", scanErr)
		}
		out = append(out, col)
	}
	return out, rows.Err()
}

// ---- ListIndexes impl ----

// listIndexesImpl returns non-primary-key indexes on the table via
// information_schema.statistics. The PRIMARY key (index_name = 'PRIMARY') is
// excluded. Multi-column indexes are assembled by grouping rows on index_name
// in seq_in_index order.
func listIndexesImpl(ctx context.Context, db *sql.DB, name string) ([]dbschema.IndexDef, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT index_name, non_unique, seq_in_index, column_name
		 FROM information_schema.statistics
		 WHERE table_schema = DATABASE()
		   AND table_name   = ?
		   AND index_name  <> 'PRIMARY'
		 ORDER BY index_name, seq_in_index`,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("dalgo2mysql: listIndexesImpl %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	byName := make(map[string]*dbschema.IndexDef)
	var order []string
	for rows.Next() {
		var (
			ixName    string
			nonUnique int
			seq       int
			colName   string
		)
		if scanErr := rows.Scan(&ixName, &nonUnique, &seq, &colName); scanErr != nil {
			return nil, fmt.Errorf("dalgo2mysql: listIndexesImpl scan: %w", scanErr)
		}
		idx, ok := byName[ixName]
		if !ok {
			idx = &dbschema.IndexDef{
				Name:       ixName,
				Collection: name,
				Unique:     nonUnique == 0,
			}
			byName[ixName] = idx
			order = append(order, ixName)
		}
		idx.Fields = append(idx.Fields, dal.FieldName(colName))
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("dalgo2mysql: listIndexesImpl rows: %w", rowsErr)
	}

	out := make([]dbschema.IndexDef, 0, len(order))
	for _, n := range order {
		out = append(out, *byName[n])
	}
	return out, nil
}
