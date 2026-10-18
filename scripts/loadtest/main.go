// Command loadtest drives a running Hooklane stack: it starts an in-process
// webhook receiver, creates an endpoint that points at it, ingests events and
// reports ingest throughput, delivery latency and delivery correctness
// (at-least-once: missing and duplicate deliveries).
//
// The API must be able to reach the receiver. That needs APP_ENV=development
// on the API and worker, because production refuses http:// and private or
// loopback destinations. When the stack runs in Docker, pass -advertise with a
// host name the containers can resolve (host.docker.internal on Docker
// Desktop) and keep the default -listen of 0.0.0.0:0.
//
// Only the standard library is used.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type config struct {
	url         string
	key         string
	project     string
	events      int
	concurrency int
	listen      string
	advertise   string
	failRate    float64
	latency     time.Duration
	timeout     time.Duration
	payloadSize int
}

func main() {
	var c config
	flag.StringVar(&c.url, "url", "http://127.0.0.1:8090", "base URL of the Hooklane API")
	flag.StringVar(&c.key, "key", "", "API key (default: $HOOKLANE_API_KEY)")
	flag.StringVar(&c.project, "project", "", "project ID to send events to")
	flag.IntVar(&c.events, "events", 1000, "number of events to ingest")
	flag.IntVar(&c.concurrency, "concurrency", 16, "concurrent ingest requests")
	flag.StringVar(&c.listen, "listen", "0.0.0.0:0", "receiver listen address (port 0 picks a free port)")
	flag.StringVar(&c.advertise, "advertise", "127.0.0.1", "host name the worker uses to reach the receiver")
	flag.Float64Var(&c.failRate, "fail-rate", 0, "fraction of receiver responses that are 500 (0..1); Hooklane retries those after its retry delay")
	flag.DurationVar(&c.latency, "latency", 0, "artificial receiver response latency")
	flag.DurationVar(&c.timeout, "timeout", 5*time.Minute, "give up waiting for deliveries after this long")
	flag.IntVar(&c.payloadSize, "payload-bytes", 256, "approximate payload size")
	flag.Parse()

	if c.key == "" {
		c.key = os.Getenv("HOOKLANE_API_KEY")
	}
	if c.key == "" || c.project == "" || c.events < 1 || c.concurrency < 1 || c.failRate < 0 || c.failRate > 1 {
		fmt.Fprintln(os.Stderr, "usage: loadtest -key KEY -project ID [-url URL] [-events N] [-concurrency C] ...")
		fmt.Fprintln(os.Stderr, "(the key may also come from HOOKLANE_API_KEY)")
		flag.PrintDefaults()
		os.Exit(2)
	}
	c.url = strings.TrimRight(c.url, "/")

	if err := run(c); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}

var runID = strconv.FormatInt(time.Now().UnixNano(), 36)

// receiver records every request it gets, keyed by the sequence number the
// test put in the event payload.
type receiver struct {
	cfg      config
	mu       sync.Mutex
	firstAt  []time.Time // first 2xx per seq
	okCount  []int32     // 2xx responses per seq
	rng      *rand.Rand
	requests atomic.Int64
	failed   atomic.Int64
	unknown  atomic.Int64
}

