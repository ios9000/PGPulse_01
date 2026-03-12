package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ios9000/PGPulse_01/internal/auth"
	"github.com/ios9000/PGPulse_01/internal/config"
)

// mockUserStore implements auth.UserStore for unit tests (no DB required).
type mockUserStore struct {
	users   map[string]*auth.User
	usersID map[int64]*auth.User
}

func newMockUserStore(users ...*auth.User) *mockUserStore {
	m := &mockUserStore{
		users:   make(map[string]*auth.User),
		usersID: make(map[int64]*auth.User),
	}
	for _, u := range users {
		m.users[u.Username] = u
		m.usersID[u.ID] = u
	}
	return m
}

func (m *mockUserStore) GetByUsername(_ context.Context, username string) (*auth.User, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserStore) GetByID(_ context.Context, id int64) (*auth.User, error) {
	u, ok := m.usersID[id]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserStore) Create(_ context.Context, username, passwordHash, role string) (*auth.User, error) {
	u := &auth.User{ID: int64(len(m.users) + 1), Username: username, PasswordHash: passwordHash, Role: role, Active: true}
	m.users[username] = u
	m.usersID[u.ID] = u
	return u, nil
}

func (m *mockUserStore) Count(_ context.Context) (int64, error) {
	return int64(len(m.users)), nil
}

func (m *mockUserStore) List(_ context.Context) ([]*auth.User, error) {
	var result []*auth.User
	for _, u := range m.users {
		result = append(result, u)
	}
	return result, nil
}

func (m *mockUserStore) Update(_ context.Context, id int64, fields auth.UpdateFields) error {
	u, ok := m.usersID[id]
	if !ok {
		return auth.ErrUserNotFound
	}
	if fields.Role != nil {
		u.Role = *fields.Role
	}
	if fields.Active != nil {
		u.Active = *fields.Active
	}
	return nil
}

