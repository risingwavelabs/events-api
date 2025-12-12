package app

import (
	"fmt"

	"github.com/cloudcarver/anclax/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/risingwavelabs/eventapi/app/zgen/apigen"
	"github.com/risingwavelabs/eventapi/pkg/config"
	"github.com/risingwavelabs/eventapi/pkg/gctx"
	"go.uber.org/zap"
)

type App struct {
	log  *zap.Logger
	app  *fiber.App
	gctx *gctx.GlobalContext
	port int
}

func NewApp(cfg *config.Config, gctx *gctx.GlobalContext, _log *zap.Logger, si apigen.ServerInterface) *App {
	log := _log.Named("app")

	app := fiber.New(fiber.Config{
		ErrorHandler: utils.ErrorHandler,
		BodyLimit:    50 * 1024 * 1024, // 50MB
	})

	apigen.RegisterHandlersWithOptions(app, si, apigen.FiberServerOptions{
		BaseURL: "/v1",
	})

	var port = 8020
	if cfg.Port != 0 {
		port = cfg.Port
	} else {
		log.Info("Using default port", zap.Int("port", port))
	}

	var host = "localhost"
	if cfg.Host != "" {
		host = cfg.Host
	} else {
		log.Info("Using default host", zap.String("host", host))
	}

	return &App{
		app:  app,
		log:  log,
		port: port,
		gctx: gctx,
	}
}

func (a *App) Listen() error {
	shutdownChan := make(chan error)

	go func() {
		if err := a.app.Listen(fmt.Sprintf(":%d", a.port)); err != nil {
			shutdownChan <- err
		}
	}()

	select {
	case err := <-shutdownChan:
		return err
	case <-a.gctx.Context().Done():
		a.log.Info("shutting down server due to context cancellation")
		return a.app.Shutdown()
	}
}

func (a *App) Shutdown() error {
	return a.app.Shutdown()
}
