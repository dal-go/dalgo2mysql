package dalgo2mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo2sql"
	dalrecord "github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

func TestDatabase_DAL_UnimplementedWhenNilBackend(t *testing.T) {
	t.Parallel()
	d := &Database{}
	ctx := context.Background()
	key := dalrecord.NewKeyWithID("widgets", "w1")
	rec := dalrecord.NewRecordWithData(key, map[string]any{"name": "test"})

	if err := d.Set(ctx, rec); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("Set = %v, want ErrNotImplementedYet", err)
	}
	if err := d.SetMulti(ctx, []dalrecord.Record{rec}); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("SetMulti = %v, want ErrNotImplementedYet", err)
	}
	if err := d.Insert(ctx, rec); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("Insert = %v, want ErrNotImplementedYet", err)
	}
	if err := d.Upsert(ctx, rec); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("Upsert = %v, want ErrNotImplementedYet", err)
	}
	if err := d.Delete(ctx, key); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("Delete = %v, want ErrNotImplementedYet", err)
	}
	if err := d.DeleteMulti(ctx, []*dalrecord.Key{key}); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("DeleteMulti = %v, want ErrNotImplementedYet", err)
	}
	if err := d.Update(ctx, key, nil); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("Update = %v, want ErrNotImplementedYet", err)
	}
	if err := d.UpdateMulti(ctx, []*dalrecord.Key{key}, nil); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("UpdateMulti = %v, want ErrNotImplementedYet", err)
	}
	if err := d.UpdateRecord(ctx, rec, nil); !errors.Is(err, dal.ErrNotImplementedYet) {
		t.Errorf("UpdateRecord = %v, want ErrNotImplementedYet", err)
	}
}

func TestDatabase_DAL_Operations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, db := newMockDBWithOpts(t, &driverHooks{
		queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
			return &mockRows{
				cols: []string{"id", "name", "price"},
				rows: [][]driver.Value{{"w1", "Sprocket", "1.23"}},
			}, nil, true
		},
	}, dalgo2sql.DbOptions{
		Recordsets: map[string]*dalgo2sql.Recordset{
			"widgets": dalgo2sql.NewRecordset("widgets", dalgo2sql.Table,
				[]dal.FieldRef{dal.Field("id")}),
		},
	})

	key1 := dalrecord.NewKeyWithID("widgets", "w1")
	rec1 := dalrecord.NewRecordWithData(key1, map[string]any{"name": "Sprocket", "price": "1.23"})

	if err := db.Insert(ctx, rec1); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	getRec1 := dalrecord.NewRecordWithData(key1, make(map[string]any))
	if err := db.Get(ctx, getRec1); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if exists, err := db.Exists(ctx, key1); err != nil || !exists {
		t.Fatalf("Exists = (%v, %v), want (true, nil)", exists, err)
	}

	if err := db.Set(ctx, rec1); err != nil {
		t.Fatalf("Set: %v", err)
	}

	key2 := dalrecord.NewKeyWithID("widgets", "w2")
	rec2 := dalrecord.NewRecordWithData(key2, map[string]any{"name": "Nut", "price": "0.50"})
	if err := db.SetMulti(ctx, []dalrecord.Record{rec2}); err != nil {
		t.Fatalf("SetMulti: %v", err)
	}

	getRec2 := dalrecord.NewRecordWithData(key2, make(map[string]any))
	if err := db.GetMulti(ctx, []dalrecord.Record{getRec1, getRec2}); err != nil {
		t.Fatalf("GetMulti: %v", err)
	}

	if err := db.Upsert(ctx, rec1); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := db.Update(ctx, key1, []update.Update{update.ByFieldName("price", "3.45")}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if err := db.UpdateMulti(ctx, []*dalrecord.Key{key1, key2}, []update.Update{update.ByFieldName("price", "4.56")}); err != nil {
		t.Fatalf("UpdateMulti: %v", err)
	}

	collRef := dal.NewRootCollectionRef("widgets", "")
	q := dal.NewQueryBuilder(dal.From(&collRef)).SelectIntoRecordset()
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	if err != nil {
		t.Fatalf("ExecuteQueryToRecordsReader: %v", err)
	}
	_ = reader.Close()

	rsReader, err := db.ExecuteQueryToRecordsetReader(ctx, q)
	if err != nil {
		t.Fatalf("ExecuteQueryToRecordsetReader: %v", err)
	}
	_ = rsReader.Close()

	if err := db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return nil
	}); err != nil {
		t.Fatalf("RunReadonlyTransaction: %v", err)
	}

	if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return nil
	}); err != nil {
		t.Fatalf("RunReadwriteTransaction: %v", err)
	}

	if err := db.Delete(ctx, key1); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if err := db.DeleteMulti(ctx, []*dalrecord.Key{key2}); err != nil {
		t.Fatalf("DeleteMulti: %v", err)
	}
}
