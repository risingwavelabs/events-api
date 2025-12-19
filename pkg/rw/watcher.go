package rw

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/risingwavelabs/events-api/pkg/gctx"
	"go.uber.org/zap"
)

type RelationType string

const watcherPollInterval = 1 * time.Second

type Watcher struct {
	rw   *RisingWave
	gctx *gctx.GlobalContext
	log  *zap.Logger

	mu            sync.RWMutex
	lastKeyToDefs map[string]string

	onRelationUpdate func(relation Relation) error
	onRelationDelete func(name string) error
}

func NewWatcher(
	rw *RisingWave,
	gctx *gctx.GlobalContext,
	log *zap.Logger,
	onRelationUpdate func(relation Relation) error,
	onRelationDelete func(name string) error,
) *Watcher {
	return &Watcher{
		rw:               rw,
		gctx:             gctx,
		log:              log.Named("watcher"),
		onRelationUpdate: onRelationUpdate,
		onRelationDelete: onRelationDelete,
		lastKeyToDefs:    make(map[string]string),
	}
}

func (w *Watcher) Start() {
	ticker := time.NewTicker(watcherPollInterval)
	defer ticker.Stop()

	w.log.Info("starting RisingWave watcher")

	for {
		select {
		case <-w.gctx.Context().Done():
			return
		case <-ticker.C:
			func() {
				ctx, cancel := context.WithTimeout(w.gctx.Context(), 5*time.Second)
				defer cancel()

				if err := w.UpdateCache(ctx); err != nil {
					w.log.Error("failed to update cache", zap.Error(err))
				}
			}()
		}
	}
}

type Column struct {
	// IsHidden Whether the column is hidden
	IsHidden bool `json:"isHidden"`

	// IsPrimaryKey Whether the column is a primary key
	IsPrimaryKey bool `json:"isPrimaryKey"`

	// Name Name of the column
	Name string `json:"name"`

	// Type Data type of the column
	Type string `json:"type"`

	isArray bool
}

type Relation struct {
	// ID Unique identifier of the table
	ID int32 `json:"ID"`

	// Columns List of columns in the table
	Columns []Column `json:"columns"`

	// Name Name of the table
	Name string `json:"name"`

	Definition string `json:"definition"`

	// Schema Name of the schema this table belongs to
	Schema string `json:"schema"`

	// Type Type of the relation
	Type RelationType `json:"type"`
}

const getRelationsSQL = `SELECT
	rw_relations.id,
	rw_schemas.name AS schema,
	rw_relations.name,
	rw_relations.relation_type,
	rw_relations.definition
FROM rw_relations 
JOIN rw_schemas ON rw_schemas.id = rw_relations.schema_id
WHERE relation_type = 'table'
`

const getColumnsSQL = `SELECT 
    rw_relations.id            AS relation_id,
    rw_schemas.name            AS schema, 
    rw_relations.name          AS relation_name, 
    rw_relations.relation_type AS relation_type, 
    rw_columns.name            AS column_name,
    rw_columns.data_type       AS column_type,
    rw_columns.is_primary_key  AS is_primary_key,
	rw_columns.is_hidden       AS is_hidden
FROM rw_columns
JOIN rw_relations ON rw_relations.id = rw_columns.relation_id
JOIN rw_schemas   ON rw_schemas.id = rw_relations.schema_id
WHERE rw_relations.relation_type = 'table'
`

func (w *Watcher) UpdateCache(ctx context.Context) error {
	rows, err := w.rw.pool.Query(ctx, getRelationsSQL)
	if err != nil {
		return errors.Wrap(err, "failed to fetch relations from RisingWave")
	}
	defer rows.Close()

	updatedRelations := make(map[string]Relation)

	newlyFetched := make(map[string]struct{})
	for rows.Next() {
		var (
			relationID   int32
			relationName string
			relationType RelationType
			definition   string
			schema       string
		)

		if err := rows.Scan(&relationID, &schema, &relationName, &relationType, &definition); err != nil {
			return errors.Wrap(err, "failed to scan relation row")
		}

		relation := Relation{
			ID:         relationID,
			Schema:     schema,
			Name:       relationName,
			Type:       relationType,
			Definition: definition,
		}

		key := schema + "." + relationName

		newlyFetched[key] = struct{}{}

		_, exist := w.lastKeyToDefs[key]
		if exist && w.lastKeyToDefs[key] == definition {
			continue
		}

		updatedRelations[key] = relation
		if !exist {
			w.log.Info("new relation detected", zap.String("relation", relationName))
		} else {
			w.log.Info("relation definition changed", zap.String("relation", relationName))
		}
	}

	if rows.Err() != nil {
		return errors.Wrap(rows.Err(), "error occurred during rows iteration")
	}

	rows, err = w.rw.pool.Query(ctx, getColumnsSQL)
	if err != nil {
		return errors.Wrap(err, "failed to fetch columns from RisingWave")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			relationID   int32
			schema       string
			relationName string
			relationType string
			columnName   string
			columnType   string
			isPrimaryKey bool
			isHidden     bool
		)

		if err := rows.Scan(&relationID, &schema, &relationName, &relationType, &columnName, &columnType, &isPrimaryKey, &isHidden); err != nil {
			return errors.Wrap(err, "failed to scan relation row")
		}

		key := schema + "." + relationName
		relation, exists := updatedRelations[key]
		if exists {
			relation.Columns = append(relation.Columns, Column{
				Name:         columnName,
				Type:         columnType,
				IsPrimaryKey: isPrimaryKey,
				IsHidden:     isHidden,
				isArray:      strings.HasSuffix(columnType, "[]"),
			})
			updatedRelations[key] = relation
		}
	}

	if rows.Err() != nil {
		return errors.Wrap(rows.Err(), "error occurred during rows iteration")
	}

	w.mu.Lock()
	for k, v := range updatedRelations {
		w.lastKeyToDefs[k] = v.Definition
		if err := w.onRelationUpdate(v); err != nil {
			w.log.Error("failed to handle relation update", zap.String("relation", k), zap.Error(err))
		}
	}
	for k := range w.lastKeyToDefs {
		if _, exist := newlyFetched[k]; !exist {
			w.log.Info("relation deleted", zap.String("relation", k))
			delete(w.lastKeyToDefs, k)
			if err := w.onRelationDelete(k); err != nil {
				w.log.Error("failed to handle relation delete", zap.String("relation", k), zap.Error(err))
			}
		}
	}
	w.mu.Unlock()
	return nil
}
