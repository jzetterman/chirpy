package main

import (
	"encoding/json"
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	hitCount := fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())
	_, err := w.Write([]byte(hitCount))
	if err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

func chirpHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	type returnVals struct {
		Error string `json:"error"`
		Valid bool   `json:"valid"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	respCode := 200
	respBody := returnVals{
		Error: "",
		Valid: true,
	}

	if len(params.Body) > 140 {
		respBody = returnVals{
			Error: "Chirp is too long",
			Valid: false,
		}
		respCode = 400
	}

	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(respCode)
	_, err = w.Write(data)
	if err != nil {
		log.Printf("Error writing response: %s", err)
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
	mux.Handle("GET /admin/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.reportingHandler(w, r)
	}))
	mux.Handle("POST /admin/reset", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.resetHandler(w, r)
	}))

	mux.HandleFunc("POST /api/validate_chirp", chirpHandler)
	mux.HandleFunc("GET /api/healthz", handler)

	server := http.Server{Handler: mux, Addr: ":8080"}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
