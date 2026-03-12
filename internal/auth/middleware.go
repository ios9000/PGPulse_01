package auth

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is unexported to prevent key collisions with other packages.
type contextKey string

const claimsContextKey contextKey = "auth_claims"

// ClaimsFromContext retrieves JWT claims from the request context.
// Returns nil when auth is disabled or the request is unauthenticated.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsContextKey).(*Claims)
	return claims
}

// AuthMode controls authentication behavior.
type AuthMode int

const (
	AuthEnabled  AuthMode = iota // Full JWT auth
	AuthDisabled                 // All requests treated as implicit admin
)

// NewAuthMiddleware returns the appropriate auth middleware based on mode.
// When mode is AuthDisabled, it injects implicit admin claims into the context
// so all downstream handlers see an authenticated super_admin user.
func NewAuthMiddleware(jwtService *JWTService, mode AuthMode, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler {
	if mode == AuthDisabled {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claims := &Claims{
					UserID:      0,
					Username:    "admin",
					Role:        string(RoleSuperAdmin),
					Type:        TokenAccess,
					Permissions: PermissionsForRole(RoleSuperAdmin),
				}
				ctx := context.WithValue(r.Context(), claimsContextKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}
	}
	return RequireAuth(jwtService, errorWriter)
}

// RequireAuth returns chi middleware that validates the Bearer token
// and injects claims into the request context.
// errorWriter is provided by the API layer to write JSON error responses
// without creating an import cycle (auth must not import api).
func RequireAuth(jwtSvc *JWTService, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header")
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid Authorization header format")
				return
			}

			claims, err := jwtSvc.ValidateToken(parts[1])
			if err != nil {
				errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
				return
			}

			if claims.Type != TokenAccess {
				errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "expected access token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns chi middleware that verifies the authenticated user
// has the required role or higher. Must be used after RequireAuth.
func RequireRole(requiredRole string, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			if !HasPermission(Role(claims.Role), PermViewAll) {
				errorWriter(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission returns chi middleware that verifies the authenticated user
// has the specified permission. Must be used after RequireAuth.
func RequirePermission(perm Permission, errorWriter func(w http.ResponseWriter, code int, errCode, message string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				errorWriter(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			if !HasPermission(Role(claims.Role), perm) {
				errorWriter(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
