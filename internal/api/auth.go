package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ios9000/PGPulse_01/internal/auth"
)

// loginRequest is the JSON body for POST /api/v1/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// refreshRequest is the JSON body for POST /api/v1/auth/refresh.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// userResponse is the JSON body for GET /api/v1/auth/me.
type userResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (s *APIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "username and password are required")
		return
	}

	user, err := s.userStore.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			if s.rateLimiter != nil {
				s.rateLimiter.RecordFailure(auth.ClientIP(r))
			}
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "authentication failed")
		return
	}

	if err := auth.CheckPassword(req.Password, user.PasswordHash); err != nil {
		if s.rateLimiter != nil {
			s.rateLimiter.RecordFailure(auth.ClientIP(r))
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}

	pair, err := s.jwtService.GenerateTokenPair(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate tokens")
		return
	}

	writeJSON(w, http.StatusOK, pair)
}

func (s *APIServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	claims, err := s.jwtService.ValidateToken(req.RefreshToken)
	if err != nil || claims.Type != auth.TokenRefresh {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid refresh token")
		return
	}

	// Fetch current user to pick up any role changes since the refresh token was issued.
	user, err := s.userStore.GetByUsername(r.Context(), claims.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found")
		return
	}

	accessToken, err := s.jwtService.GenerateAccessToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"expires_in":   int64(s.jwtService.AccessTokenTTL().Seconds()),
	})
}

func (s *APIServer) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	writeJSON(w, http.StatusOK, userResponse{
		ID:       claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
	})
}
