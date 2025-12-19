// loadtest.go
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type headerFlags []string

func (h *headerFlags) String() string {
	return strings.Join(*h, ", ")
}
func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

type Result struct {
	statusCode int
	latency    time.Duration
	err        error
    errorBodySnippet string
}

func parseHeaders(hs headerFlags) (map[string]string, error) {
	headers := make(map[string]string)
	for _, line := range hs {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header: %q (expected 'Key: Value')", line)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		headers[key] = val
	}
	return headers, nil
}

func main() {
	var (
		targetURL     string
		method        string
		requests      int
		concurrency   int
		timeoutSec    int
		payloadFile   string
		payloadString string
		contentType   string
		headersFlag   headerFlags
	)
	flag.StringVar(&targetURL, "url", "http://localhost:8080/healthz", "Target URL")
	flag.StringVar(&method, "method", "GET", "HTTP method (GET|POST|...)")
	flag.IntVar(&requests, "requests", 10000, "Total number of requests to send")
	flag.IntVar(&concurrency, "concurrency", 10000, "Number of concurrent workers")
    flag.IntVar(&timeoutSec, "timeout", 60, "Per-request timeout seconds")
	flag.StringVar(&payloadFile, "payload-file", "", "Payload file path (for POST/PUT)")
	flag.StringVar(&payloadString, "payload", "", "Inline payload string (for POST/PUT)")
	flag.StringVar(&contentType, "content-type", "application/x-www-form-urlencoded", "Content-Type header")
	flag.Var(&headersFlag, "H", "Extra header (repeatable), e.g., -H 'Authorization: ...'")

	flag.Parse()

	if requests <= 0 || concurrency <= 0 {
		fmt.Println("requests and concurrency must be > 0")
		os.Exit(1)
	}
	if concurrency > requests {
		concurrency = requests
	}

	extraHeaders, err := parseHeaders(headersFlag)
	if err != nil {
		fmt.Println("header parse error:", err)
		os.Exit(1)
	}

	var payloadBytes []byte
	if payloadFile != "" {
		payloadBytes, err = os.ReadFile(payloadFile)
		if err != nil {
			fmt.Println("read payload file error:", err)
			os.Exit(1)
		}
	} else if payloadString != "" {
		payloadBytes = []byte(payloadString)
	}

    transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          concurrency,
		MaxIdleConnsPerHost:   concurrency,
		MaxConnsPerHost:       concurrency,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
        ResponseHeaderTimeout: time.Duration(timeoutSec) * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeoutSec) * time.Second,
	}

	jobs := make(chan int, requests)
	results := make(chan Result, requests)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

    var wg sync.WaitGroup
    testStart := time.Now()
	worker := func() {
		defer wg.Done()
		for range jobs {
			var body io.Reader
			if len(payloadBytes) > 0 && strings.ToUpper(method) != "GET" {
				body = bytes.NewReader(payloadBytes)
			}
			req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
			if err != nil {
				results <- Result{err: err}
				continue
			}
			if body != nil && contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			for k, v := range extraHeaders {
				req.Header.Set(k, v)
			}

			start := time.Now()
            resp, err := client.Do(req)
			lat := time.Since(start)

			if err != nil {
                results <- Result{latency: lat, err: err}
				continue
			}
            var snippet string
            if resp.StatusCode < 200 || resp.StatusCode >= 300 {
                // capture small error body snippet
                b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
                snippet = strings.TrimSpace(string(b))
            } else {
                io.Copy(io.Discard, resp.Body)
            }
            resp.Body.Close()
            results <- Result{statusCode: resp.StatusCode, latency: lat, err: nil, errorBodySnippet: snippet}
		}
	}

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go worker()
	}

	for i := 0; i < requests; i++ {
		jobs <- i
	}
	close(jobs)

    wg.Wait()
    totalElapsed := time.Since(testStart)
	close(results)

    var (
		latencies      []time.Duration
		successCount   int
		errorCount     int
		statusCounters = make(map[int]int)
        errorKinds     = make(map[string]int)
	)

	for r := range results {
        if r.err != nil {
            errorCount++
            errorKinds[r.err.Error()]++
            latencies = append(latencies, r.latency)
            continue
        }
        statusCounters[r.statusCode]++
        if r.statusCode >= 200 && r.statusCode < 400 {
            successCount++
        } else {
            errorCount++
            key := fmt.Sprintf("HTTP %d", r.statusCode)
            if r.errorBodySnippet != "" {
                key = fmt.Sprintf("%s: %s", key, truncateForPrint(r.errorBodySnippet, 120))
            }
            errorKinds[key]++
        }
        latencies = append(latencies, r.latency)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	p := func(percent float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(percent*float64(len(latencies))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(latencies) {
			idx = len(latencies) - 1
		}
		return latencies[idx]
	}

	var avg time.Duration
	for _, d := range latencies {
		avg += d
	}
	if len(latencies) > 0 {
		avg /= time.Duration(len(latencies))
	}

    fmt.Println("=== Load Test Summary ===")
	fmt.Printf("URL:            %s\n", targetURL)
	fmt.Printf("Method:         %s\n", method)
	fmt.Printf("Requests:       %d\n", requests)
	fmt.Printf("Concurrency:    %d\n", concurrency)
	fmt.Printf("Success:        %d\n", successCount)
	fmt.Printf("Errors:         %d\n", errorCount)
    fmt.Printf("Total Elapsed:  %v\n", totalElapsed)
	fmt.Printf("Status Counts:  %v\n", statusCounters)
	if len(latencies) > 0 {
		fmt.Printf("Avg Latency:    %v\n", avg)
		fmt.Printf("P50 Latency:    %v\n", p(0.50))
		fmt.Printf("P90 Latency:    %v\n", p(0.90))
		fmt.Printf("P95 Latency:    %v\n", p(0.95))
		fmt.Printf("P99 Latency:    %v\n", p(0.99))
	}

    if len(errorKinds) > 0 {
        type kv struct{ k string; v int }
        var arr []kv
        for k, v := range errorKinds {
            arr = append(arr, kv{k, v})
        }
        sort.Slice(arr, func(i, j int) bool { return arr[i].v > arr[j].v })
        maxShow := 10
        if len(arr) < maxShow { maxShow = len(arr) }
        fmt.Println("Top Error Kinds:")
        for i := 0; i < maxShow; i++ {
            fmt.Printf("  %d) %s  (count=%d)\n", i+1, arr[i].k, arr[i].v)
        }
    }
}

func truncateForPrint(s string, max int) string {
    if max <= 0 || len(s) <= max {
        return s
    }
    return s[:max] + "..."
}