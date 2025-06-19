package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/jzetterman/chirpy/internal/auth"
	"github.com/jzetterman/chirpy/internal/database"

	_ "github.com/lib/pq"
)

// "github.com/tetratelabs/wazero/api"

type apiConfig struct {
	fileserverHits atomic.Int32
	platform       string
	database       *database.Queries
	secret         string
}

type User struct {
	ID               uuid.UUID `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Email            string    `json:"email"`
	ExpiresInSeconds int       `json:"expires_in_seconds"`
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
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Error hasing password: %s", err)
	}

	userArgs := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}

	user, err := cfg.database.CreateUser(r.Context(), userArgs)
	if err != nil {
		log.Printf("Error creating user: %s", err)
	}

	respUser := User{}
	respUser.ID = user.ID
	respUser.CreatedAt = user.CreatedAt
	respUser.UpdatedAt = user.UpdatedAt
	respUser.Email = user.Email

	respondWithJSON(w, 201, respUser)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		ExpiresInSeconds *int   `json:"expires_in_seconds"`
	}

	type finalUser struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
		Token     string    `json:"token"`
	}

	tokenExpirationInSeconds := 0

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}

	authenticated, err := cfg.authenticateUser(r.Context(), params.Email, params.Password)
	if err != nil {
		log.Printf("User was not successfully authenticated: %s", err)
		respondWithError(w, 401, "Authentication failed")
		return
	}

	if params.ExpiresInSeconds == nil {
		tokenExpirationInSeconds = 3600
	} else {
		if *params.ExpiresInSeconds > 3600 {
			tokenExpirationInSeconds = 3600
		} else {
			tokenExpirationInSeconds = *params.ExpiresInSeconds
		}
	}

	jwt, err := auth.MakeJWT(authenticated.ID, cfg.secret, time.Duration(tokenExpirationInSeconds)*time.Second)
	if err != nil {
		log.Printf("Error creating JWT token: %s", err)
		respondWithError(w, 500, fmt.Sprintf("Error creating JWT token: %s", err))
		return
	}

	userResp := finalUser{
		ID:        authenticated.ID,
		CreatedAt: authenticated.CreatedAt,
		UpdatedAt: authenticated.UpdatedAt,
		Email:     authenticated.Email,
		Token:     jwt,
	}
	respondWithJSON(w, 200, userResp)

}

// Authenticate user method
func (cfg *apiConfig) authenticateUser(ctx context.Context, email, password string) (User, error) {
	user, err := cfg.database.GetUserByEmail(ctx, email)
	if err != nil {
		log.Printf("Error retrieving user: %s", err)
		return User{}, errors.New("user not found")
	}

	authCheck := auth.CheckPasswordHash(user.HashedPassword, password)
	if authCheck != nil {
		return User{}, errors.New("incorrect email or password")
	}

	respUser := User{}
	respUser.ID = user.ID
	respUser.CreatedAt = user.CreatedAt
	respUser.UpdatedAt = user.UpdatedAt
	respUser.Email = user.Email
	return respUser, nil
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
	if err != nil {
		log.Printf("There was an error deleting users: %s", err)
	}
	hitCount := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())
	_, err = w.Write([]byte(hitCount))
	if err != nil {
		log.Printf("error while writing response: %v", err)
	}
}

// chirpsGetHandler() retrieves chirps using a GET request
func (cfg *apiConfig) chirpsGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("chirpID") != "" {
		chirpUUID, err := uuid.Parse(r.PathValue("chirpID"))
		if err != nil {
			log.Printf("Error parsing UUID: %s", r.PathValue("chirpID"))
			respondWithError(w, 400, fmt.Sprintf("Error parsing UUID: %s", r.PathValue("chirpID")))
			return
		}

		chirp, err := cfg.database.GetOnehirp(r.Context(), chirpUUID)
		if err == sql.ErrNoRows {
			respondWithError(w, 404, fmt.Sprintf("No results found for UUID: %s", chirpUUID))
			return
		} else if err != nil {
			log.Printf("Error retrieving data from database: %s", err)
			respondWithError(w, 500, fmt.Sprintf("Error retrieving data from database: %s", err))
			return
		}

		respChirp := Chirp{}
		respChirp.ID = chirp.ID
		respChirp.CreatedAt = chirp.CreatedAt
		respChirp.UpdatedAt = chirp.UpdatedAt
		respChirp.Body = chirp.Body
		respChirp.UserID = chirp.UserID
		respondWithJSON(w, 200, respChirp)
		return
	}

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
		Body string `json:"body"`
	}

	type response struct {
		CleanedBody string `json:"cleaned_body"`
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

	if len(params.Body) > 140 {
		log.Printf("Chirp is more than 140 chars")
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error retrieving the token: %s", err)
		respondWithError(w, 401, "Unauthorized")
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		log.Printf("Error validating the JWT token: %s", err)
		respondWithError(w, 401, "Unauthorized")
		return
	}

	resp.CleanedBody = badWordReplacer(params.Body)
	dbParams := database.CreateChirpParams{
		Body:   resp.CleanedBody,
		UserID: userID,
	}

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
	secret := os.Getenv("SECRET")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Printf("Error connecting to database: %s", err)
	}
	dbQueries := database.New(db)

	apiCfg := &apiConfig{
		database: dbQueries,
		platform: platform,
		secret:   secret,
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
	mux.Handle("GET /api/chirps/{chirpID}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.chirpsGetHandler(w, r)
	}))
	mux.Handle("POST /admin/reset", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.resetHandler(w, r)
	}))
	mux.Handle("POST /api/users", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.createNewUser(w, r)
	}))
	mux.Handle("POST /api/login", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.loginHandler(w, r)
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
