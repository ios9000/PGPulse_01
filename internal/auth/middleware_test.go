package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testErrorWriter records what was written for assertions.
func testErrorWriter(t *testing.T) (func(w http.ResponseWriter, code int, errCode, message string), *int, *string) {
	t.Helper()
	code := 0
	errCode := ""
	return func(w http.ResponseWriter, c int, ec, _ string) {
		code = c
		errCode = ec
		w.WriteHeader(c)
	}, &code, &errCode
}

func newAuthMiddlewareJWT() *JWTService {
	return NewJWTService("test-secret-that-is-at-least-32-chars!", "test-refresh-secret-at-least-32-chars!", time.Hour, 7*24*time.Hour)
}

func TestRequireAuth_ValidToken(t *testing.T) {
	svc := newAuthMiddlewareJWT()
	user := &User{ID: 1, Username: "bob", Role: string(RoleDBA), Active: true}
	pair, _ := svc.GenerateTokenPair(user)

	ew, code, _ := testErrorWriter(t)
	handler := RequireAuth(svc, ew)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			t.Error("claims should be set in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if *code != 0 {
		t.Errorf("errorWriter called with code %d, want not called", *code)
	}
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	svc := newAuthMiddlewareJWT()
	ew, code, _ := testErrorWriter(t)
	handler := RequireAuth(svc, ew)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if *code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", *code)
	}
}

func TestRequireAuth_MalformedHeader(t *testing.T) {
	svc := newAuthMiddlewareJWT()
	ew, code, _ := testErrorWriter(t)
	handler := RequireAuth(svc, ew)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "NotBearer token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if *code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", *code)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	svc := NewJWTService("test-secret-that-is-at-least-32-chars!", "test-refresh-secret-at-least-32-chars!", -time.Second, time.Hour)
	ew, code, _ := testErrorWriter(t)
	user := &User{ID: 1, Username: "alice", Role: string(RoleSuperAdmin), Active: true}
	pair, _ := svc.GenerateTokenPair(user)

	// Validate with a fresh service (same secret) — token already expired.
	freshSvc := newAuthMiddlewareJWT()
	handler := RequireAuth(freshSvc, ew)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if *code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", *code)
	}
}

func TestRequireAuth_RefreshTokenRejected(t *testing.T) {
	svc := newAuthMiddlewareJWT()
	user := &User{ID: 1, Username: "alice", Role: string(RoleSuperAdmin), Active: true}
	pair, _ := svc.GenerateTokenPair(user)

	ew, code, _ := testErrorWriter(t)
	handler := RequireAuth(svc, ew)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with refresh token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.RefreshToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if *code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", *code)
	}
}

func TestRequirePermission_BlocksUnauthorized(t *testing.T) {
	svc := newAuthMiddlewareJWT()
	// app_admin does NOT have user_management permission.
	user := &User{ID: 2, Username: "carol", Role: string(RoleAppAdmin), Active: true}
	pair, _ := svc.GenerateTokenPair(user)

	ew, code, _ := testErrorWriter(t)

	// Chain: RequireAuth → RequirePermission(user_management).
	inner := RequirePermission(PermUserManagement, ew)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for app_admin on user_management route")
	}))
	handler := RequireAuth(svc, ew)(inner)

	req := httptest.NewRequest(http.MethodDelete, "/admin-only", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if *code != http.StatusForbidden {
		t.Errorf("code = %d, want 403", *code)
	}
}
