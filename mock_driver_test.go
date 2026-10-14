package dalgo2mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo2sql"
)

type driverHooks struct {
	connectErr   error
	pingErr      error
	queryHook    func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error, bool)
	execHook     func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error, bool)
	beginHook    func(ctx context.Context, opts driver.TxOptions) (driver.Tx, error, bool)
	commitHook   func() (error, bool)
	rollbackHook func() (error, bool)
}

type mockConnector struct {
	hooks *driverHooks
}

func (c *mockConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if c.hooks != nil && c.hooks.connectErr != nil {
		return nil, c.hooks.connectErr
	}
	return &mockConn{hooks: c.hooks}, nil
}

func (c *mockConnector) Driver() driver.Driver {
	return &mockDriver{hooks: c.hooks}
}

type mockDriver struct {
	hooks *driverHooks
}

func (d *mockDriver) Open(name string) (driver.Conn, error) {
	return (&mockConnector{hooks: d.hooks}).Connect(context.Background())
}

type mockConn struct {
	hooks *driverHooks
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{conn: c, query: query}, nil
}

func (c *mockConn) Close() error {
	return nil
}

func (c *mockConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *mockConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.hooks != nil && c.hooks.beginHook != nil {
		if tx, err, ok := c.hooks.beginHook(ctx, opts); ok {
			return tx, err
		}
	}
	return &mockTx{hooks: c.hooks}, nil
}

func (c *mockConn) Ping(ctx context.Context) error {
	if c.hooks != nil && c.hooks.pingErr != nil {
		return c.hooks.pingErr
	}
	return nil
}

func (c *mockConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.hooks != nil && c.hooks.queryHook != nil {
		if rows, err, ok := c.hooks.queryHook(ctx, query, args); ok {
			return rows, err
		}
	}
	return &mockRows{cols: []string{"id"}}, nil
}

func (c *mockConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.hooks != nil && c.hooks.execHook != nil {
		if res, err, ok := c.hooks.execHook(ctx, query, args); ok {
			return res, err
		}
	}
	return driver.RowsAffected(1), nil
}

type mockStmt struct {
	conn  *mockConn
	query string
}

func (s *mockStmt) Close() error  { return nil }
func (s *mockStmt) NumInput() int { return -1 }

func (s *mockStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.ExecContext(context.Background(), s.query, valuesToNamed(args))
}

func (s *mockStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.QueryContext(context.Background(), s.query, valuesToNamed(args))
}

func valuesToNamed(args []driver.Value) []driver.NamedValue {
	nv := make([]driver.NamedValue, len(args))
	for i, v := range args {
		nv[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return nv
}

type mockTx struct {
	hooks *driverHooks
}

func (t *mockTx) Commit() error {
	if t.hooks != nil && t.hooks.commitHook != nil {
		if err, ok := t.hooks.commitHook(); ok {
			return err
		}
	}
	return nil
}

func (t *mockTx) Rollback() error {
	if t.hooks != nil && t.hooks.rollbackHook != nil {
		if err, ok := t.hooks.rollbackHook(); ok {
			return err
		}
	}
	return nil
}

type mockRows struct {
	cols     []string
	rows     [][]driver.Value
	pos      int
	nextErr  error
	closeErr error
}

func (m *mockRows) Columns() []string { return m.cols }
func (m *mockRows) Close() error      { return m.closeErr }
func (m *mockRows) Next(dest []driver.Value) error {
	if m.pos >= len(m.rows) {
		if m.nextErr != nil {
			return m.nextErr
		}
		return io.EOF
	}
	copy(dest, m.rows[m.pos])
	m.pos++
	return nil
}

func newMockDB(t *testing.T, hooks *driverHooks) (*sql.DB, *Database) {
	t.Helper()
	return newMockDBWithOpts(t, hooks, dalgo2sql.DbOptions{})
}

func newMockDBWithOpts(t *testing.T, hooks *driverHooks, opts dalgo2sql.DbOptions) (*sql.DB, *Database) {
	t.Helper()
	if opts.IsAlreadyExists == nil {
		opts.IsAlreadyExists = IsAlreadyExists
	}
	sqlDB := sql.OpenDB(&mockConnector{hooks: hooks})
	t.Cleanup(func() { _ = sqlDB.Close() })
	innerDB := dalgo2sql.NewDatabase(sqlDB, dal.NewSchema(nil, nil), opts)
	d := &Database{
		DB:    innerDB,
		sqlDB: sqlDB,
		dsn:   "mock",
	}
	return sqlDB, d
}
