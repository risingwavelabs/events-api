package rw

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/risingwavelabs/events-api/pkg/gctx"
	"go.uber.org/zap"
)

const (
	FlushInterval  = 500 * time.Millisecond
	DefaultBIOSize = 5000
)

type Connection interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Release()
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
	log   *zap.Logger
	sql   string
	table string
	cols  []Column

	itemPool sync.Pool

	c      chan *Item
	buf    []*Item
	rowCnt int
	conn   Connection

	bufSize int
}

func newBulkInsertOperator(ctx context.Context, table string, cols []Column, conn Connection, bufSize int, log *zap.Logger) *BulkInsertOperator {
	o := &BulkInsertOperator{
		sql:     _buildPrepareSQL(table, cols),
		cols:    cols,
		buf:     make([]*Item, 0, bufSize),
		conn:    conn,
		c:       make(chan *Item, bufSize),
		bufSize: bufSize,
		table:   table,
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
	}

	o.run(ctx)

	return o
}

func (o *BulkInsertOperator) Close() {
	close(o.c)
	o.conn.Release()
}

func (o *BulkInsertOperator) Insert(ctx context.Context, rows [][]any) error {
	item := o.itemPool.Get().(*Item)
	defer o.itemPool.Put(item)

	item.rows = rows

	select {
	case <-item.c: // drain previous error if any
	default:
	}

	select {
	case o.c <- item:
	default:
		BulkInsertBackpressureHit.Inc()
		return ErrInsertBackpressure
	}

	select {
	case err := <-item.c:
		return err
	case <-ctx.Done():
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
				if len(o.buf) > 0 {
					o.flush(ctx)
				}
				return
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
				if o.rowCnt >= o.bufSize {
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
	sql, args := _buildInsertStatement(o.sql, o.buf, o.cols)
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
		defer o.onFlushDone(err, items)
		if _, err = o.conn.Exec(c, sql, args...); err != nil {
			o.log.Error("failed to run exec in bulk insert operator", zap.Error(err), zap.String("table", o.table), zap.Int("n_args", len(args)))
			return
		}
		if _, err = o.conn.Exec(c, "FLUSH"); err != nil {
			o.log.Error("failed to run flush in bulk insert operator", zap.Error(err), zap.String("table", o.table), zap.Int("n_args", len(args)))
			return
		}
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

func _buildInsertStatement(sql string, items []*Item, cols []Column) (string, []any) {
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

type BulkInsertManager struct {
	globalCtx *gctx.GlobalContext
	log       *zap.Logger
	rw        *RisingWave
}

func NewBulkInsertManager(globalCtx *gctx.GlobalContext, rw *RisingWave, log *zap.Logger) (*BulkInsertManager, error) {
	m := &BulkInsertManager{
		globalCtx: globalCtx,
		log:       log.Named("bim"),
		rw:        rw,
	}

	return m, nil
}

func (b *BulkInsertManager) NewBulkInsertOperator(table string, cols []Column, bufSize int) (*BulkInsertOperator, error) {
	b.log.Info("creating new bulk insert operator", zap.String("table", table), zap.Any("cols", cols), zap.Int("buf_size", bufSize))

	ctx, cancel := context.WithTimeout(b.globalCtx.Context(), 5*time.Second)
	defer cancel()

	conn, err := b.rw.pool.Acquire(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to acquire connection for bulk insert operator")
	}

	op := newBulkInsertOperator(b.globalCtx.Context(), table, cols, conn, bufSize, b.log)

	return op, nil
}
