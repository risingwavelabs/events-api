package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/risingwavelabs/eventapi/app/zgen/apigen"
	"github.com/risingwavelabs/eventapi/pkg/rw"
)

type Handler struct {
	rw *rw.RisingWave
	es *rw.EventService
}

func NewHandler(rw *rw.RisingWave, es *rw.EventService) apigen.ServerInterface {
	return &Handler{
		rw: rw,
		es: es,
	}
}

func (h *Handler) IngestEvent(c *fiber.Ctx, params apigen.IngestEventParams) error {
	return h.es.IngestEvent(c.Context(), params.Name, c.Body())
}
