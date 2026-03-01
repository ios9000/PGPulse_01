package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType distinguishes access tokens from refresh tokens.
type TokenType string

const (
	TokenAccess  TokenType = "access"
	TokenRefresh TokenType = "refresh"
)

// Claims are the JWT payload for PGPulse tokens.
type Claims struct {
	jwt.RegisteredClaims
	UserID   int64     `json:"uid"`
	Username string    `json:"usr"`
	Role     string    `json:"role"`
	Type     TokenType `json:"type"`
}

// TokenPair holds the access and refresh tokens returned on login.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until access token expires
}

// JWTService handles token generation and validation.
type JWTService struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewJWTService creates a JWTService with the given signing secret and TTLs.
func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService {
	return &JWTService{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

// AccessTokenTTL returns the configured access token lifetime.
func (s *JWTService) AccessTokenTTL() time.Duration { return s.accessTokenTTL }

// GenerateTokenPair creates both an access token and a refresh token for a user.
func (s *JWTService) GenerateTokenPair(user *User) (*TokenPair, error) {
	now := time.Now()

	accessToken, err := s.generateToken(user, TokenAccess, now, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.generateToken(user, TokenRefresh, now, s.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}

// GenerateAccessToken creates a new access token only (used by the refresh flow).
func (s *JWTService) GenerateAccessToken(user *User) (string, error) {
	return s.generateToken(user, TokenAccess, time.Now(), s.accessTokenTTL)
}

func (s *JWTService) generateToken(user *User, tokenType TokenType, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "pgpulse",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Type:     tokenType,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateToken parses and validates a JWT string. Returns claims on success.
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
