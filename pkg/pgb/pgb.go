package pgb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

type Column struct {
	// Name Name of the column
	Name string `json:"name"`

	// Type Data type of the column
	Type string `json:"type"`
}

const (
	FlushInterval  = 500 * time.Millisecond
	DefaultBufSize = 5000
	MaxParamLimit  = 65535
)

var ErrBulkInsertClosed = errors.New("bulk insert operator is closed")

type Connection interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Close()
}

var BulkInsertError = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "events-api_rw_bulk_insert_error",
		Help: "The number of errors encountered during bulk insert operations",
	},
)

var BulkInsertFlushByTimeout = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "events-api_rw_bulk_insert_flush_by_timeout",
		Help: "The number of times the bulk insert buffer was flushed due to timeout",
	},
)

var BulkInsertFlushBySize = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "events-api_rw_bulk_insert_flush_by_size",
		Help: "The number of times the bulk insert buffer was flushed due to reaching max size",
	},
)

var FlushGoroutine = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "events-api_rw_bulk_insert_flush_goroutine",
		Help: "The number of active goroutines flushing the bulk insert buffer",
	},
)

var FlushSuccessCount = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "events-api_rw_bulk_insert_flush_success_count",
		Help: "The number of successful flush operations",
	},
)

var FlushErrCount = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "events-api_rw_bulk_insert_flush_error_count",
		Help: "The number of errors encountered during flush operations",
	},
)

var BulkInsertBackpressureHit = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "events-api_rw_bulk_insert_backpressure_hit",
		Help: "The number of times bulk insert backpressure was hit",
	},
)

type Item struct {
	rows [][]any
	c    chan error
}

type BulkInsertOperator struct {
	log      *zap.Logger
	sql      string
	table    string
	cols     []Column
	endStmts string

	itemPool sync.Pool

	c      chan *Item
	buf    []*Item
	rowCnt int
	conn   Connection

	bufSize int
	maxRows int

	closed    *atomic.Bool
	runCancel context.CancelFunc
	inFlight  *atomic.Int32
}

func newBulkInsertOperator(ctx context.Context, table string, cols []Column, conn Connection, bufSize int, log *zap.Logger, endStmts ...string) *BulkInsertOperator {
	ctx, cancel := context.WithCancel(ctx)

	o := &BulkInsertOperator{
		sql:      _buildPrepareSQL(table, cols),
		cols:     cols,
		endStmts: strings.Join(endStmts, "; "),
		buf:      make([]*Item, 0, bufSize),
		conn:     conn,
		c:        make(chan *Item, bufSize),
		bufSize:  bufSize,
		maxRows:  MaxParamLimit / len(cols),
		table:    table,
		log: log.Named("bulk_insert").With(
			zap.String("table", table),
		),
		itemPool: sync.Pool{
			New: func() any {
				return &Item{
					c: make(chan error, 1),
				}
			},
		},
		runCancel: cancel,
		closed:    &atomic.Bool{},
		inFlight:  &atomic.Int32{},
	}

	o.run(ctx)

	return o
}

func (o *BulkInsertOperator) Close() {
	o.runCancel()
}

func (o *BulkInsertOperator) releaseItem(item *Item) {
	item.rows = nil
	o.itemPool.Put(item)
}

func (o *BulkInsertOperator) Insert(ctx context.Context, rows [][]any) error {
	o.inFlight.Add(1)
	defer o.inFlight.Add(-1)

	if o.closed.Load() {
		return ErrBulkInsertClosed
	}

	item := o.itemPool.Get().(*Item)

	item.rows = rows

	select {
	case <-item.c: // drain previous error if any
	default:
	}

	select {
	case o.c <- item:
	default:
		BulkInsertBackpressureHit.Inc()
		o.releaseItem(item)
		return ErrInsertBackpressure
	}

	select {
	case err := <-item.c:
		o.releaseItem(item)
		return err
	case <-ctx.Done():
		go func() {
			// wait for the flush to avoid channel race
			<-item.c
			o.releaseItem(item)
		}()
		return ctx.Err()
	}
}

func (o *BulkInsertOperator) run(ctx context.Context) {
	tick := time.NewTicker(FlushInterval)

	go func() {
		defer tick.Stop()

		for {
			select {
			case <-ctx.Done():
				// block all new inserts
				o.closed.Store(true)

				// flush remaining buffer
				if len(o.buf) > 0 {
					o.onFlushDone(ErrBulkInsertClosed, o.buf)
				}

				// keep draining the channel until all in-flight inserts are done
				for {
					select {
					case <-tick.C:
						if o.inFlight.Load() == 0 {
							close(o.c)
							for item := range o.c {
								item.c <- ErrBulkInsertClosed
							}
							return
						}
					case item, ok := <-o.c:
						if ok {
							item.c <- ErrBulkInsertClosed
						}
					}
				}
			case <-tick.C:
				if len(o.buf) > 0 {
					BulkInsertFlushByTimeout.Add(1)
					o.flush(ctx)
				}
			case args, ok := <-o.c:
				if !ok {
					if len(o.buf) > 0 {
						o.flush(ctx)
					}
					return
				}
				o.buf = append(o.buf, args)
				o.rowCnt += len(args.rows)
				if o.rowCnt >= o.maxRows || len(o.buf) >= o.bufSize {
					BulkInsertFlushBySize.Add(1)
					o.flush(ctx)
				}
			}
		}
	}()
}

