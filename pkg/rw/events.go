package rw

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/risingwavelabs/events-api/pkg/gctx"
	"go.uber.org/zap"
)

type EventParser struct {
	cidx  map[string]int
	cType map[string]string
}

func NewEventParser(cols []Column) *EventParser {
	cidx := make(map[string]int)
	cType := make(map[string]string)
	for i, col := range cols {
		cidx[col.Name] = i
		cType[col.Name] = col.Type
	}

	return &EventParser{
		cidx:  cidx,
		cType: cType,
	}
}

func (p *EventParser) Parse(lines [][]byte) ([][]any, error) {
	result := make([][]any, 0, len(lines))
	for _, line := range lines {
		if bytes.Trim(line, " \n\r\t\r") == nil {
			continue
		}
		v, err := p.extractValues(line)
		if err != nil {
			return nil, errors.Wrap(err, "failed to extract values from line")
		}
		result = append(result, v)
	}
	return result, nil
}

func (p *EventParser) extractValues(line []byte) ([]any, error) {
	ret := make([]any, len(p.cidx))
	m := NewLiteMap(p.cType)
	if err := m.UnmarshalJSON(line); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal json: %s", string(line))
	}
	for key, value := range m.data {
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
	filteredCols := []Column{}
	for _, c := range cols {
		if c.Name == "_row_id" {
			continue
		}
		if strings.HasPrefix(c.Name, "_rw") {
			continue
		}
		filteredCols = append(filteredCols, c)
	}

	bio, err := bim.NewBulkInsertOperator(table, filteredCols, DefaultBIOSize) // TODO: dynamic buffer size
	if err != nil {
		return nil, errors.Wrap(err, "failed to create bulk insert operator")
	}

	return &EventHandler{
		bio:    bio,
		parser: NewEventParser(filteredCols),
	}, nil
}

func (i *EventHandler) Ingest(ctx context.Context, lines [][]byte) error {
	rows, err := i.parser.Parse(lines)
	if err != nil {
		return errors.Wrapf(err, "failed to parse lines")
	}

	if err := i.bio.Insert(ctx, rows); err != nil {
		return errors.Wrap(err, "failed to insert event")
	}

	return nil
}

type EventService struct {
	handlers map[string]*EventHandler
	mu       sync.RWMutex

	bim *BulkInsertManager
	log *zap.Logger
}

func NewEventService(gctx *gctx.GlobalContext, rw *RisingWave, log *zap.Logger, bim *BulkInsertManager) (*EventService, error) {
	es := &EventService{
		handlers: make(map[string]*EventHandler),
		bim:      bim,
		log:      log.Named("event_service"),
	}

	watcher := NewWatcher(rw, gctx, log, es.onRelatioonUpdate, es.onRelationDelete)
	if err := watcher.UpdateCache(gctx.Context()); err != nil { // initial cache update
		return nil, errors.Wrap(err, "failed to perform initial cache update")
	}
	go watcher.Start()

	return es, nil
}

func (s *EventService) IngestEvent(ctx context.Context, name string, raw []byte) error {
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

	if err := handler.Ingest(ctx, bytes.Split(raw, []byte("\n"))); err != nil {
		return errors.Wrap(err, "failed to ingest event")
	}

	return nil
}

func (s *EventService) onRelatioonUpdate(relation Relation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.log.Info("create event handler for relation", zap.Any("relation", relation))

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

type LiteMap struct {
	data  map[string]any
	typem map[string]string
}

func NewLiteMap(typem map[string]string) *LiteMap {
	return &LiteMap{
		data:  make(map[string]any),
		typem: typem,
	}
}

func (m *LiteMap) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for k, v := range raw {
		trimmed := bytes.TrimSpace(v)

		if len(trimmed) == 0 {
			continue
		}

		switch trimmed[0] {
		case '{':
			m.data[k] = v
		case '[':
			typ, ok := m.typem[k]
			if !ok {
				return errors.Errorf("no type information for field %s", k)
			}
			arr, err := parseArray(v, typ)
			if err != nil {
				return errors.Wrapf(err, "failed to parse array for field %s", k)
			}
			m.data[k] = arr
		default:
			var val any
			if err := json.Unmarshal(v, &val); err != nil {
				return err
			}
			m.data[k] = val
		}
	}

	return nil
}

func parseArray(raw json.RawMessage, typ string) (any, error) {
	itemTyp := strings.TrimSuffix(typ, "[]")
	if strings.HasSuffix(itemTyp, "[]") {
		var ret []json.RawMessage
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		result := make([]any, 0, len(ret))
		for _, item := range ret {
			arr, err := parseArray(item, itemTyp)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse nested array")
			}
			result = append(result, arr)
		}
		return result, nil
	}

	if strings.HasPrefix(itemTyp, "struct") {
		var ret []string
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	}

	switch itemTyp {
	case "character varying", "interval", "date", "time", "timestamp", "timestamptz", "time with time zone", "time without time zone", "rw_int256":
		var ret []string
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "integer":
		var ret []int32
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "jsonb":
		var ret []json.RawMessage
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "bigint":
		var ret []int64
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "double precision":
		var ret []float64
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "boolean":
		var ret []bool
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "smallint":
		var ret []int16
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "real":
		var ret []float32
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "numeric":
		var ret []string
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	case "bytea":
		var ret [][]byte
		if err := json.Unmarshal(raw, &ret); err != nil {
			return nil, err
		}
		return ret, nil
	}
	return nil, errors.Errorf("unsupported type: %s", typ)
}
