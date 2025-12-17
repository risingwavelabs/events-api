//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/risingwavelabs/events-api/app"
	"github.com/risingwavelabs/events-api/pkg/closer"
	"github.com/risingwavelabs/events-api/pkg/config"
	"github.com/risingwavelabs/events-api/pkg/gctx"
	"github.com/risingwavelabs/events-api/pkg/logger"
	"github.com/risingwavelabs/events-api/pkg/rw"

	"github.com/google/wire"
)

func InitApp() (*app.App, error) {
	wire.Build(
		logger.NewLogger,
		app.NewApp,
		app.NewHandler,
		config.NewConfig,
		gctx.New,
		rw.NewRisingWave,
		rw.NewBulkInsertManager,
		rw.NewEventService,
		closer.NewCloserManager,
	)
	return nil, nil
}
