package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestMakeJWTAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	token, err := MakeJWT(userID, "test", time.Minute)
	if err != nil {
		t.Fatalf("failed to create JWT: %s", err)
	}

	gotUserID, err := ValidateJWT(token, "test")
	if err != nil {
		t.Fatalf("failed to validate JWT: %s", err)
	}

	if userID != gotUserID {
		t.Errorf("expected %v, got %v", userID, gotUserID)
	}
}

func TestExpiredJWTIsRejected(t *testing.T) {
	userID := uuid.New()
	expiredDuration := -1 * time.Second
	token, err := MakeJWT(userID, "test", expiredDuration)
	if err != nil {
		t.Fatalf("failed to create JWT: %s", err)
	}

	_, err = ValidateJWT(token, "test")
	if err == nil {
		t.Fatalf("expected error validating expired JWT, got nil")
	}

	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Errorf("expected jwt.ErrTokenExpired, got %v", err)
	}
}