func (m *mockUserStore) UpdatePassword(_ context.Context, id int64, passwordHash string) error {
	u, ok := m.usersID[id]
	if !ok {
		return auth.ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	return nil
}

func (m *mockUserStore) UpdateLastLogin(_ context.Context, _ int64) error {
	return nil
}

func (m *mockUserStore) CountActiveByRole(_ context.Context, role string) (int64, error) {
	var count int64
	for _, u := range m.usersID {
		if u.Role == role && u.Active {
			count++
		}
	}
	return count, nil
}

func (m *mockUserStore) Delete(_ context.Context, id int64) error {
	u, ok := m.usersID[id]
	if !ok {
		return auth.ErrUserNotFound
	}
	delete(m.usersID, id)
	delete(m.users, u.Username)
	return nil
}

// newAuthTestServer creates an APIServer with auth enabled and a mock store.
func newAuthTestServer(t *testing.T, userStore auth.UserStore, jwtSvc *auth.JWTService) *APIServer {
	t.Helper()
	cfg := config.Config{
		Server:    config.ServerConfig{CORSEnabled: false},
		Auth:      config.AuthConfig{Enabled: true, JWTSecret: "test-secret-at-least-32-characters!"},
		Instances: []config.InstanceConfig{{ID: "x", DSN: "postgres://x"}},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, nil, nil, jwtSvc, userStore, logger, nil, nil, nil, nil, nil, false, 0, auth.AuthEnabled)
}

func testJWTSvc() *auth.JWTService {
	return auth.NewJWTService("test-secret-at-least-32-characters!", "test-refresh-secret-at-least-32-chars!", time.Hour, 7*24*time.Hour)
}

// hashedPassword returns the bcrypt hash of "password" at cost 4.
func hashedPassword(t *testing.T) string {
	t.Helper()
	h, err := auth.HashPassword("password", 4)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func postJSON(handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// --- Tests ---

func TestHandleLogin_Success(t *testing.T) {
	jwtSvc := testJWTSvc()
	hash := hashedPassword(t)
	user := &auth.User{ID: 1, Username: "alice", PasswordHash: hash, Role: string(auth.RoleSuperAdmin), Active: true}
	srv := newAuthTestServer(t, newMockUserStore(user), jwtSvc)

	rr := postJSON(srv.Routes(), "/api/v1/auth/login", map[string]string{
		"username": "alice",
		"password": "password",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var resp loginResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("access_token is empty")
	}
	if resp.RefreshToken == "" {
		t.Error("refresh_token is empty")
	}
	if resp.User.Username != "alice" {
		t.Errorf("user.username = %q, want %q", resp.User.Username, "alice")
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	jwtSvc := testJWTSvc()
	hash := hashedPassword(t)
	user := &auth.User{ID: 1, Username: "alice", PasswordHash: hash, Role: string(auth.RoleSuperAdmin), Active: true}
	srv := newAuthTestServer(t, newMockUserStore(user), jwtSvc)

	rr := postJSON(srv.Routes(), "/api/v1/auth/login", map[string]string{
		"username": "alice",
		"password": "wrong",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandleLogin_UnknownUser(t *testing.T) {
	jwtSvc := testJWTSvc()
	srv := newAuthTestServer(t, newMockUserStore(), jwtSvc)

	rr := postJSON(srv.Routes(), "/api/v1/auth/login", map[string]string{
		"username": "nobody",
		"password": "password",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandleLogin_EmptyBody(t *testing.T) {
	jwtSvc := testJWTSvc()
	srv := newAuthTestServer(t, newMockUserStore(), jwtSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleLogin_DeactivatedUser(t *testing.T) {
	jwtSvc := testJWTSvc()
	hash := hashedPassword(t)
	user := &auth.User{ID: 1, Username: "alice", PasswordHash: hash, Role: string(auth.RoleSuperAdmin), Active: false}
	srv := newAuthTestServer(t, newMockUserStore(user), jwtSvc)

	rr := postJSON(srv.Routes(), "/api/v1/auth/login", map[string]string{
		"username": "alice",
		"password": "password",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for deactivated user", rr.Code)
	}
}

func TestHandleRefresh_Valid(t *testing.T) {
	jwtSvc := testJWTSvc()
	hash := hashedPassword(t)
	user := &auth.User{ID: 1, Username: "bob", PasswordHash: hash, Role: string(auth.RoleDBA), Active: true}
	store := newMockUserStore(user)
	srv := newAuthTestServer(t, store, jwtSvc)

	pair, _ := jwtSvc.GenerateTokenPair(user)

	rr := postJSON(srv.Routes(), "/api/v1/auth/refresh", map[string]string{
		"refresh_token": pair.RefreshToken,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["access_token"] == "" {
		t.Error("access_token missing from refresh response")
	}
}

func TestHandleRefresh_WithAccessToken(t *testing.T) {
	jwtSvc := testJWTSvc()
	user := &auth.User{ID: 1, Username: "bob", Role: string(auth.RoleDBA), Active: true}
	pair, _ := jwtSvc.GenerateTokenPair(user)
	srv := newAuthTestServer(t, newMockUserStore(user), jwtSvc)

	// Passing an access token where a refresh token is expected must fail.
	rr := postJSON(srv.Routes(), "/api/v1/auth/refresh", map[string]string{
		"refresh_token": pair.AccessToken,
	})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandleRefresh_Invalid(t *testing.T) {
	jwtSvc := testJWTSvc()
	srv := newAuthTestServer(t, newMockUserStore(), jwtSvc)

	rr := postJSON(srv.Routes(), "/api/v1/auth/refresh", map[string]string{
		"refresh_token": "not.a.valid.jwt",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandleMe_ValidToken(t *testing.T) {
	jwtSvc := testJWTSvc()
	user := &auth.User{ID: 42, Username: "charlie", Role: string(auth.RoleSuperAdmin), Active: true}
	pair, _ := jwtSvc.GenerateTokenPair(user)
	srv := newAuthTestServer(t, newMockUserStore(user), jwtSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var resp userResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Username != "charlie" {
		t.Errorf("Username = %q, want %q", resp.Username, "charlie")
	}
	if resp.ID != 42 {
		t.Errorf("ID = %d, want 42", resp.ID)
	}
	if !resp.Active {
		t.Error("Active = false, want true")
	}
	if len(resp.Permissions) == 0 {
		t.Error("Permissions should not be empty for super_admin")
	}
}

// Compile-time check that mockUserStore satisfies auth.UserStore.
var _ auth.UserStore = (*mockUserStore)(nil)
