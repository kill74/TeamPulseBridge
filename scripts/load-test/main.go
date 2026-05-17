package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	target := flag.String("target", "http://localhost:8080", "target URL")
	rate := flag.Int("rate", 100, "requests per second")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	source := flag.String("source", "github", "webhook source")
	payload := flag.String("payload", `{"action":"opened"}`, "request body")
	flag.Parse()

	endpoint := fmt.Sprintf("%s/webhooks/%s", *target, *source)
	body := []byte(*payload)

	var (
		total   int64
		success int64
		failed  int64
	)

	interval := time.Second / time.Duration(*rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	deadline := time.After(*duration)
	var wg sync.WaitGroup

	fmt.Printf("load test: target=%s rate=%d/s duration=%s source=%s\n", endpoint, *rate, *duration, *source)

	for {
		select {
		case <-ticker.C:
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-GitHub-Event", "pull_request")
				req.Header.Set("X-Hub-Signature-256", "sha256=skip-for-load-test")
				resp, err := http.DefaultClient.Do(req)
				atomic.AddInt64(&total, 1)
				if err == nil && resp.StatusCode < 500 {
					atomic.AddInt64(&success, 1)
					resp.Body.Close()
				} else {
					atomic.AddInt64(&failed, 1)
				}
			}()
		case <-deadline:
			wg.Wait()
			t := atomic.LoadInt64(&total)
			s := atomic.LoadInt64(&success)
			f := atomic.LoadInt64(&failed)
			fmt.Printf("\n=== Results ===\n")
			fmt.Printf("Total:     %d\n", t)
			fmt.Printf("Success:   %d (%.1f%%)\n", s, float64(s)/float64(t)*100)
			fmt.Printf("Failed:    %d (%.1f%%)\n", f, float64(f)/float64(t)*100)
			fmt.Printf("Throughput: %.1f rps\n", float64(t)/duration.Seconds())
			return
		}
	}
}
