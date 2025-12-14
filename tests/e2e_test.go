package tests

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIngestEvents(t *testing.T) {
	N := 10000

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqs := []*http.Request{}
	for range N {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost:8080/v1/events?name=test", strings.NewReader("{\"c\": 1}"))
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
