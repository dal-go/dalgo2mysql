package dalgo2mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dalgotest"
	"github.com/dal-go/dalgo2sql"
	dalrecord "github.com/dal-go/record"
)

// The rule every read keeps: within one read, no two records have the same key (a record
// with no key, per dalgotest.HasNoKey, may repeat). These tests hold every read path of
// this package to it with dalgotest.AssertUniqueKeys.
//
// No MySQL server is needed: the rows come from this package's mock driver (the statement
// is rendered by dalgo2sql, the server's answer is scripted), with fixtures a constant key
// cannot pass by accident: hundreds of rows whose non-key values repeat.

const uniqueKeysRows = 412

// uniqueKeysRowsOf scripts n rows with the given column names; every non-key value repeats.
func uniqueKeysRowsOf(n int, cols ...string) *mockRows {
	rows := make([][]driver.Value, n)
	for i := range rows {
		row := make([]driver.Value, len(cols))
		for c, name := range cols {
			switch name {
			case "InvoiceId":
				row[c] = int64(i + 1)
			case "BillingCountry":
				row[c] = []string{"USA", "France", "Brazil"}[i%3]
			default:
				row[c] = fmt.Sprintf("same-%d", c)
			}
		}
		rows[i] = row
	}
	return &mockRows{cols: cols, rows: rows}
}

func ukSource(name, alias string) dal.FromSource {
	return dal.From(dal.NewRootCollectionRef(name, alias))
}

func ukColumn(name string) dal.Column {
	return dal.Column{Expression: dal.NewFieldRef("", name)}
}

func TestEveryReadKeepsItsKeysUniqueWithinTheRead(t *testing.T) {
	groupBy := ukSource("Invoice", "").NewQuery().GroupBy(dal.Field("BillingCountry")).
		SelectColumns(ukColumn("BillingCountry"), dal.Count())
	for _, tc := range []struct {
		name  string
		opts  dalgo2sql.DbOptions
		query dal.Query
		cols  []string
		rows  int
	}{
		{"select-all of a single-column primary key table", dalgo2sql.DbOptions{}, ukSource("Invoice", "").NewQuery().SelectColumns(),
			[]string{"InvoiceId", "CustomerId", "BillingCountry", "Total"}, uniqueKeysRows},
		{"select-all with the primary key declared", dalgo2sql.DbOptions{Recordsets: map[string]*dalgo2sql.Recordset{
			"Invoice": dalgo2sql.NewRecordset("Invoice", dalgo2sql.Table, []dal.FieldRef{dal.Field("InvoiceId")}),
		}}, ukSource("Invoice", "").NewQuery().SelectColumns(),
			[]string{"InvoiceId", "CustomerId", "BillingCountry", "Total"}, uniqueKeysRows},
		{"select columns without the key", dalgo2sql.DbOptions{}, ukSource("Invoice", "").NewQuery().SelectColumns(ukColumn("BillingCountry"), ukColumn("Total")),
			[]string{"BillingCountry", "Total"}, uniqueKeysRows},
		{"a composite or absent primary key", dalgo2sql.DbOptions{Recordsets: map[string]*dalgo2sql.Recordset{
			"CompositePk": dalgo2sql.NewRecordset("CompositePk", dalgo2sql.Table, []dal.FieldRef{dal.Field("Region"), dal.Field("Code")}),
			"NoPk":        dalgo2sql.NewRecordset("NoPk", dalgo2sql.Table, nil),
		}}, ukSource("CompositePk", "").NewQuery().SelectColumns(),
			[]string{"Region", "Code", "Label"}, 4},
		{"a table with no primary key and rows that are all alike", dalgo2sql.DbOptions{}, ukSource("NoPk", "").NewQuery().SelectColumns(),
			[]string{"Name", "Qty"}, 5},
		{"a view", dalgo2sql.DbOptions{}, ukSource("InvoiceView", "").NewQuery().SelectColumns(),
			[]string{"InvoiceId", "BillingCountry"}, uniqueKeysRows},
		// dal aggregates over the scripted source rows: three distinct countries, three groups.
		{"a GROUP BY", dalgo2sql.DbOptions{}, groupBy, []string{"BillingCountry", "Total"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, db := newMockDBWithOpts(t, &driverHooks{
				queryHook: func(context.Context, string, []driver.NamedValue) (driver.Rows, error, bool) {
					return uniqueKeysRowsOf(tc.rows, tc.cols...), nil, true
				},
			}, tc.opts)
			var records []dalrecord.Record
			err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
				reader, err := tx.ExecuteQueryToRecordsReader(ctx, tc.query)
				if err != nil {
					return err
				}
				defer func() { _ = reader.Close() }()
				for {
					r, err := reader.Next()
					if errors.Is(err, dal.ErrNoMoreRecords) {
						return nil
					}
					if err != nil {
						return err
					}
					records = append(records, r)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != tc.rows {
				t.Fatalf("read %d records, want %d", len(records), tc.rows)
			}
			dalgotest.AssertUniqueKeys(t, records)
			for i, r := range records {
				if dalgotest.HasNoKey(r.Key()) {
					t.Fatalf("record %d has no key", i)
				}
				if r.Key().ID == "__dalgo_record_id" {
					t.Fatalf("record %d has the retired constant key", i)
				}
			}
		})
	}
}

// A join is not a read path of the MySQL dialect yet: dalgo2sql plans joins only for SQLite,
// and says so, so no record is produced to be keyed.
func TestAJoinIsRefusedNotKeyedWrongly(t *testing.T) {
	join := ukSource("Customer", "c").Join(
		dal.NewJoinedSource(dal.NewRootCollectionRef("Invoice", "i"), dal.JoinInner,
			dal.NewComparison(dal.NewFieldRef("c", "CustomerId"), dal.Equal, dal.NewFieldRef("i", "CustomerId"))),
	).NewQuery().SelectColumns(ukColumn("Country"), ukColumn("Total"))
	_, db := newMockDB(t, &driverHooks{})
	err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
		reader, err := tx.ExecuteQueryToRecordsReader(ctx, join)
		if err == nil {
			_ = reader.Close()
		}
		return err
	})
	if err == nil {
		t.Fatal("a join was read, want a refusal")
	}
}
