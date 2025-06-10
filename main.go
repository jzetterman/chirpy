package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

func (cfg *apiConfig) reportingHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	hitCount := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())
	_, err := w.Write([]byte(hitCount))
	if err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Swap(0)
	hitCount := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())
	_, err := w.Write([]byte(hitCount))
	if err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

func main() {
	apiCfg := &apiConfig{}
	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))
	mux.Handle("GET /api/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.reportingHandler(w, r)
	}))
	mux.Handle("POST /api/reset", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.resetHandler(w, r)
	}))

	mux.HandleFunc("GET /api/healthz", handler)

	server := http.Server{Handler: mux, Addr: ":8080"}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
