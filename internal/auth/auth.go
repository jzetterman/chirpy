package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		return "", err
	}

	return string(hashedPassword), nil
}

func CheckPasswordHash(hash, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return err
	}
	return nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := &jwt.RegisteredClaims{
		Issuer:    "chirpy",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		log.Printf("Error signing token: %s", err)
		return "", err
	}

	return signedToken, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})

	if err != nil {
		return uuid.UUID{}, err
	}

	if !token.Valid {
		return uuid.UUID{}, err
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.UUID{}, err
	}

	return userID, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")

	if authHeader == "" {
		log.Printf("authorization header is missing")
		return "", errors.New("authorization header is missing")
	}

	ok := strings.HasPrefix(authHeader, "Bearer ")
	if !ok {
		log.Printf("authorization header missing or malformed")
		return "", errors.New("authorization header missing or malformed")
	}

	token := strings.SplitN(authHeader, " ", 2)

	if len(token) != 2 || strings.TrimSpace(token[1]) == "" {
		log.Printf("token was missing from authorization header")
		return "", errors.New("token was missing from authorization header")
	}

	return token[1], nil
}

func MakeRefreshToken() (string, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return "", errors.New("Error generating refresh token")
	}

	encodedToken := hex.EncodeToString(key)

	return encodedToken, nil
}

func GetAPIKey(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")

	if authHeader == "" {
		log.Printf("authorization header is missing")
		return "", errors.New("authorization header is missing")
	}

	ok := strings.HasPrefix(authHeader, "ApiKey ")
	if !ok {
		log.Printf("authorization header missing or malformed")
		return "", errors.New("authorization header missing or malformed")
	}

	token := strings.SplitN(authHeader, " ", 2)

	if len(token) != 2 || strings.TrimSpace(token[1]) == "" {
		log.Printf("token was missing from authorization header")
		return "", errors.New("token was missing from authorization header")
	}

	return token[1], nil
}
