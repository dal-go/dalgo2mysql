package dalgo2mysql

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/dal-go/dalgo/ddl"
)

// safeIdentRe matches identifier names that are safe to embed in SQL without
// further escaping: ASCII letters, digits, and underscores only.
// Reserved words, spaces, and special characters are intentionally excluded
// because they require back-tick quoting; all identifiers are always back-tick
// quoted by this package, so only the character-set check is enforced here.
var safeIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateIdent returns an error if name is empty or contains characters
// outside the safe ASCII identifier set.
func validateIdent(name string) error {
	if name == "" {
		return fmt.Errorf("dalgo2mysql: identifier cannot be empty")
	}
	if !safeIdentRe.MatchString(name) {
		return fmt.Errorf("dalgo2mysql: identifier %q contains characters not matching [A-Za-z_][A-Za-z0-9_]*", name)
	}
	return nil
}

// quoteIdent wraps name in back-ticks for MySQL, escaping any embedded
// back-tick by doubling it. Unlike PostgreSQL, MySQL does NOT fold unquoted
// identifiers to lower case: with lower_case_table_names=0 table names are
// case-sensitive and case-preserving, and column names are
// case-insensitive-but-preserving. So the bare identifiers that dalgo2sql's
// DML and dal's structured-query rendering emit already match the
// back-tick-quoted DDL this package produces — no case folding is required.
// Caller must have validated name first via validateIdent to prevent injection.
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func buildCreateTableSQL(c dbschema.CollectionDef, opts ddl.Options) (string, error) {
	if err := validateIdent(c.Name); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("CREATE TABLE ")
	if opts.IfNotExists {
		sb.WriteString("IF NOT EXISTS ")
	}
	sb.WriteString(quoteIdent(c.Name))
	sb.WriteString(" (")

	// Build a set of PK field names for quick lookup.
	pkSet := make(map[dal.FieldName]bool, len(c.PrimaryKey))
	for _, n := range c.PrimaryKey {
		pkSet[n] = true
	}

	parts := make([]string, 0, len(c.Fields)+1)
	for _, f := range c.Fields {
		colSQL, err := buildColumnDecl(f, pkSet[f.Name])
		if err != nil {
			return "", err
		}
		parts = append(parts, colSQL)
	}
	if len(c.PrimaryKey) > 0 {
		pkNames := make([]string, len(c.PrimaryKey))
		for i, n := range c.PrimaryKey {
			pkNames[i] = quoteIdent(string(n))
		}
		parts = append(parts, "PRIMARY KEY ("+strings.Join(pkNames, ", ")+")")
	}
	sb.WriteString(strings.Join(parts, ", "))
	sb.WriteString(")")
	return sb.String(), nil
}

// buildColumnDecl renders one column declaration with back-tick-quoted
// identifier and MySQL-specific type mapping.
// inPK: this field is a member of the primary key (always NOT NULL in MySQL).
func buildColumnDecl(f dbschema.FieldDef, inPK bool) (string, error) {
	if err := validateIdent(string(f.Name)); err != nil {
		return "", err
	}
	sqlType, err := mysqlTypeFor(f)
	if err != nil {
		return "", fmt.Errorf("dalgo2mysql: field %q: %w", f.Name, err)
	}
	parts := []string{quoteIdent(string(f.Name)), sqlType}
	if !f.Nullable || inPK {
		parts = append(parts, "NOT NULL")
	}
	return strings.Join(parts, " "), nil
}

func buildCreateIndexSQL(idx dbschema.IndexDef, opts ddl.Options) (string, error) {
	if idx.Name == "" {
		return "", fmt.Errorf("dalgo2mysql: index name cannot be empty")
	}
	if idx.Collection == "" {
		return "", fmt.Errorf("dalgo2mysql: index %q: collection cannot be empty", idx.Name)
	}
	if len(idx.Fields) == 0 {
		return "", fmt.Errorf("dalgo2mysql: index %q: must have at least one field", idx.Name)
	}
	if err := validateIdent(idx.Name); err != nil {
		return "", err
	}
	if err := validateIdent(idx.Collection); err != nil {
		return "", err
	}
	// MySQL does not support "CREATE INDEX IF NOT EXISTS"; opts.IfNotExists is
	// intentionally ignored for index creation.
	var sb strings.Builder
	sb.WriteString("CREATE ")
	if idx.Unique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX ")
	sb.WriteString(quoteIdent(idx.Name))
	sb.WriteString(" ON ")
	sb.WriteString(quoteIdent(idx.Collection))
	sb.WriteString(" (")
	cols := make([]string, len(idx.Fields))
	for i, n := range idx.Fields {
		cols[i] = quoteIdent(string(n))
	}
	sb.WriteString(strings.Join(cols, ", "))
	sb.WriteString(")")
	return sb.String(), nil
}

func buildDropTableSQL(name string, opts ddl.Options) string {
	if opts.IfExists {
		return "DROP TABLE IF EXISTS " + quoteIdent(name)
	}
	return "DROP TABLE " + quoteIdent(name)
}

// buildDropIndexSQL renders a DROP INDEX statement. MySQL requires the target
// table, so the collection name must be supplied. MySQL does not support
// "DROP INDEX IF EXISTS", so opts.IfExists is ignored.
func buildDropIndexSQL(name, table string, opts ddl.Options) string {
	return "DROP INDEX " + quoteIdent(name) + " ON " + quoteIdent(table)
}

func buildAlterTableAddColumnSQL(table string, f dbschema.FieldDef) (string, error) {
	colDecl, err := buildColumnDecl(f, false)
	if err != nil {
		return "", err
	}
	return "ALTER TABLE " + quoteIdent(table) + " ADD COLUMN " + colDecl, nil
}

func buildAlterTableDropColumnSQL(table string, col dal.FieldName) string {
	return "ALTER TABLE " + quoteIdent(table) + " DROP COLUMN " + quoteIdent(string(col))
}

func buildAlterTableRenameColumnSQL(table string, oldName, newName dal.FieldName) string {
	// MySQL 8.0+ supports RENAME COLUMN ... TO ... (no type respecification
	// required, unlike CHANGE COLUMN).
	return "ALTER TABLE " + quoteIdent(table) +
		" RENAME COLUMN " + quoteIdent(string(oldName)) +
		" TO " + quoteIdent(string(newName))
}
