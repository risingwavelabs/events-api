package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIngestEvents(t *testing.T) {
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
			"http://localhost:8080/v1/events?name=test",
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
			if res.StatusCode != http.StatusOK {
				t.Log("unexpected status code:", res.StatusCode)
				return
			}
		}(i)
	}

	wg.Wait()
}
