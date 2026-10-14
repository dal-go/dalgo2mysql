package dalgo2mysql

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

func TestListConstraints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ref := dal.NewRootCollectionRef("orders", "")

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"constraint_name", "constraint_type"},
					rows: [][]driver.Value{
						{"PRIMARY", "PRIMARY KEY"},
						{"uq_email", "UNIQUE"},
						{"fk_user", "FOREIGN KEY"},
						{"chk_custom", "CHECK"},
					},
				}, nil, true
			},
		})
		got, err := db.ListConstraints(ctx, &ref)
		if err != nil {
			t.Fatalf("ListConstraints: %v", err)
		}
		want := []dbschema.ConstraintDef{
			{Name: "PRIMARY", Type: "primary-key"},
			{Name: "uq_email", Type: "unique"},
			{Name: "fk_user", Type: "foreign-key"},
			{Name: "chk_custom", Type: "CHECK"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return nil, errors.New("query failed"), true
			},
		})
		if _, err := db.ListConstraints(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("scan_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"constraint_name", "constraint_type"},
					rows: [][]driver.Value{{nil, "UNIQUE"}},
				}, nil, true
			},
		})
		if _, err := db.ListConstraints(ctx, &ref); err == nil {
			t.Fatal("expected scan error")
		}
	})
}

func TestListReferrers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ref := dal.NewRootCollectionRef("users", "")

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"referrer_table", "referrer_col"},
					rows: [][]driver.Value{
						{"orders", "user_id"},
						{"orders", "reviewer_id"},
						{"audits", "actor_id"},
					},
				}, nil, true
			},
		})
		got, err := db.ListReferrers(ctx, &ref)
		if err != nil {
			t.Fatalf("ListReferrers: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		if got[0].Collection.Name() != "orders" || !reflect.DeepEqual(got[0].Fields, []dal.FieldName{"user_id", "reviewer_id"}) {
			t.Errorf("got[0] = %+v", got[0])
		}
		if got[1].Collection.Name() != "audits" || !reflect.DeepEqual(got[1].Fields, []dal.FieldName{"actor_id"}) {
			t.Errorf("got[1] = %+v", got[1])
		}
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return nil, errors.New("query failed"), true
			},
		})
		if _, err := db.ListReferrers(ctx, &ref); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("scan_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols: []string{"referrer_table", "referrer_col"},
					rows: [][]driver.Value{{nil, "user_id"}},
				}, nil, true
			},
		})
		if _, err := db.ListReferrers(ctx, &ref); err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("rows_error", func(t *testing.T) {
		t.Parallel()
		_, db := newMockDB(t, &driverHooks{
			queryHook: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool) {
				return &mockRows{
					cols:    []string{"referrer_table", "referrer_col"},
					rows:    [][]driver.Value{{"orders", "user_id"}},
					nextErr: errors.New("rows iteration error"),
				}, nil, true
			},
		})
		if _, err := db.ListReferrers(ctx, &ref); err == nil {
			t.Fatal("expected rows error")
		}
	})
}
