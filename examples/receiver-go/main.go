// Command receiver-go is a minimal webhook receiver that verifies Hooklane
// signatures and deduplicates retries by webhook-id.
//
//	HOOKLANE_SECRET=whsec_... go run . -addr :9000
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type webhook struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9000", "listen address")
	flag.Parse()
	secret := os.Getenv("HOOKLANE_SECRET")
	if secret == "" {
		log.Fatal("HOOKLANE_SECRET is required (the endpoint's whsec_ secret)")
	}

	// Delivery is at-least-once: remember processed webhook-ids. Use a
	// database with a unique constraint in a real service.
	var mu sync.Mutex
	seen := map[string]bool{}

	http.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := verify(secret, r.Header, body, time.Now()); err != nil {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		var event webhook
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		mu.Lock()
		duplicate := seen[event.ID]
		seen[event.ID] = true
		mu.Unlock()
		if duplicate {
			log.Printf("duplicate %s (%s) ignored", event.ID, event.Type)
		} else {
			log.Printf("received %s (%s): %s", event.ID, event.Type, event.Data)
		}
		// Any 2xx acknowledges the delivery. Answer quickly and do slow work
		// asynchronously so the request does not time out and get retried.
		w.WriteHeader(http.StatusNoContent)
	})

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
