package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := ":8161"
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		addr = v
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		log.Printf("%s %s body=%s", r.Method, r.URL.Path, string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request":null,"value":null,"status":200}`))
	})
	s := &http.Server{Addr: addr, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("mock jolokia on %s", addr)
	log.Fatal(s.ListenAndServe())
}
