# dalgo2mysql

MySQL-specific DALgo driver. Wraps `github.com/dal-go/dalgo2sql` to provide the `dal.DB` surface, and adds MySQL-native implementations of:

- `dbschema.SchemaReader` — schema introspection via `information_schema` scoped to the connected database (`DATABASE()`)
- `ddl.Applier` — MySQL-flavored `CREATE TABLE` / `CREATE INDEX` / `DROP TABLE` / `ALTER TABLE`
- `dal.ConcurrencyAware` — advertises `SupportsConcurrentConnections() = true`
  (MySQL supports concurrent connections from multiple goroutines and processes,
  unlike SQLite which serializes writers)

## Constructor usage

```go
import (
    "github.com/dal-go/dalgo/dal"
    "github.com/dal-go/dalgo2mysql"
    "github.com/dal-go/dalgo2sql"
)

// Minimal — no per-collection primary-key configuration.
db, err := dalgo2mysql.NewDatabase("user:pass@tcp(127.0.0.1:3306)/mydb?parseTime=true")

// With per-collection recordset options (required for Insert/Get/Delete with map[string]any data):
db, err := dalgo2mysql.NewDatabaseWithOptions(dsn, dal.NewSchema(nil, nil),
    dalgo2sql.DbOptions{
        Recordsets: map[string]*dalgo2sql.Recordset{
            "widgets": dalgo2sql.NewRecordset("widgets", dalgo2sql.Table,
                []dal.FieldRef{dal.Field("id")}),
        },
    })
if err != nil {
    return err
}
defer db.Close()
```

The DSN is in [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) format. Always include `?parseTime=true` so `DATETIME`/`TIMESTAMP` columns scan into `time.Time` rather than `[]byte`.

## Type mapping

| `dbschema.Type` | MySQL column type |
|-----------------|-------------------|
| `String`        | `VARCHAR(255)` (or `VARCHAR(n)` when `Length` is set) |
| `Int`           | `BIGINT` |
| `Float`         | `DOUBLE` |
| `Bool`          | `BOOLEAN` (stored as `tinyint(1)`) |
| `Time`          | `DATETIME` |
| `Decimal`       | `DECIMAL` (or `DECIMAL(total, scale)` when `Precision` is set) |
| `Bytes`         | `BLOB` |

`String` always maps to `VARCHAR`, never `TEXT`: a `VARCHAR(255)` can serve as a
`PRIMARY KEY` or index member under `utf8mb4` on InnoDB, whereas `TEXT`/`BLOB`
columns cannot be indexed without an explicit prefix length. This matters for
the conventional `id` primary key, which is a `String`.

On introspection MySQL reports `BOOLEAN` columns as `tinyint`; `dbschemaTypeFromMySQL`
maps `tinyint` back to `dbschema.Bool`.

## Running the tests

The tests that touch a live database are gated on the `DALGO2MYSQL_TEST_DSN`
environment variable. When it is not set, those tests are skipped so plain CI
passes without a database.

Start a local MySQL 8.4 server (Docker). `lower_case_table_names=0` keeps
identifiers case-sensitive and case-preserving:

```sh
docker run -d \
  --name mysql84 \
  -e MYSQL_USER=ovdb \
  -e MYSQL_PASSWORD=ovdb \
  -e MYSQL_DATABASE=ovdb \
  -e MYSQL_ROOT_PASSWORD=ovdb \
  -p 13306:3306 \
  mysql:8.4 --lower_case_table_names=0
```

Then run the tests:

```sh
DALGO2MYSQL_TEST_DSN='ovdb:ovdb@tcp(127.0.0.1:13306)/ovdb?parseTime=true' \
    go test ./... -count=1 -v
```

## MySQL driver: `github.com/go-sql-driver/mysql` (pure Go, `CGO_ENABLED=0`)

This package uses **`github.com/go-sql-driver/mysql`** — a pure-Go MySQL driver
exposed through the standard `database/sql` interface (driver name `"mysql"`).
No C toolchain is required; the package compiles with `CGO_ENABLED=0`.

### Placeholder dialect

MySQL uses `?` positional parameter markers, which is exactly `dalgo2sql`'s
default (`PlaceholderQuestion`). Unlike `dalgo2postgres` (which forces
`PlaceholderDollar` for `$1`, `$2`, …), `dalgo2mysql` leaves
`dalgo2sql.DbOptions.Placeholder` at its zero value.

### Identifier case: preserved, not folded

Unlike PostgreSQL — which folds unquoted identifiers to lower case —
MySQL with `lower_case_table_names=0` **preserves** identifier case: table
names are case-sensitive, and column names are case-insensitive but
case-preserving. `dalgo2mysql` back-tick quotes identifiers in DDL (escaping
embedded back-ticks by doubling) but does **not** lower-case them, so the bare
identifiers that `dalgo2sql`'s DML and dalgo's structured-query rendering emit
already match the back-tick-quoted DDL. No case-folding workaround is needed.

### DDL is non-transactional

MySQL implicitly commits the current transaction before and after every DDL
statement (`CREATE` / `DROP` / `ALTER`), so schema changes **cannot** be rolled
back atomically. `SupportsTransactionalDDL()` returns `false` (this differs from
`dalgo2postgres`, which returns `true`). A multi-statement `CreateCollection` or
`AlterCollection` that fails midway leaves earlier statements applied; callers
that need clean recovery should drop the affected table.
```
