package rw

import (
	"testing"

	"github.com/risingwavelabs/events-api/pkg/pgb"
	"github.com/stretchr/testify/require"
)

func TestFilterInsertableColumns(t *testing.T) {
	cols := []pgb.Column{
		{Name: "id", Type: "integer"},
		{Name: "_row_id", Type: "integer"},
		{Name: "_rw_timestamp", Type: "timestamptz", IsHidden: true},
		{Name: "ingested_at", Type: "timestamptz", IsGenerated: true},
		{Name: "data", Type: "character varying"},
	}

	filtered := filterInsertableColumns(cols)
	require.Len(t, filtered, 2)
	require.Equal(t, []string{"id", "data"}, []string{filtered[0].Name, filtered[1].Name})
}
