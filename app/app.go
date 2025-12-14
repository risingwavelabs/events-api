package app

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
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

func ErrorHandler(c *fiber.Ctx, err error) error {
	var code = fiber.StatusInternalServerError

	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}

	c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)

	rid := c.Locals(requestid.ConfigDefault.ContextKey)

	if code == fiber.StatusInternalServerError {
		log.Info(fmt.Sprintf("unexpected error, request-id: %v, err: %v", rid, err), zap.Error(err), zap.String("path", c.Path()))
		return c.Status(code).SendString(fmt.Sprintf("unexpected error, request-id: %v", rid))
	}

	return c.Status(code).SendString(err.Error())
}

func NewApp(cfg *config.Config, gctx *gctx.GlobalContext, _log *zap.Logger, si apigen.ServerInterface) *App {
	log := _log.Named("app")

	app := fiber.New(fiber.Config{
		ErrorHandler: ErrorHandler,
		BodyLimit:    50 * 1024 * 1024, // 50MB
		Concurrency:  256 * 1024,       // 256k
	})

	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))

	app.Use(requestid.New())

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

	debug := NewDebugServer(cfg, gctx, log)
	go func() {
		if err := debug.Start(); err != nil {
			log.Error("debug server exited", zap.Error(err))
		}
	}()

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
