package rw

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudcarver/anclax/pkg/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/events-api/app/zgen/apigen"
	"github.com/risingwavelabs/events-api/pkg/closer"
	"github.com/risingwavelabs/events-api/pkg/config"
	"github.com/risingwavelabs/events-api/pkg/gctx"
	"go.uber.org/zap"
)

var ErrQueryFailed = errors.New("query failed")

type RisingWave struct {
	pool      *pgxpool.Pool
	dsn       string
	globalCtx *gctx.GlobalContext
}

func parse(cfg *config.Rw) string {
	if cfg.DSN != nil {
		return *cfg.DSN
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Db,
		cfg.SSLMode,
	)
}

func NewRisingWave(cfg *config.Config, globalCtx *gctx.GlobalContext, cm *closer.CloserManager, log *zap.Logger) (*RisingWave, error) {
	if cfg.Rw == nil {
		return nil, errors.New("risingwave config is nil")
	}

	dialCtx, cancel := context.WithTimeout(globalCtx.Context(), 15*time.Second)
	defer cancel()

	dsn := parse(cfg.Rw)

	pgxCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse dsn")
	}

	pgxCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pgxCfg.MaxConns = 1000
	pgxCfg.MinConns = 10

	pool, err := pgxpool.NewWithConfig(dialCtx, pgxCfg)
	if err != nil {
		return nil, err
	}

	ready := false
	for i := 0; i < 10; i++ {
		err = pool.Ping(dialCtx)
		if err == nil {
			ready = true
			break
		}
		time.Sleep(500 * time.Millisecond)
		log.Error("failed to connect to risingwave, retrying...", zap.Error(err))
	}

	if !ready {
		pool.Close()
		return nil, errors.Wrap(err, "failed to connect to risingwave")
	}

	rw := &RisingWave{
		pool:      pool,
		globalCtx: globalCtx,
		dsn:       dsn,
	}

	cm.Register(func(ctx context.Context) error {
		pool.Close()
		return nil
	})

	return rw, nil
}

func (rw *RisingWave) Pool() *pgxpool.Pool {
	return rw.pool
}

func (rw *RisingWave) QueryDatabase(ctx context.Context, sql string) (*apigen.QueryResponse, error) {
	result, err := query(ctx, rw.Pool(), sql, false /*TODO, support background DDL and acuqire conn when true */)
	if err != nil {
		if errors.Is(err, ErrQueryFailed) {
			return &apigen.QueryResponse{
				Error: utils.Ptr(err.Error()),
			}, nil
		}
		return nil, errors.Wrapf(err, "failed to query database")
	}

	columns := make([]apigen.Column, len(result.Columns))
	for i, column := range result.Columns {
		columns[i] = apigen.Column{
			Name: column.Name,
			Type: column.Type,
		}
	}

	return &apigen.QueryResponse{
		Columns: columns,
		Rows:    result.Rows,
	}, nil
}

type DB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type Result struct {
	RowsAffected int64
	Columns      []Column
	Rows         []map[string]any
}

func query(ctx context.Context, db DB, query string, backgroundDDL bool) (*Result, error) {
	if backgroundDDL {
		_, err := db.Exec(ctx, "SET BACKGROUND_DDL = true")
		if err != nil {
			return nil, errors.Wrap(ErrQueryFailed, err.Error())
		}
	}

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, errors.Wrap(ErrQueryFailed, err.Error())
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	columns := make([]Column, len(fieldDescs))
	for i, d := range fieldDescs {
		columns[i] = Column{
			Name: string(d.Name),
			Type: getDataTypeName(d.DataTypeOID),
		}
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(fieldDescs))
		scanArgs := make([]any, len(fieldDescs))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(fieldDescs))
		for i, col := range fieldDescs {
			row[string(col.Name)] = values[i]
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(ErrQueryFailed, err.Error())
	}

	return &Result{
		RowsAffected: rows.CommandTag().RowsAffected(),
		Columns:      columns,
		Rows:         result,
	}, nil
}

func getDataTypeName(oid uint32) string {
	typeMap := map[uint32]string{
		16:   "boolean",
		20:   "bigint",
		21:   "smallint",
		23:   "integer",
		25:   "text",
		700:  "real",
		701:  "double precision",
		1043: "varchar",
		1114: "timestamp",
		1184: "timestamptz",
		2950: "uuid",
	}

	if name, ok := typeMap[oid]; ok {
		return name
	}
	return fmt.Sprintf("unknown_OID(%d)", oid)
}
