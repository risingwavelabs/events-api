package app

import (
	"encoding/json"
	"time"

	"github.com/risingwavelabs/eventapi/app/zgen/apigen"

	"github.com/cloudcarver/anclax/lib/ws"
	"github.com/cloudcarver/anclax/pkg/utils"
	"github.com/gofiber/fiber/v2/log"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type WsHandler struct {
}

func NewWsHandler() *WsHandler {
	return &WsHandler{}
}

func sendSuccess(c *ws.Ctx, msg string) error {
	return c.WriteTextMessage(&apigen.WsResponse{
		ID:      utils.Unwrap(c.ID),
		Type:    apigen.WsResponseTypeSuccess,
		Payload: apigen.WsResponsePayload{Success: &apigen.WsResSuccess{Msg: msg}},
	})
}

func SendError(c *ws.Ctx, err error) error {
	return c.WriteTextMessage(&apigen.WsResponse{
		ID:      utils.Unwrap(c.ID),
		Type:    apigen.WsResponseTypeError,
		Payload: apigen.WsResponsePayload{Error: &apigen.WsResError{Msg: err.Error()}},
	})
}

func (m *WsHandler) OnSessionCreated(s *ws.Session) error {
	s.Conn().Locals("subs", make(map[string]struct{}))

	s.RegisterOnClose(func() error {
		subs := s.Conn().Locals("subs").(map[string]struct{})
		for topic := range subs {
			m.subs.Unsubscribe(topic, s)
		}
		return nil
	})
	return nil
}

func (m *WsHandler) Handle(c *ws.Ctx, data []byte) error {
	var req apigen.WsMessage
	if err := json.Unmarshal(data, &req); err != nil {
		return errors.Wrap(ws.ErrBadRequest, "invalid json")
	}

	log.Info("ws message received: ", zap.Any("req", req), zap.String("ws_request_id", c.Session.ID()))

	switch req.Type {
	case apigen.WsMessageTypeSubscribe:
		if req.Payload.Subscribe == nil {
			return ws.ErrBadRequest
		}
		return m.handleSubscribe(c, req.Payload.Subscribe)
	case apigen.WsMessageTypeUnsubscribe:
		if req.Payload.Unsubscribe == nil {
			return ws.ErrBadRequest
		}
		return m.handleUnsubscribe(c, req.Payload.Unsubscribe)
	case apigen.WsMessageTypePing:
		return m.handlePing(c)
	default:
		return errors.Errorf("unknown message type: %s", req.Type)
	}
}

func (m *WsHandler) handleSubscribe(c *ws.Ctx, payload *apigen.WsSubscribePayload) error {
	subs := c.Conn().Locals("subs").(map[string]struct{})
	topic := string(payload.Topic)
	if _, ok := subs[topic]; ok {
		return sendSuccess(c, "already subscribed to "+topic)
	}
	subs[topic] = struct{}{}

	if err := m.subs.Subscribe(topic, c.Session); err != nil {
		return errors.Wrap(ws.ErrBiz, err.Error())
	}
	return sendSuccess(c, "subscribed to "+topic)
}

func (m *WsHandler) handleUnsubscribe(c *ws.Ctx, payload *apigen.WsUnsubscribePayload) error {
	if c.Conn().Locals("subs") == nil {
		return errors.New("subs is empty")
	}
	topic := string(payload.Topic)

	subs := c.Conn().Locals("subs").(map[string]struct{})
	if _, ok := subs[topic]; !ok {
		return sendSuccess(c, "not subscribed to "+topic)
	}
	delete(subs, topic)
	m.subs.Unsubscribe(topic, c.Session)

	return sendSuccess(c, "unsubscribed from "+topic)
}

func (m *WsHandler) handlePing(c *ws.Ctx) error {
	return c.WriteTextMessage(&apigen.WsResponse{
		ID:   utils.Unwrap(c.ID),
		Type: apigen.WsResponseTypePing,
		Payload: apigen.WsResponsePayload{
			Ping: &apigen.WsResPing{
				TimestampMillis: time.Now().UnixMilli(),
			},
		},
	})
}
