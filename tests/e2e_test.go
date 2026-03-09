package tests

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func randomTableName(prefix string) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"

	buf := make([]byte, 8)
	_, _ = crand.Read(buf)
	for i := range buf {
		buf[i] = letters[int(buf[i])%len(letters)]
	}

	return fmt.Sprintf("%s_%s", prefix, string(buf))
}

func mustExecSQL(t *testing.T, sql string, action string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://localhost:8000/v1/sql",
		bytes.NewBufferString(sql),
	)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equalf(t, http.StatusOK, res.StatusCode, "failed to %s: status=%d body=%s", action, res.StatusCode, string(body))
	return body
}

func dropTable(t *testing.T, tableName string) {
	t.Helper()
	mustExecSQL(t, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName), "drop test table")
}

func initTestTable(t *testing.T) string {
	t.Helper()

	tableName := randomTableName("test")
	dropTable(t, tableName)

	createSQL := fmt.Sprintf(`CREATE TABLE %s (
		i DOUBLE PRECISION,
		b BOOLEAN,
		s STRING,
		f DOUBLE PRECISION,
		a STRING[]
	)`, tableName)

	mustExecSQL(t, createSQL, "create test table")
	t.Cleanup(func() {
		dropTable(t, tableName)
	})

	// Wait for table watcher refresh before ingesting events.
	time.Sleep(2 * time.Second)
	return tableName
}

func initGeneratedColumnTestTable(t *testing.T) string {
	t.Helper()

	tableName := randomTableName("test_events_generated")
	dropTable(t, tableName)

	createSQL := fmt.Sprintf(`CREATE TABLE %s (
		id INT PRIMARY KEY,
		data VARCHAR,
		ingested_at TIMESTAMPTZ AS (proctime())
	)`, tableName)

	mustExecSQL(t, createSQL, "create generated-column test table")
	t.Cleanup(func() {
		dropTable(t, tableName)
	})

	// Wait for table watcher refresh before ingesting events.
	time.Sleep(3 * time.Second)
	return tableName
}

func TestIngestEvents(t *testing.T) {
	tableName := initTestTable(t)

	var (
		// number of requests
		N = 1000
		// number of lines per request
		L = 1000
	)

	data, err := json.Marshal(map[string]any{
		"i": 1,
		"b": false,
		"s": "test",
		"f": 3.14,
		"j": map[string]any{
			"nested": "value",
		},
		"a": []string{"s1", "s2"},
	})
	require.NoError(t, err)
	line := []byte{}
	for range L {
		line = append(line, data...)
		line = append(line, '\n')
	}

	reqs := []*http.Request{}
	for range N {
		req, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"http://localhost:8000/v1/events?name="+tableName,
			bytes.NewReader(line),
		)
		if err != nil {
			t.Fatal("failed to create request:", err.Error())
		}
		reqs = append(reqs, req)
	}

	fmt.Println("Starting benchmark with", N, "requests")

	wg := &sync.WaitGroup{}
	for i := range reqs {
		wg.Add(1)
		go func(i int) {
			defer func() {
				wg.Done()
			}()
			res, err := http.DefaultClient.Do(reqs[i])
			if err != nil {
				t.Log("failed to send request:", err.Error())
				return
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Log("unexpected status code:", res.StatusCode)
				return
			}
		}(i)
	}

	wg.Wait()

	countBody := mustExecSQL(t, fmt.Sprintf("SELECT COUNT(*) AS cnt FROM %s;", tableName), "count rows")
	var countResp struct {
		Error *string `json:"error"`
		Rows  []struct {
			Cnt int64 `json:"cnt"`
		} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(countBody, &countResp))
	if countResp.Error != nil {
		t.Fatalf("count query returned error: %s", *countResp.Error)
	}
	require.Len(t, countResp.Rows, 1)
	require.Equal(t, int64(N*L), countResp.Rows[0].Cnt, "row count mismatch")
}

func TestIngestEventsWithGeneratedColumn(t *testing.T) {
	tableName := initGeneratedColumnTestTable(t)

	insertPayload := []byte(`{"id": 1, "data": "test"}`)
	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"http://localhost:8000/v1/events?name="+tableName,
		bytes.NewReader(insertPayload),
	)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, res.StatusCode, "failed to ingest event for generated-column table: body=%s", string(body))
	require.Equal(t, "OK", string(bytes.TrimSpace(body)))
}
