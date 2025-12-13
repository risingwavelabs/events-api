package rw

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/risingwavelabs/eventapi/pkg/gctx"
	"go.uber.org/zap"
)

type EventParser struct {
	cidx map[string]int
}

func NewEventParser(cols []Column) *EventParser {
	cidx := make(map[string]int)
	for i, col := range cols {
		cidx[col.Name] = i
	}

	return &EventParser{
		cidx: cidx,
	}
}

func (p *EventParser) Parse(raw json.RawMessage) ([]any, error) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	ret := make([]any, len(p.cidx))
	for key, value := range data {
		if idx, ok := p.cidx[key]; ok {
			ret[idx] = value
		}
	}

	return ret, nil
}

type EventHandler struct {
	bio    *BulkInsertOperator
	parser *EventParser
}

func NewEventHandler(table string, cols []Column, bim *BulkInsertManager) (*EventHandler, error) {
	bio, err := bim.NewBulkInsertOperator(table, cols, 1000) // TODO: dynamic buffer size
	if err != nil {
		return nil, errors.Wrap(err, "failed to create bulk insert operator")
	}

	return &EventHandler{
		bio:    bio,
		parser: NewEventParser(cols),
	}, nil
}

func (i *EventHandler) Ingest(ctx context.Context, raw json.RawMessage) error {
	args, err := i.parser.Parse(raw)
	if err != nil {
		return errors.Wrap(err, "failed to parse event")
	}

	if err := i.bio.Insert(ctx, args...); err != nil {
		return errors.Wrap(err, "failed to insert event")
	}

	return nil
}

type EventService struct {
	handlers map[string]*EventHandler
	mu       sync.RWMutex

	bim *BulkInsertManager
}

func NewEventService(gctx *gctx.GlobalContext, rw *RisingWave, log *zap.Logger, bim *BulkInsertManager) *EventService {
	es := &EventService{
		handlers: make(map[string]*EventHandler),
		bim:      bim,
	}

	watcher := NewWatcher(rw, gctx, log, es.onRelatioonUpdate, es.onRelationDelete)
	go watcher.Start()

	return es
}

func (s *EventService) IngestEvent(ctx context.Context, name string, raw json.RawMessage) error {
	key := name
	if !strings.ContainsAny(name, ".") {
		key = "public." + name
	}

	s.mu.RLock()
	handler, exist := s.handlers[key]
	s.mu.RUnlock()

	if !exist {
		return errors.Errorf("no handler for relation %s", key)
	}

	if err := handler.Ingest(ctx, raw); err != nil {
		return errors.Wrap(err, "failed to ingest event")
	}

	return nil
}

func (s *EventService) onRelatioonUpdate(relation Relation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	handler, err := NewEventHandler(relation.Schema+"."+relation.Name, relation.Columns, s.bim)
	if err != nil {
		return errors.Wrap(err, "failed to create event handler")
	}
	s.handlers[relation.Schema+"."+relation.Name] = handler
	return nil
}

func (s *EventService) onRelationDelete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.handlers, name)
	return nil
}
