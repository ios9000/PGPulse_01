# M5_02 Technical Design — Auth + RBAC UI

**Iteration:** M5_02
**Milestone:** M5 (Web UI)
**Date:** 2026-03-03

---

## 1. Backend: Role Migration

### Migration SQL (migrations/00X_expand_roles.sql)

```sql
-- Expand role enum from (admin, viewer) to 4-role model
-- Migration mapping: admin -> super_admin, viewer -> dba

BEGIN;

-- Step 1: Add new role values to the enum
-- Approach depends on how roles are stored. If it's a VARCHAR column:
ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_role_check;

-- Step 2: Migrate existing data
UPDATE users SET role = 'super_admin' WHERE role = 'admin';
UPDATE users SET role = 'dba' WHERE role = 'viewer';

-- Step 3: Add new constraint with all 4 roles
ALTER TABLE users
  ADD CONSTRAINT users_role_check
  CHECK (role IN ('super_admin', 'roles_admin', 'dba', 'app_admin'));

-- Step 4: Add fields for user management
ALTER TABLE users ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login TIMESTAMPTZ;

COMMIT;
```

Note: The exact migration depends on how M3 defined the users table. If the role is a PostgreSQL ENUM type, the migration needs `ALTER TYPE ... ADD VALUE` instead. The agent must inspect the existing schema and adapt.

---

## 2. Backend: Permission System

### internal/auth/rbac.go (updated)

```go
// Permission represents a named capability
type Permission string

const (
    PermUserManagement     Permission = "user_management"
    PermInstanceManagement Permission = "instance_management"
    PermAlertManagement    Permission = "alert_management"
    PermViewAll            Permission = "view_all"
    PermSelfManagement     Permission = "self_management"
)

// Role represents a user role in the system
type Role string

const (
    RoleSuperAdmin Role = "super_admin"
    RoleRolesAdmin Role = "roles_admin"
    RoleDBA        Role = "dba"
    RoleAppAdmin   Role = "app_admin"
)

// ValidRoles is the set of all valid roles
var ValidRoles = []Role{RoleSuperAdmin, RoleRolesAdmin, RoleDBA, RoleAppAdmin}

// RolePermissions maps each role to its granted permissions
var RolePermissions = map[Role][]Permission{
    RoleSuperAdmin: {PermUserManagement, PermInstanceManagement, PermAlertManagement, PermViewAll, PermSelfManagement},
    RoleRolesAdmin: {PermUserManagement, PermViewAll, PermSelfManagement},
    RoleDBA:        {PermInstanceManagement, PermAlertManagement, PermViewAll, PermSelfManagement},
    RoleAppAdmin:   {PermViewAll, PermSelfManagement},
}

// HasPermission checks if a role has a specific permission
func HasPermission(role Role, perm Permission) bool {
    perms, ok := RolePermissions[role]
    if !ok {
        return false
    }
    for _, p := range perms {
        if p == perm {
            return true
        }
    }
    return false
}

// PermissionsForRole returns the permission list for a role (used in JWT claims)
func PermissionsForRole(role Role) []string {
    perms := RolePermissions[role]
    result := make([]string, len(perms))
    for i, p := range perms {
        result[i] = string(p)
    }
    return result
}
```

### Middleware update

Replace the existing role-checking middleware with permission-based:

