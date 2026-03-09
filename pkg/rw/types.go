package rw

import "github.com/risingwavelabs/events-api/pkg/pgb"

type RelationType string

type Column struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	IsHidden     bool   `json:"isHidden"`
	IsGenerated  bool   `json:"isGenerated"`
	IsPrimaryKey bool   `json:"isPrimaryKey"`
	IsArray      bool   `json:"isArray"`
}

type Relation struct {
	ID         int32        `json:"ID"`
	Columns    []Column     `json:"columns"`
	Name       string       `json:"name"`
	Definition string       `json:"definition"`
	Schema     string       `json:"schema"`
	Type       RelationType `json:"type"`
}

func toPGBColumns(cols []Column) []pgb.Column {
	converted := make([]pgb.Column, len(cols))
	for i, c := range cols {
		converted[i] = pgb.Column{
			Name: c.Name,
			Type: c.Type,
		}
	}
	return converted
}
