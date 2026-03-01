package auth

import (
	"testing"
	"time"
)

func newTestJWT() *JWTService {
	return NewJWTService("test-secret-that-is-at-least-32-chars!", 15*time.Minute, 7*24*time.Hour)
}

func testUser() *User {
	return &User{ID: 1, Username: "alice", Role: RoleAdmin}
}

func TestGenerateTokenPair_Valid(t *testing.T) {
	svc := newTestJWT()
	pair, err := svc.GenerateTokenPair(testUser())
	if err != nil {
		t.Fatalf("GenerateTokenPair error: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("AccessToken is empty")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshToken is empty")
	}
	if pair.ExpiresIn != int64((15 * time.Minute).Seconds()) {
		t.Errorf("ExpiresIn = %d, want %d", pair.ExpiresIn, int64((15*time.Minute).Seconds()))
	}
}

func TestValidateToken_Valid(t *testing.T) {
	svc := newTestJWT()
	pair, _ := svc.GenerateTokenPair(testUser())

	claims, err := svc.ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}
	if claims.Username != "alice" {
		t.Errorf("Username = %q, want %q", claims.Username, "alice")
	}
	if claims.Role != RoleAdmin {
		t.Errorf("Role = %q, want %q", claims.Role, RoleAdmin)
	}
	if claims.Type != TokenAccess {
		t.Errorf("Type = %q, want %q", claims.Type, TokenAccess)
	}
}

func TestValidateToken_Expired(t *testing.T) {
	// TTL in the past → token already expired.
	svc := NewJWTService("test-secret-that-is-at-least-32-chars!", -time.Second, time.Hour)
	pair, _ := svc.GenerateTokenPair(testUser())

	_, err := svc.ValidateToken(pair.AccessToken)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestValidateToken_WrongSigningMethod(t *testing.T) {
	// A token signed with a different secret should fail.
	svc1 := NewJWTService("test-secret-that-is-at-least-32-chars!", time.Hour, time.Hour)
	svc2 := NewJWTService("different-secret-at-least-32-characters!!", time.Hour, time.Hour)

	pair, _ := svc1.GenerateTokenPair(testUser())
	_, err := svc2.ValidateToken(pair.AccessToken)
	if err == nil {
		t.Error("expected error for mismatched secret, got nil")
	}
}

func TestValidateToken_AccessVsRefreshType(t *testing.T) {
	svc := newTestJWT()
	pair, _ := svc.GenerateTokenPair(testUser())

	// Refresh token must parse successfully and have the refresh type.
	claims, err := svc.ValidateToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateToken(refresh) error: %v", err)
	}
	if claims.Type != TokenRefresh {
		t.Errorf("Type = %q, want %q", claims.Type, TokenRefresh)
	}
}

func TestValidateToken_InvalidString(t *testing.T) {
	svc := newTestJWT()
	_, err := svc.ValidateToken("not.a.jwt")
	if err == nil {
		t.Error("expected error for invalid token string, got nil")
	}
}

func TestAccessTokenTTL(t *testing.T) {
	ttl := 30 * time.Minute
	svc := NewJWTService("test-secret-that-is-at-least-32-chars!", ttl, time.Hour)
	if svc.AccessTokenTTL() != ttl {
		t.Errorf("AccessTokenTTL() = %v, want %v", svc.AccessTokenTTL(), ttl)
	}
}