```go
// RequirePermission returns middleware that checks for a specific permission
func RequirePermission(perm Permission) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims, ok := ClaimsFromContext(r.Context())
            if !ok {
                http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
                return
            }
            if !HasPermission(Role(claims.Role), perm) {
                http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Router wiring example:

```go
r.Route("/api/v1", func(r chi.Router) {
    // Public
    r.Post("/auth/login", h.Login)
    r.Post("/auth/refresh", h.RefreshToken)

    // Authenticated (any role)
    r.Group(func(r chi.Router) {
        r.Use(auth.RequireAuth)

        // view_all
        r.Get("/instances", h.ListInstances)
        r.Get("/instances/{id}", h.GetInstance)
        r.Get("/instances/{id}/metrics", h.GetInstanceMetrics)
        r.Get("/alerts", h.ListAlerts)
        r.Get("/auth/me", h.GetCurrentUser)

        // self_management
        r.Put("/auth/me/password", h.ChangeOwnPassword)

        // instance_management
        r.With(auth.RequirePermission(auth.PermInstanceManagement)).Post("/instances", h.CreateInstance)
        r.With(auth.RequirePermission(auth.PermInstanceManagement)).Put("/instances/{id}", h.UpdateInstance)
        r.With(auth.RequirePermission(auth.PermInstanceManagement)).Delete("/instances/{id}", h.DeleteInstance)

        // alert_management
        r.With(auth.RequirePermission(auth.PermAlertManagement)).Post("/alerts/rules", h.CreateAlertRule)
        r.With(auth.RequirePermission(auth.PermAlertManagement)).Put("/alerts/rules/{id}", h.UpdateAlertRule)
        r.With(auth.RequirePermission(auth.PermAlertManagement)).Delete("/alerts/rules/{id}", h.DeleteAlertRule)
        r.With(auth.RequirePermission(auth.PermAlertManagement)).Post("/alerts/test", h.TestAlert)

        // user_management
        r.With(auth.RequirePermission(auth.PermUserManagement)).Post("/auth/register", h.RegisterUser)
        r.With(auth.RequirePermission(auth.PermUserManagement)).Get("/auth/users", h.ListUsers)
        r.With(auth.RequirePermission(auth.PermUserManagement)).Put("/auth/users/{id}", h.UpdateUser)
    })
})
```

---

## 3. Backend: Dual Token JWT

### Token generation (internal/auth/jwt.go updated)

```go
type TokenPair struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int    `json:"expires_in"` // seconds
}

type AccessClaims struct {
    jwt.RegisteredClaims
    Username    string   `json:"username"`
    Role        string   `json:"role"`
    Permissions []string `json:"permissions"`
}

type RefreshClaims struct {
    jwt.RegisteredClaims
    TokenType string `json:"type"` // always "refresh"
}

const (
    AccessTokenTTL  = 15 * time.Minute
    RefreshTokenTTL = 7 * 24 * time.Hour
)

func (s *JWTService) GenerateTokenPair(userID, username string, role Role) (TokenPair, error) {
    now := time.Now()

    // Access token
    accessClaims := AccessClaims{
        RegisteredClaims: jwt.RegisteredClaims{
            Subject:   userID,
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
        },
        Username:    username,
        Role:        string(role),
        Permissions: PermissionsForRole(role),
    }
    accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(s.secret)
    if err != nil {
        return TokenPair{}, fmt.Errorf("sign access token: %w", err)
    }

    // Refresh token
    refreshClaims := RefreshClaims{
        RegisteredClaims: jwt.RegisteredClaims{
            Subject:   userID,
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTokenTTL)),
        },
        TokenType: "refresh",
    }
    refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(s.refreshSecret)
    if err != nil {
        return TokenPair{}, fmt.Errorf("sign refresh token: %w", err)
    }

    return TokenPair{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        ExpiresIn:    int(AccessTokenTTL.Seconds()),
    }, nil
}
```

Key detail: access and refresh tokens use DIFFERENT signing secrets. This prevents a refresh token from being used as an access token. Both secrets come from config:

```yaml
auth:
  access_secret: "${PGPULSE_ACCESS_SECRET}"
  refresh_secret: "${PGPULSE_REFRESH_SECRET}"
  access_ttl: 15m
  refresh_ttl: 168h  # 7 days
