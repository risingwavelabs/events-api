package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/risingwavelabs/events-api/pkg/config"
	"github.com/risingwavelabs/events-api/pkg/gctx"
	"go.uber.org/zap"
)

type DebugServer struct {
	gctx   *gctx.GlobalContext
	port   int
	enable bool
	log    *zap.Logger
}

func NewDebugServer(cfg *config.Config, gctx *gctx.GlobalContext, log *zap.Logger) *DebugServer {
	return &DebugServer{
		gctx:   gctx,
		port:   cfg.Debug.Port,
		enable: cfg.Debug.Enable,
		log:    log,
	}
}

func (d *DebugServer) Start() error {
	if !d.enable {
		return nil
	}
	if d.port == 0 {
		d.port = 8777
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", d.port),
		Handler: mux,
	}

	go func() {
		d.log.Info("debug server is listening", zap.Int("port", d.port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			d.log.Error("debug server exited", zap.Error(err))
		}
	}()

	<-d.gctx.Context().Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		d.log.Error("debug server shutdown error", zap.Error(err))
	} else {
		d.log.Info("debug server shutdown gracefully")
	}

	return nil
}
