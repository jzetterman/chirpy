package main

import (
	"log"
	"net/http"
)

func handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

func main() {
	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))

	mux.HandleFunc("/healthz", handler)

	server := http.Server{Handler: mux, Addr: ":8080"}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
