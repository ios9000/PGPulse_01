package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

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

// loginResponse is the JSON body returned on successful login.
type loginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         userResponse `json:"user"`
}

// userResponse is the JSON representation of a user.
type userResponse struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Active      bool     `json:"active"`
	Permissions []string `json:"permissions"`
}

// registerRequest is the JSON body for POST /api/v1/auth/register.
type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// updateUserRequest is the JSON body for PUT /api/v1/auth/users/{id}.
type updateUserRequest struct {
	Role   *string `json:"role,omitempty"`
	Active *bool   `json:"active,omitempty"`
}

// changePasswordRequest is the JSON body for PUT /api/v1/auth/me/password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
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

	if !user.Active {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "account deactivated")
		return
	}

	if err := s.userStore.UpdateLastLogin(r.Context(), user.ID); err != nil {
		s.logger.Warn("failed to update last login", "user_id", user.ID, "err", err)
	}

	pair, err := s.jwtService.GenerateTokenPair(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate tokens")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		User: userResponse{
			ID:          user.ID,
			Username:    user.Username,
			Role:        user.Role,
			Active:      user.Active,
			Permissions: auth.PermissionsForRole(auth.Role(user.Role)),
		},
	})
}

func (s *APIServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	claims, err := s.jwtService.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid refresh token")
		return
	}

	// Fetch current user to pick up any role changes since the refresh token was issued.
	user, err := s.userStore.GetByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found")
		return
	}

	if !user.Active {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "account deactivated")
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
		ID:          claims.UserID,
		Username:    claims.Username,
		Role:        claims.Role,
		Active:      true,
		Permissions: auth.PermissionsForRole(auth.Role(claims.Role)),
	})
}

func (s *APIServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.userStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list users")
		return
	}
	result := make([]userResponse, len(users))
	for i, u := range users {
		result[i] = userResponse{
			ID:          u.ID,
			Username:    u.Username,
			Role:        u.Role,
			Active:      u.Active,
			Permissions: auth.PermissionsForRole(auth.Role(u.Role)),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

func (s *APIServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "username, password, and role are required")
		return
	}
	if !auth.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid role")
		return
	}
	// Only super_admin can create super_admin or roles_admin users.
	claims := auth.ClaimsFromContext(r.Context())
	if claims != nil && auth.Role(claims.Role) != auth.RoleSuperAdmin {
		if req.Role == string(auth.RoleSuperAdmin) || req.Role == string(auth.RoleRolesAdmin) {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "only super_admin can create privileged users")
			return
		}
	}
	hash, err := auth.HashPassword(req.Password, s.authCfg.BcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}
	user, err := s.userStore.Create(r.Context(), req.Username, hash, req.Role)
	if err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", "username already exists")
		return
	}
	writeJSON(w, http.StatusCreated, userResponse{
		ID:          user.ID,
		Username:    user.Username,
		Role:        user.Role,
		Active:      user.Active,
		Permissions: auth.PermissionsForRole(auth.Role(user.Role)),
	})
}

func (s *APIServer) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid user ID")
		return
	}
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	// Prevent self-deactivation.
	if claims != nil && claims.UserID == id && req.Active != nil && !*req.Active {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "cannot deactivate your own account")
		return
	}
	// Only super_admin can promote to super_admin or roles_admin.
	if req.Role != nil {
		if !auth.ValidRole(*req.Role) {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid role")
			return
		}
		if claims != nil && auth.Role(claims.Role) != auth.RoleSuperAdmin {
			if *req.Role == string(auth.RoleSuperAdmin) || *req.Role == string(auth.RoleRolesAdmin) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "only super_admin can assign privileged roles")
				return
			}
		}
	}
	if err := s.userStore.Update(r.Context(), id, auth.UpdateFields{Role: req.Role, Active: req.Active}); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *APIServer) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "current_password and new_password are required")
		return
	}
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	user, err := s.userStore.GetByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch user")
		return
	}
	if err := auth.CheckPassword(req.CurrentPassword, user.PasswordHash); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword, s.authCfg.BcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}
	if err := s.userStore.UpdatePassword(r.Context(), claims.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}
