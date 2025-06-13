package main

import "github.com/google/uuid"
import (
	// "context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"github.com/jzetterman/chirpy/internal/database"
	_ "github.com/lib/pq"
	// "github.com/tetratelabs/wazero/api"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	platform       string
	database       *database.Queries
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

// Main handler for front page
func handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

// Middleware to handle our metrics
func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

// Handler for adding new users
func (cfg *apiConfig) createNewUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	user, err := cfg.database.CreateUser(r.Context(), params.Email)
	if err != nil {
		log.Printf("Error creating user: %s", err)
	}

	respUser := User{}
	respUser.Email = user.Email
	respUser.CreatedAt = user.CreatedAt
	respUser.UpdatedAt = user.UpdatedAt
	respUser.ID = user.ID

	respondWithJSON(w, 201, respUser)
}

// Reporting handler
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

// Reset handler resets hit counter
func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		respondWithError(w, 403, "Forbidden")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Swap(0)
	err := cfg.database.DeleteAllUsers(r.Context())
	hitCount := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())
	_, err = w.Write([]byte(hitCount))
	if err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

// chirpsGetHandler() retrieves chirps using a GET request
func (cfg *apiConfig) chirpsGetHandler(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.database.GetChirps(r.Context())
	if err != nil {
		log.Printf("Error retrieving data from database: %s", err)
	}

	respChirps := []Chirp{}
	for _, chirp := range chirps {
		temp := Chirp{}
		temp.ID = chirp.ID
		temp.CreatedAt = chirp.CreatedAt
		temp.UpdatedAt = chirp.UpdatedAt
		temp.Body = chirp.Body
		temp.UserID = chirp.UserID
		respChirps = append(respChirps, temp)
	}

	respondWithJSON(w, 200, respChirps)
}

// chirpsPostHandler() accepts 140 character string to be posted
func (cfg *apiConfig) chirpsPostHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body    string    `json:"body"`
		User_ID uuid.UUID `json:"user_id"`
	}

	type response struct {
		Cleaned_Body string `json:"cleaned_body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	resp := response{}

	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	resp.Cleaned_Body = badWordReplacer(params.Body)
	dbParams := database.CreateChirpParams{
		Body:   resp.Cleaned_Body,
		UserID: params.User_ID,
	}

	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
	} else {
		chirp, err := cfg.database.CreateChirp(r.Context(), dbParams)
		if err != nil {
			log.Printf("Error creating chirp: %s", err)
			w.WriteHeader(500)
			return
		}

		respChirp := Chirp{}
		respChirp.ID = chirp.ID
		respChirp.CreatedAt = chirp.CreatedAt
		respChirp.UpdatedAt = chirp.UpdatedAt
		respChirp.Body = chirp.Body
		respChirp.UserID = chirp.UserID

		respondWithJSON(w, 201, respChirp)
	}
}

// Bad word replacer looks for bad words and replaces them
func badWordReplacer(msg string) string {
	replacements := map[string]string{
		"kerfuffle": "****",
		"sharbert":  "****",
		"fornax":    "****",
	}

	cleanedWords := []string{}
	individualWords := strings.Split(msg, " ")
	for _, word := range individualWords {
		if value, exists := replacements[strings.ToLower(word)]; exists {
			cleanedWords = append(cleanedWords, value)
		} else {
			cleanedWords = append(cleanedWords, word)
		}
	}

	return strings.Join(cleanedWords, " ")
}

// Handler for responding with errors
func respondWithError(w http.ResponseWriter, code int, msg string) {
	type returnVals struct {
		Error string `json:"error"`
	}

	errorJSON := returnVals{
		Error: msg,
	}

	data, err := json.Marshal(errorJSON)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err = w.Write(data)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

// Hanlder for responding with JSON to requests
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err = w.Write(data)
	if err != nil {
		log.Printf("Error writing response: %s", err)
		return
	}
}

func main() {
	// Set all prerequisities:
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading environment variables: %s", err)
	}

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Printf("Error connecting to database: %s", err)
	}
	dbQueries := database.New(db)

	apiCfg := &apiConfig{
		database: dbQueries,
		platform: platform,
	}
	apiCfg.database = dbQueries

	// ------------------ Route Handlers ------------------
	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))
	mux.Handle("GET /admin/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.reportingHandler(w, r)
	}))
	mux.Handle("GET /api/chirps", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.chirpsGetHandler(w, r)
	}))
	mux.Handle("POST /admin/reset", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.resetHandler(w, r)
	}))
	mux.Handle("POST /api/users", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.createNewUser(w, r)
	}))
	mux.Handle("POST /api/chirps", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.chirpsPostHandler(w, r)
	}))

	mux.HandleFunc("GET /api/healthz", handler)

	server := http.Server{Handler: mux, Addr: ":8080"}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