func newReceiver(c config) *receiver {
	return &receiver{
		cfg:     c,
		firstAt: make([]time.Time, c.events),
		okCount: make([]int32, c.events),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	now := time.Now()
	r.requests.Add(1)
	body, _ := io.ReadAll(io.LimitReader(req.Body, 1<<20))

	var msg struct {
		Data struct {
			Run string `json:"run"`
			Seq *int   `json:"seq"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &msg) != nil || msg.Data.Seq == nil || msg.Data.Run != runID ||
		*msg.Data.Seq < 0 || *msg.Data.Seq >= r.cfg.events {
		r.unknown.Add(1)
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.cfg.latency > 0 {
		time.Sleep(r.cfg.latency)
	}
	if r.cfg.failRate > 0 {
		r.mu.Lock()
		fail := r.rng.Float64() < r.cfg.failRate
		r.mu.Unlock()
		if fail {
			r.failed.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	seq := *msg.Data.Seq
	r.mu.Lock()
	if r.okCount[seq] == 0 {
		r.firstAt[seq] = now
	}
	r.okCount[seq]++
	r.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

type api struct {
	cfg    config
	client *http.Client
}

// do sends a JSON request and returns the HTTP status (0 on transport errors).
func (a *api) do(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.cfg.url+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.key)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

type stats struct {
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Succeeded  int64 `json:"succeeded"`
	DeadLetter int64 `json:"dead_letter"`
}

func run(c config) error {
	ctx := context.Background()

	rcv := newReceiver(c)
	ln, err := net.Listen("tcp", c.listen)
	if err != nil {
		return fmt.Errorf("receiver listen: %w", err)
	}
	srv := &http.Server{Handler: rcv, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	receiverURL := fmt.Sprintf("http://%s:%d/hook", c.advertise, port)

	a := &api{cfg: c, client: &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{MaxIdleConnsPerHost: c.concurrency * 2},
	}}

	fmt.Printf("target        %s (project %s)\n", c.url, c.project)
	fmt.Printf("receiver      %s (fail rate %.2f, latency %s)\n", receiverURL, c.failRate, c.latency)

	var ep struct {
		ID string `json:"id"`
	}
	if _, err := a.do(ctx, "POST", "/v1/projects/"+c.project+"/endpoints", map[string]any{"url": receiverURL}, &ep); err != nil {
		return fmt.Errorf("create endpoint (a local receiver needs APP_ENV=development on the API): %w", err)
	}
	// Stop the endpoint from receiving retries after the test, however it ends.
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := a.do(cctx, "PATCH", "/v1/endpoints/"+ep.ID, map[string]any{"enabled": false}, nil); err != nil {
			fmt.Fprintln(os.Stderr, "warning: could not disable the test endpoint:", err)
		}
	}()

	// --- ingest ---
	pad := strings.Repeat("x", max(0, c.payloadSize-64))
	sentAt := make([]time.Time, c.events)
	var ingestErrs, throttled atomic.Int64
	var firstErr atomic.Value
	jobs := make(chan int)
	var wg sync.WaitGroup
	ingestStart := time.Now()
	for w := 0; w < c.concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := range jobs {
				payload := map[string]any{"run": runID, "seq": seq, "pad": pad}
				for attempt := 0; ; attempt++ {
					sentAt[seq] = time.Now()
					code, err := a.do(ctx, "POST", "/v1/projects/"+c.project+"/events",
						map[string]any{"event_type": "loadtest.event", "payload": payload}, nil)
					if err == nil {
						break
					}
					if code == http.StatusTooManyRequests && attempt < 50 {
						throttled.Add(1)
						time.Sleep(200 * time.Millisecond)
						continue
					}
					ingestErrs.Add(1)
					firstErr.CompareAndSwap(nil, err.Error())
					sentAt[seq] = time.Time{}
					break
				}
			}
		}()
	}
	for i := 0; i < c.events; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	ingestDur := time.Since(ingestStart)
	accepted := c.events - int(ingestErrs.Load())

	// --- wait until nothing is pending or processing ---
	deadline := time.Now().Add(c.timeout)
	var cur stats
	timedOut := false
	for {
		if _, err := a.do(ctx, "GET", "/v1/projects/"+c.project+"/delivery-stats", nil, &cur); err != nil {
			return fmt.Errorf("delivery-stats: %w", err)
		}
		if cur.Pending == 0 && cur.Processing == 0 {
			break
		}
		if time.Now().After(deadline) {
			timedOut = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	// A receiver hit can be recorded just after the worker stored the result.
	time.Sleep(500 * time.Millisecond)

	// --- report ---
	var lat []time.Duration
	var missing, duplicates, seen int
	var lastSeen time.Time
	rcv.mu.Lock()
	for seq := 0; seq < c.events; seq++ {
		if sentAt[seq].IsZero() {
			continue // never accepted
		}
		n := rcv.okCount[seq]
		if n == 0 {
			missing++
			continue
		}
		seen++
		duplicates += int(n) - 1
		lat = append(lat, rcv.firstAt[seq].Sub(sentAt[seq]))
		if rcv.firstAt[seq].After(lastSeen) {
			lastSeen = rcv.firstAt[seq]
		}
	}
	rcv.mu.Unlock()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })

	fmt.Println()
	fmt.Printf("ingest        %d accepted, %d failed in %s -> %.0f events/s (concurrency %d, %d throttled retries)\n",
		accepted, ingestErrs.Load(), ingestDur.Round(time.Millisecond), float64(accepted)/ingestDur.Seconds(), c.concurrency, throttled.Load())
	if v := firstErr.Load(); v != nil {
		fmt.Printf("first ingest error: %v\n", v)
	}
	if seen > 0 {
		span := lastSeen.Sub(ingestStart)
		fmt.Printf("delivery      %d/%d delivered; first send -> last delivery %s -> %.0f deliveries/s end to end\n",
			seen, accepted, span.Round(time.Millisecond), float64(seen)/span.Seconds())
		fmt.Printf("latency       p50 %s  p95 %s  p99 %s  max %s   (ingest request sent -> first successful receiver hit)\n",
			pct(lat, 50), pct(lat, 95), pct(lat, 99), lat[len(lat)-1].Round(time.Millisecond))
	}
	fmt.Printf("receiver      %d requests, %d injected failures, %d unrelated\n", rcv.requests.Load(), rcv.failed.Load(), rcv.unknown.Load())
	fmt.Printf("correctness   missing %d, duplicate deliveries %d (at-least-once allows duplicates, never missing)\n", missing, duplicates)
	fmt.Printf("hooklane      project totals: pending %d, processing %d, succeeded %d, dead_letter %d\n",
		cur.Pending, cur.Processing, cur.Succeeded, cur.DeadLetter)

	switch {
	case timedOut:
		return errors.New("timed out waiting for deliveries to finish")
	case ingestErrs.Load() > 0:
		return errors.New("some events were not accepted")
	case missing > 0:
		return fmt.Errorf("%d events were never delivered successfully", missing)
	}
	return nil
}

func pct(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := (len(sorted)*p + 99) / 100
	if i < 1 {
		i = 1
	}
	return sorted[i-1].Round(time.Millisecond)
}