// flush is not thread-safe. It should only be called in the run goroutine.
func (o *BulkInsertOperator) flush(ctx context.Context) {
	if len(o.buf) == 0 {
		return
	}
	sql, args := _buildInsertStatement(o.sql, o.buf, o.cols, o.endStmts)
	items := make([]*Item, len(o.buf))
	copy(items, o.buf)
	o.buf = o.buf[:0]
	o.rowCnt = 0
	go func() {
		FlushGoroutine.Inc()
		defer FlushGoroutine.Dec()

		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		var err error
		if _, err = o.conn.Exec(c, sql, args...); err != nil {
			o.log.Error("failed to run exec in bulk insert operator", zap.Error(err), zap.String("table", o.table), zap.Int("n_args", len(args)))
		}
		o.onFlushDone(err, items)
	}()
}

func (o *BulkInsertOperator) onFlushDone(err error, items []*Item) {
	for _, item := range items {
		item.c <- err
	}
	if err != nil {
		FlushErrCount.Inc()
	} else {
		FlushSuccessCount.Inc()
	}
	o.log.Debug("bulk insert flush done", zap.Int("n_items", len(items)), zap.Error(err))
}

func _buildPrepareSQL(table string, cols []Column) string {
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Name)
	}
	return "INSERT INTO " + table + " (" + strings.Join(names, ", ") + ") VALUES "
}

func _buildInsertStatement(sql string, items []*Item, cols []Column, endStmts string) (string, []any) {
	n := 0
	for _, item := range items {
		n += len(item.rows) * len(cols)
	}
	var pos int = 0
	var sb strings.Builder
	sb.WriteString(sql)
	for i, item := range items {
		for j := range item.rows {
			sb.WriteString("(")
			for k := range cols {
				sb.WriteString("$")
				sb.WriteString(fmt.Sprint(pos + 1))
				pos++
				if k != len(cols)-1 {
					sb.WriteString(", ")
				} else {
					sb.WriteString(")")
				}
			}
			if j != len(item.rows)-1 || i != len(items)-1 {
				sb.WriteString(", ")
			}
		}
	}
	if endStmts != "" {
		sb.WriteString(";")
		sb.WriteString(endStmts)
		sb.WriteString(";")
	}

	var args = make([]any, 0, n)
	for _, item := range items {
		for _, row := range item.rows {
			rowCopy := make([]any, len(row))
			copy(rowCopy, row)
			args = append(args, rowCopy...)
		}
	}

	return sb.String(), args
}

var (
	ErrInsertStmtNotPrepared = errors.New("insert statement not prepared")
	ErrInsertBackpressure    = errors.New("insert backpressure")
)

type Manager struct {
	ctx context.Context
	log *zap.Logger
	p   *pgxpool.Pool

	mu  sync.Mutex
	ops []*BulkInsertOperator
}

func NewManager(ctx context.Context, p *pgxpool.Pool, log *zap.Logger) (*Manager, error) {
	m := &Manager{
		ctx: ctx,
		log: log.Named("bim"),
		p:   p,
	}

	go func() {
		<-ctx.Done()
		log.Info("bulk insert manager received ctx.Done, closing")
		m.Close()
	}()

	return m, nil
}

// NewBulkInsertOperator creates and registers a bulk insert operator.
//
// Parameters:
//   - table: destination table name used in the generated INSERT statement.
//   - cols: ordered list of columns to insert into; row values must follow this order.
//   - endStmts: optional SQL statements appended after each flushed INSERT,
//     joined with '; ' (for example, "FLUSH").
func (b *Manager) NewBulkInsertOperator(table string, cols []Column, endStmts ...string) (*BulkInsertOperator, error) {

	bufSize := DefaultBufSize

	b.log.Info("creating new bulk insert operator", zap.String("table", table), zap.Any("cols", cols), zap.Int("buf_size", bufSize))

	op := newBulkInsertOperator(b.ctx, table, cols, b.p, bufSize, b.log, endStmts...)

	b.mu.Lock()
	b.ops = append(b.ops, op)
	b.mu.Unlock()

	return op, nil
}

func (b *Manager) Close() {
	for _, op := range b.ops {
		op.Close()
	}
}
