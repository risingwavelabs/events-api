package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/risingwavelabs/eventapi/app/zgen/apigen"
	"github.com/risingwavelabs/eventapi/pkg/rw"
)

type Handler struct {
	rw *rw.RisingWave
}

func NewHandler(rw *rw.RisingWave) apigen.ServerInterface {
	return &Handler{
		rw: rw,
	}
}

func (h *Handler) IngestEvent(c *fiber.Ctx) error {
	return nil
}
