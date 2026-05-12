package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gmodzelewski/artemis-handbrake/pkg/jolokia"
)

type webhookPayload struct {
	Status string  `json:"status"`
	Alerts []alert `json:"alerts"`
}

type alert struct {
	Status string            `json:"status"`
	Labels map[string]string `json:"labels"`
}

var (
	jolokiaMu sync.Mutex
)

func main() {
	addr := getenv("LISTEN_ADDR", ":8080")
	jURL := strings.TrimSpace(os.Getenv("JOLOKIA_URL"))
	jUser := os.Getenv("JOLOKIA_USER")
	jPass := os.Getenv("JOLOKIA_PASSWORD")
	jOrigin := os.Getenv("JOLOKIA_ORIGIN")
	broker := os.Getenv("BROKER_NAME")
	artAddr := os.Getenv("ARTEMIS_ADDRESS")
	pauseName := getenv("ALERT_PAUSE_NAME", "WorkloadMemoryHigh")
	resumeName := getenv("ALERT_RESUME_NAME", "WorkloadMemoryLow")

	if jURL == "" || broker == "" || artAddr == "" {
		log.Fatal("JOLOKIA_URL, BROKER_NAME, ARTEMIS_ADDRESS are required")
	}

	client := jolokia.New(jURL, jUser, jPass, jOrigin)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var p webhookPayload
		if err := json.Unmarshal(body, &p); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		var lastErr error
		for _, a := range p.Alerts {
			if a.Status != "firing" {
				continue
			}
			name := a.Labels["alertname"]
			var op string
			switch name {
			case pauseName:
				op = "pause"
			case resumeName:
				op = "resume"
			default:
				continue
			}
			jolokiaMu.Lock()
			err := client.Exec(op, broker, artAddr)
			jolokiaMu.Unlock()
			if err != nil {
				log.Printf("jolokia %s failed: %v", op, err)
				lastErr = err
			} else {
				log.Printf("jolokia %s ok (alert=%s)", op, name)
			}
		}
		if lastErr != nil {
			http.Error(w, lastErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	log.Printf("listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
