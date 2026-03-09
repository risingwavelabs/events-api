package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createTestTable(t *testing.T) {
	t.Helper()

	sql := `CREATE TABLE IF NOT EXISTS test (
		i DOUBLE PRECISION,
		b BOOLEAN,
		s STRING,
		f DOUBLE PRECISION,
		a STRING[]
	)`

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"http://localhost:8000/v1/sql",
		bytes.NewBufferString(sql),
	)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("failed to create test table: status=%d body=%s", res.StatusCode, string(body))
	}

	// Wait for table watcher refresh before ingesting events.
	time.Sleep(2 * time.Second)
}

func TestIngestEvents(t *testing.T) {
	createTestTable(t)

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
			"http://localhost:8000/v1/events?name=test",
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
}

func TestIngestEventsWithGeneratedColumn(t *testing.T) {
	dropSQL := `DROP TABLE IF EXISTS test_events_generated`
	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"http://localhost:8000/v1/sql",
		bytes.NewBufferString(dropSQL),
	)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	createSQL := `CREATE TABLE test_events_generated (
		id INT PRIMARY KEY,
		data VARCHAR,
		ingested_at TIMESTAMPTZ AS (proctime())
	)`
	req, err = http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"http://localhost:8000/v1/sql",
		bytes.NewBufferString(createSQL),
	)
	require.NoError(t, err)

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("failed to create generated-column table: status=%d body=%s", res.StatusCode, string(body))
	}

	// Wait for table watcher refresh before ingesting events.
	time.Sleep(3 * time.Second)

	insertPayload := []byte(`{"id": 1, "data": "test"}`)
	req, err = http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"http://localhost:8000/v1/events?name=test_events_generated",
		bytes.NewReader(insertPayload),
	)
	require.NoError(t, err)

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, res.StatusCode, "failed to ingest event for generated-column table: body=%s", string(body))
	require.Equal(t, "OK", string(bytes.TrimSpace(body)))
}
