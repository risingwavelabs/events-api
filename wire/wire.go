//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/risingwavelabs/eventapi/app"
	"github.com/risingwavelabs/eventapi/pkg/closer"
	"github.com/risingwavelabs/eventapi/pkg/config"
	"github.com/risingwavelabs/eventapi/pkg/gctx"
	"github.com/risingwavelabs/eventapi/pkg/logger"
	"github.com/risingwavelabs/eventapi/pkg/rw"

	"github.com/cloudcarver/anclax/lib/ws"

	"github.com/google/wire"
)

func NewWsc(gctx *gctx.GlobalContext) *ws.WebsocketController {
	return ws.New(gctx.Context(), &ws.WsCfg{})
}

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
		NewWsc,
	)
	return nil, nil
}
