package main

import (
	"database/sql"
	"net/http"

	"github.com/google/uuid"
	"github.com/jzetterman/chirpy/internal/auth"
	"github.com/jzetterman/chirpy/internal/database"
)

func (cfg *apiConfig) deleteChirpHandler(w http.ResponseWriter, r *http.Request) {
	chirpIDString := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid chirp ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Error validating token", err)
		return
	}

	authedUserID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unable to verify token", err)
		return
	}

	// decoder := json.NewDecoder(r.Body)
	// params := parameters{}
	// err = decoder.Decode(&params)
	// if err != nil {
	// 	respondWithError(w, http.StatusInternalServerError, "Couldn't decode the provided parameters", err)
	// 	return
	// }

	chirp, err := cfg.database.GetOneChirp(r.Context(), chirpID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithError(w, http.StatusNotFound, "No chirp found with provided ID", err)
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Query completed unsuccessfully", err)
		return
	}

	if chirp.UserID != authedUserID {
		respondWithError(w, http.StatusForbidden, "User didn't create Chirp", err)
		return
	}

	_, err = cfg.database.DeleteChirp(r.Context(), database.DeleteChirpParams{
		UserID: authedUserID,
		ID:     chirpID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Delete operation failed", err)
		return
	}

	respondWithJSON(w, http.StatusNoContent, Chirp{})
}
