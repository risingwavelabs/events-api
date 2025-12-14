package rw

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/errors"
	"github.com/risingwavelabs/eventapi/pkg/closer"
	"github.com/risingwavelabs/eventapi/pkg/config"
	"github.com/risingwavelabs/eventapi/pkg/gctx"
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