```

### Refresh endpoint handler

```go
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
    var req struct {
        RefreshToken string `json:"refresh_token"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // Validate refresh token with refresh secret
    claims, err := h.jwtService.ValidateRefreshToken(req.RefreshToken)
    if err != nil {
        respondError(w, http.StatusUnauthorized, "invalid or expired refresh token")
        return
    }

    // Look up user (they might have been deactivated since token was issued)
    user, err := h.userStore.GetByID(r.Context(), claims.Subject)
    if err != nil || !user.Active {
        respondError(w, http.StatusUnauthorized, "user not found or deactivated")
        return
    }

    // Issue new access token only (refresh token stays the same)
    accessToken, err := h.jwtService.GenerateAccessToken(user.ID, user.Username, Role(user.Role))
    if err != nil {
        respondError(w, http.StatusInternalServerError, "token generation failed")
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "access_token": accessToken,
        "expires_in":   int(AccessTokenTTL.Seconds()),
    })
}
```

---

## 4. Backend: CSP Middleware

```go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Security-Policy",
            "default-src 'self'; "+
            "script-src 'self'; "+
            "style-src 'self' 'unsafe-inline'; "+
            "img-src 'self' data:; "+
            "connect-src 'self'; "+
            "font-src 'self'; "+
            "object-src 'none'; "+
            "frame-ancestors 'none'")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        next.ServeHTTP(w, r)
    })
}
```

Applied at the router root level, before all other middleware.

---

## 5. Frontend: Auth Store

### web/src/lib/permissions.ts

```typescript
export type Role = 'super_admin' | 'roles_admin' | 'dba' | 'app_admin';
export type Permission =
  | 'user_management'
  | 'instance_management'
  | 'alert_management'
  | 'view_all'
  | 'self_management';

export const ROLE_PERMISSIONS: Record<Role, Permission[]> = {
  super_admin:  ['user_management', 'instance_management', 'alert_management', 'view_all', 'self_management'],
  roles_admin:  ['user_management', 'view_all', 'self_management'],
  dba:          ['instance_management', 'alert_management', 'view_all', 'self_management'],
  app_admin:    ['view_all', 'self_management'],
};

export function hasPermission(role: Role, permission: Permission): boolean {
  return ROLE_PERMISSIONS[role]?.includes(permission) ?? false;
}

export function getPermissions(role: Role): Permission[] {
  return ROLE_PERMISSIONS[role] ?? [];
}
```

### web/src/stores/authStore.ts

```typescript
import { create } from 'zustand';
import { apiFetch } from '../lib/api';
import { type Role, type Permission, getPermissions } from '../lib/permissions';

const REFRESH_TOKEN_KEY = 'pgpulse_refresh_token';

interface User {
  id: string;
  username: string;
  role: Role;
}

interface AuthState {
  accessToken: string | null;
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  permissions: Permission[];
  refreshTimerId: number | null;

  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  refresh: () => Promise<boolean>;
  initialize: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  accessToken: null,
  user: null,
  isAuthenticated: false,
  isLoading: true,
  permissions: [],
  refreshTimerId: null,

  login: async (username: string, password: string) => {
    const res = await apiFetch('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
      skipAuth: true, // don't attach Bearer on login
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({ error: 'Login failed' }));
      throw new Error(data.error || `HTTP ${res.status}`);
    }
    const data = await res.json();
    const user: User = data.user;
    localStorage.setItem(REFRESH_TOKEN_KEY, data.refresh_token);

    set({
      accessToken: data.access_token,
      user,
      isAuthenticated: true,
      isLoading: false,
      permissions: getPermissions(user.role),
    });

    get().scheduleRefresh(data.expires_in);
  },

  logout: () => {
    const { refreshTimerId } = get();
    if (refreshTimerId) clearTimeout(refreshTimerId);
    localStorage.removeItem(REFRESH_TOKEN_KEY);
    set({
      accessToken: null,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      permissions: [],
      refreshTimerId: null,
    });
  },

  refresh: async () => {
    const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
    if (!refreshToken) return false;

    try {
      const res = await apiFetch('/api/v1/auth/refresh', {
        method: 'POST',
        body: JSON.stringify({ refresh_token: refreshToken }),
        skipAuth: true,
      });
      if (!res.ok) return false;

      const data = await res.json();
      set({ accessToken: data.access_token });
      get().scheduleRefresh(data.expires_in);
      return true;
    } catch {
      return false;
    }
  },

  initialize: async () => {
    set({ isLoading: true });
    const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
    if (!refreshToken) {
      set({ isLoading: false });
      return;
    }

    try {
      const res = await apiFetch('/api/v1/auth/refresh', {
        method: 'POST',
        body: JSON.stringify({ refresh_token: refreshToken }),
        skipAuth: true,
      });
      if (!res.ok) throw new Error('refresh failed');

      const data = await res.json();
      // Decode user from access token (or add user to refresh response)
      // For simplicity, fetch /auth/me with the new token
      const meRes = await apiFetch('/api/v1/auth/me', {
        headers: { Authorization: `Bearer ${data.access_token}` },
      });
      if (!meRes.ok) throw new Error('me failed');
      const user = await meRes.json();

      set({
        accessToken: data.access_token,
        user,
        isAuthenticated: true,
        isLoading: false,
        permissions: getPermissions(user.role),
      });
      get().scheduleRefresh(data.expires_in);
    } catch {
      localStorage.removeItem(REFRESH_TOKEN_KEY);
      set({ isLoading: false, isAuthenticated: false });
    }
  },

  // Internal helper — not exposed in the interface
  scheduleRefresh: (expiresIn: number) => {
    const { refreshTimerId } = get();
    if (refreshTimerId) clearTimeout(refreshTimerId);
    // Refresh 60 seconds before expiry
    const delay = Math.max((expiresIn - 60) * 1000, 10_000);
    const id = window.setTimeout(() => get().refresh(), delay);
    set({ refreshTimerId: id });
  },
}));
```

Note: `scheduleRefresh` is an internal method added to the store. The agent should handle the TypeScript typing properly (it's not in the AuthState interface but is available via `get()`).

### Visibility change handler (in App.tsx or a dedicated hook)

```typescript
useEffect(() => {
  const handleVisibility = () => {
    if (document.visibilityState === 'visible' && useAuthStore.getState().isAuthenticated) {
      useAuthStore.getState().refresh();
    }
  };
  document.addEventListener('visibilitychange', handleVisibility);
  return () => document.removeEventListener('visibilitychange', handleVisibility);
}, []);
```

---

## 6. Frontend: API Client Update

### web/src/lib/api.ts (updated)

```typescript
import { useAuthStore } from '../stores/authStore';

interface FetchOptions extends RequestInit {
  skipAuth?: boolean;
}

let isRefreshing = false;
let refreshQueue: Array<(token: string) => void> = [];

export async function apiFetch(url: string, options: FetchOptions = {}): Promise<Response> {
  const { skipAuth, ...fetchOptions } = options;

  // Attach auth header
  if (!skipAuth) {
    const token = useAuthStore.getState().accessToken;
    if (token) {
      fetchOptions.headers = {
        ...fetchOptions.headers,
        Authorization: `Bearer ${token}`,
      };
    }
  }

  // Default content type
  if (!fetchOptions.headers?.['Content-Type'] && fetchOptions.body) {
    fetchOptions.headers = {
      ...fetchOptions.headers,
      'Content-Type': 'application/json',
    };
  }

  const response = await fetch(url, fetchOptions);

  // Handle 401: try refresh
  if (response.status === 401 && !skipAuth) {
    const newToken = await handleTokenRefresh();
    if (newToken) {
      // Retry with new token
      fetchOptions.headers = {
        ...fetchOptions.headers,
        Authorization: `Bearer ${newToken}`,
      };
      return fetch(url, fetchOptions);
    } else {
      // Refresh failed, logout
      useAuthStore.getState().logout();
      window.location.href = '/login';
    }
  }

  return response;
}

async function handleTokenRefresh(): Promise<string | null> {
  if (isRefreshing) {
    // Queue this request until refresh completes
    return new Promise<string>((resolve) => {
      refreshQueue.push(resolve);
    });
  }

  isRefreshing = true;
  const success = await useAuthStore.getState().refresh();
  isRefreshing = false;

  if (success) {
    const newToken = useAuthStore.getState().accessToken!;
    // Resolve all queued requests
    refreshQueue.forEach((resolve) => resolve(newToken));
    refreshQueue = [];
    return newToken;
  }

  refreshQueue = [];
  return null;
}
```

---

## 7. Frontend: Route Guards

### web/src/components/auth/ProtectedRoute.tsx

```tsx
import { Navigate } from 'react-router-dom';
import { useAuthStore } from '../../stores/authStore';
import { Spinner } from '../ui/Spinner';

export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuthStore();

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-screen bg-gray-950">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}
```

### web/src/components/auth/PermissionGate.tsx

```tsx
import { Navigate } from 'react-router-dom';
import { useAuthStore } from '../../stores/authStore';
import type { Permission } from '../../lib/permissions';

interface PermissionGateProps {
  requires: Permission;
  children: React.ReactNode;
  fallback?: React.ReactNode; // optional: show instead of redirect
}

export function PermissionGate({ requires, children, fallback }: PermissionGateProps) {
  const permissions = useAuthStore((s) => s.permissions);

  if (!permissions.includes(requires)) {
    if (fallback) return <>{fallback}</>;
    return <Navigate to="/fleet" replace />;
  }

  return <>{children}</>;
}
```

### App.tsx route structure

```tsx
<Routes>
  <Route path="/login" element={<LoginPage />} />
  <Route element={<ProtectedRoute><AppShell /></ProtectedRoute>}>
    <Route index element={<Navigate to="/fleet" replace />} />
    <Route path="/fleet" element={<FleetOverview />} />
    <Route path="/servers/:id" element={<ServerDetail />} />
    <Route path="/databases/:id" element={<DatabaseDetail />} />
    <Route path="/alerts" element={<AlertsPage />} />
    <Route path="/alerts/rules" element={
      <PermissionGate requires="alert_management"><AlertRulesPage /></PermissionGate>
    } />
    <Route path="/admin/users" element={
      <PermissionGate requires="user_management"><UsersPage /></PermissionGate>
    } />
    <Route path="*" element={<NotFound />} />
  </Route>
</Routes>
```

---

## 8. Frontend: Login Page

### web/src/pages/LoginPage.tsx

Dark full-page layout. No app shell. Key elements:
- PGPulse logo (SVG or text) centered
- Card with username/password fields
- Submit button with loading state
- Error message area (below form, red text on dark bg)
- Version string in footer

Form state managed with React useState (no need for form library).
On submit: call `authStore.login(username, password)`. On success: `navigate('/fleet')`.
Error handling: catch the thrown error from login, display message.

Rate limit response (429): parse `Retry-After` header if present, show countdown.

---

## 9. Frontend: User Management

### web/src/hooks/useUsers.ts

TanStack Query hooks:

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../lib/api';

export function useUsers() {
  return useQuery({
    queryKey: ['users'],
    queryFn: async () => {
      const res = await apiFetch('/api/v1/auth/users');
      if (!res.ok) throw new Error('Failed to fetch users');
      return res.json();
    },
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (data: { username: string; password: string; role: string }) => {
      const res = await apiFetch('/api/v1/auth/register', {
        method: 'POST',
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed' }));
        throw new Error(err.error);
      }
      return res.json();
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, ...data }: { id: string; role?: string; active?: boolean }) => {
      const res = await apiFetch(`/api/v1/auth/users/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed' }));
        throw new Error(err.error);
      }
      return res.json();
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useChangePassword() {
  return useMutation({
    mutationFn: async (data: { current_password: string; new_password: string }) => {
      const res = await apiFetch('/api/v1/auth/me/password', {
        method: 'PUT',
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Failed' }));
        throw new Error(err.error);
      }
    },
  });
}
```

### UsersPage layout

DataTable with columns: Username, Role, Status (Active/Inactive badge), Created, Last Login, Actions.

Actions column:
- Role dropdown (only roles the current user can assign — super_admin restricted to super_admin users)
- Deactivate/Activate toggle button
- Own row highlighted with "You" badge, deactivate disabled

"Create User" button top-right opens CreateUserModal.

---

## 10. Sidebar Permission Integration

Update Sidebar.tsx nav items array to include a `permission` field:

```typescript
const navItems = [
  { label: 'Fleet Overview', path: '/fleet', icon: Server },
  { label: 'Alerts', path: '/alerts', icon: AlertTriangle },
  { label: 'Alert Rules', path: '/alerts/rules', icon: Settings, permission: 'alert_management' as Permission },
  { label: 'Users', path: '/admin/users', icon: Users, permission: 'user_management' as Permission },
];

// Filter based on permissions
const visibleItems = navItems.filter(
  (item) => !item.permission || permissions.includes(item.permission)
);
```

---

## 11. Sequence Diagrams

### Login Flow

```
User          LoginPage       authStore       Backend
 |  enter creds  |               |              |
 |--------------->|               |              |
 |                | login()       |              |
 |                |-------------->|              |
 |                |               | POST /login  |
 |                |               |------------->|
 |                |               |  {access,    |
 |                |               |   refresh,   |
 |                |               |   user}      |
 |                |               |<-------------|
 |                |               | store refresh|
 |                |               | in localStorage
 |                |               | store access |
 |                |               | in memory    |
 |                |               | schedule     |
 |                |               | refresh timer|
 |                | navigate      |              |
 |                | /fleet        |              |
 |<---------------|               |              |
```

### Token Refresh Flow (automatic)

```
Timer fires     authStore       Backend
     |            |              |
     |  refresh() |              |
     |----------->|              |
     |            | POST /refresh|
     |            |   {refresh_  |
     |            |    token}    |
     |            |------------->|
     |            |  {access,    |
     |            |   expires_in}|
     |            |<-------------|
     |            | update token |
     |            | in memory    |
     |            | reschedule   |
     |            | timer        |
```

### 401 Retry Flow

```
Component       apiFetch        authStore       Backend
  |  GET /data     |               |              |
  |--------------->|               |              |
  |                | Bearer token  |              |
  |                |------------------------------>|
  |                |               |     401      |
  |                |<------------------------------|
  |                | refresh()     |              |
  |                |-------------->|              |
  |                |               | POST /refresh|
  |                |               |------------->|
  |                |               |  new token   |
  |                |               |<-------------|
  |                | retry with    |              |
  |                | new token     |              |
  |                |------------------------------>|
  |                |               |     200      |
  |    data        |<------------------------------|
  |<---------------|               |              |
```

---

## 12. Testing Strategy

### Backend Tests (Go)

| Test | File | What It Validates |
|------|------|-------------------|
| TestPermissionMapping | auth/rbac_test.go | All 4 roles have correct permissions |
| TestHasPermission | auth/rbac_test.go | Permission checks for each role |
| TestDualTokenGeneration | auth/jwt_test.go | Access + refresh tokens generated correctly |
| TestRefreshTokenValidation | auth/jwt_test.go | Refresh token validates with correct secret |
| TestAccessTokenCantRefresh | auth/jwt_test.go | Access token rejected on refresh endpoint |
| TestRefreshEndpoint | api/auth_test.go | Returns new access token, validates refresh |
| TestRefreshWithDeactivatedUser | api/auth_test.go | Returns 401 if user deactivated |
| TestListUsers | api/auth_test.go | Returns users for admin-tier, 403 for others |
| TestUpdateUser | api/auth_test.go | Role change, deactivation, self-protection |
| TestChangePassword | api/auth_test.go | Validates current password, updates |
| TestCSPHeaders | api/middleware_test.go | All responses include CSP |
| TestPermissionMiddleware | api/middleware_test.go | Each endpoint checks correct permission |
| TestRoleMigration | (manual) | Existing admin->super_admin, viewer->dba |

### Frontend Tests (if time permits — not blocking)

- Login form renders, submits, shows errors
- ProtectedRoute redirects unauthenticated users
- PermissionGate hides content from unauthorized roles
- Auth store initializes from localStorage refresh token

---

## 13. Risk Mitigations

| Risk | Mitigation |
|------|-----------|
| Existing M3 auth tests break with 4-role change | Migration maps old roles; update test fixtures to use new role names |
| Refresh token stolen via XSS | CSP blocks inline scripts and third-party domains; short access TTL limits damage window |
| Race condition: multiple 401s trigger multiple refreshes | Queue mechanism in apiFetch — only one refresh at a time, others wait |
| User deactivated while session active | Refresh endpoint checks user.active before issuing new token |
| Tailwind 'unsafe-inline' weakens CSP | Test if Tailwind 4.1 requires it; tighten in M5_03 if not |
