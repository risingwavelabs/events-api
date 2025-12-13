package rw

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/risingwavelabs/eventapi/pkg/closer"
	"github.com/risingwavelabs/eventapi/pkg/gctx"
	"go.uber.org/zap"
)

const (
	FlushInterval = 100 * time.Millisecond
)

type Connection interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)

	Close()
}

var BulkInsertError = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "eventapi_rw_bulk_insert_error",
		Help: "The number of errors encountered during bulk insert operations",
	},
)

var BulkInsertFlushByTimeout = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "eventapi_rw_bulk_insert_flush_by_timeout",
		Help: "The number of times the bulk insert buffer was flushed due to timeout",
	},
)

var BulkInsertFlushBySize = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "eventapi_rw_bulk_insert_flush_by_size",
		Help: "The number of times the bulk insert buffer was flushed due to reaching max size",
	},
)

var FlushGoroutine = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "eventapi_rw_bulk_insert_flush_goroutine",
		Help: "The number of active goroutines flushing the bulk insert buffer",
	},
)

var FlushSuccessCount = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "eventapi_rw_bulk_insert_flush_success_count",
		Help: "The number of successful flush operations",
	},
)

var FlushErrCount = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "eventapi_rw_bulk_insert_flush_error_count",
		Help: "The number of errors encountered during flush operations",
	},
)

var BulkInsertBackpressureHit = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "eventapi_rw_bulk_insert_backpressure_hit",
		Help: "The number of times bulk insert backpressure was hit",
	},
)

type BulkInsertOperator struct {
	log   *zap.Logger
	sql   string
	table string
	nCol  int

	c    chan []any
	buf  [][]any
	conn Connection

	bufSize int
}

func newBulkInsertOperator(ctx context.Context, table string, columns []Column, conn Connection, bufSize int, log *zap.Logger) *BulkInsertOperator {
	cols := make([]string, 0, len(columns))
	for _, c := range columns {
		cols = append(cols, c.Name)
	}

	o := &BulkInsertOperator{
		sql:     _buildPrepareSQL(table, cols),
		nCol:    len(cols),
		buf:     make([][]any, 0, bufSize),
		conn:    conn,
		c:       make(chan []any, bufSize),
		bufSize: bufSize,
		table:   table,
		log: log.Named("bulk_insert").With(
			zap.String("table", table),
		),
	}

	o.run(ctx)

	return o
}

func (o *BulkInsertOperator) Close() {
	close(o.c)
}

func (o *BulkInsertOperator) Insert(ctx context.Context, args ...any) error {
	select {
	case o.c <- args:
	default:
		BulkInsertBackpressureHit.Inc()
		return ErrInsertBackpressure
	}
	return nil
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
				if len(o.buf) == o.bufSize {
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
	sql, args := _buildInsertStatement(o.sql, o.buf, o.nCol)
	o.buf = o.buf[:0]
	go func() {
		FlushGoroutine.Inc()
		defer FlushGoroutine.Dec()

		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if _, err := o.conn.Exec(c, sql, args...); err != nil {
			FlushErrCount.Inc()
			o.log.Error("failed to run exec in bulk insert operator", zap.Error(err), zap.String("table", o.table), zap.Int("n_args", len(args)))
		}
		if _, err := o.conn.Exec(c, "FLUSH"); err != nil {
			FlushErrCount.Inc()
			o.log.Error("failed to run flush in bulk insert operator", zap.Error(err), zap.String("table", o.table), zap.Int("n_args", len(args)))
		}

		FlushSuccessCount.Inc()
	}()
}

func BuildBulkInsertStatement(table string, cols []string, values [][]any) (string, []any) {
	return _buildInsertStatement(
		_buildPrepareSQL(table, cols),
		values,
		len(cols),
	)
}

func _buildPrepareSQL(table string, cols []string) string {
	return "INSERT INTO " + table + " (" + strings.Join(cols, ", ") + ") VALUES "
}

func _buildInsertStatement(sql string, values [][]any, nCols int) (string, []any) {
	n := len(values) * nCols
	var pos int = 0
	var sb strings.Builder
	sb.WriteString(sql)
	for pos < n {
		sb.WriteString("(")
		for i := 1; i <= nCols; i++ {
			sb.WriteString("$")
			sb.WriteString(fmt.Sprint(pos + 1))
			pos++
			if i != nCols {
				sb.WriteString(", ")
			} else {
				sb.WriteString(")")
			}
		}
		if pos < n {
			sb.WriteString(", ")
		}
	}

	var args = make([]any, 0, n)
	for _, row := range values {
		rowCopy := make([]any, len(row))
		copy(rowCopy, row)
		args = append(args, rowCopy...)
	}

	return sb.String(), args
}

var (
	ErrInsertStmtNotPrepared = errors.New("insert statement not prepared")
	ErrInsertBackpressure    = errors.New("insert backpressure")
)

type BulkInsertManager struct {
	globalCtx *gctx.GlobalContext
	cm        *closer.CloserManager
	log       *zap.Logger
	rw        *RisingWave
}

func NewBulkInsertManager(globalCtx *gctx.GlobalContext, rw *RisingWave, cm *closer.CloserManager, log *zap.Logger) (*BulkInsertManager, error) {
	m := &BulkInsertManager{
		globalCtx: globalCtx,
		cm:        cm,
		log:       log.Named("bim"),
		rw:        rw,
	}

	return m, nil
}

func (b *BulkInsertManager) NewBulkInsertOperator(table string, cols []Column, bufSize int) (*BulkInsertOperator, error) {
	ctx := b.globalCtx.Context()

	op := newBulkInsertOperator(ctx, table, cols, b.rw.pool, bufSize, b.log)

	b.cm.Register(func(ctx context.Context) error {
		op.Close()
		return nil
	})

	return op, nil
}
