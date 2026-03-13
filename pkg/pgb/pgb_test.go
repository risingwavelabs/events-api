package pgb

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type execCall struct {
	sql  string
	args []any
}

type mockConn struct {
	mu    sync.Mutex
	calls []execCall
}

func (m *mockConn) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	argsCopy := make([]any, len(args))
	copy(argsCopy, args)
	m.calls = append(m.calls, execCall{sql: sql, args: argsCopy})

	return pgconn.CommandTag{}, nil
}

func (m *mockConn) Close() {}

func (m *mockConn) Calls() []execCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	calls := make([]execCall, len(m.calls))
	copy(calls, m.calls)
	return calls
}

func TestBuildInsertStatement_AppendsInsertStmtSuffix(t *testing.T) {
	cols := []Column{{Name: "id"}, {Name: "payload"}}
	items := []*Item{{rows: [][]any{{1, "a"}}}}

	sql, args := _buildInsertStatement(
		_buildPrepareSQL("events", cols),
		items,
		cols,
		"ON CONFLICT (id) DO NOTHING",
		"FLUSH",
	)

	require.Equal(t, "INSERT INTO events (id, payload) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING;FLUSH;", sql)
	require.Equal(t, []any{1, "a"}, args)
}

func TestFlush_UsesOperatorInsertStmtSuffix(t *testing.T) {
	conn := &mockConn{}
	cols := []Column{{Name: "id"}}
	items := []*Item{
		{rows: [][]any{{1}}, c: make(chan error, 1)},
		{rows: [][]any{{2}}, c: make(chan error, 1)},
	}

	op := &BulkInsertOperator{
		sql:              _buildPrepareSQL("events", cols),
		cols:             cols,
		endStmts:         "FLUSH",
		insertStmtSuffix: "ON CONFLICT DO NOTHING",
		buf:              items,
		conn:             conn,
		table:            "events",
		log:              zap.NewNop(),
	}

	op.flush(context.Background())

	for _, item := range items {
		require.NoError(t, waitForFlushResult(t, item.c))
	}

	calls := conn.Calls()
	require.Len(t, calls, 1)
	require.Equal(t, "INSERT INTO events (id) VALUES ($1), ($2) ON CONFLICT DO NOTHING;FLUSH;", calls[0].sql)
	require.Equal(t, []any{1, 2}, calls[0].args)
}

func waitForFlushResult(t *testing.T, c <-chan error) error {
	t.Helper()

	select {
	case err := <-c:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for flush result")
		return nil
	}
}
