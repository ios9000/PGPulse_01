# M5_02 Requirements — Auth + RBAC UI

**Iteration:** M5_02
**Milestone:** M5 (Web UI)
**Date:** 2026-03-03

---

## Goal

Connect the frontend to the real backend JWT auth system. After M5_02:
1. Login page authenticates against POST /api/v1/auth/login and receives a dual-token pair
2. Access token (15min, in memory) used for all API calls; refresh token (7d, localStorage) silently renews it
3. Unauthenticated users are redirected to /login; authenticated users see role-appropriate navigation
4. Admin-tier users can manage users (create, list, deactivate)
5. Any user can change their own password
6. Backend recognizes 4 roles (super_admin, roles_admin, dba, app_admin) with permission-based middleware
7. Strict CSP headers on all responses

---

## Decisions (from planning)

| ID | Decision | Rationale |
|----|----------|-----------|
| D90 | 4-role backend migration now; frontend uses permission groups, not raw roles | Avoids second migration; frontend checks capabilities not role names |
| D91 | Access token 15min in Zustand (memory), refresh token 7d in localStorage | Security/UX balance; CSP blocks XSS vectors; short access TTL limits exposure |
| D92 | User management: create + list + deactivate + change own password | Covers operational needs without full identity management |
| D93 | Backend changes included in M5_02 (not split) | Small scope, tightly coupled to frontend |
| D94 | Simple login form, token, redirect to /fleet | No "remember me", no "forgot password" (internal tool) |

---

## Backend Changes

### 1. Role Migration

New migration expands the role enum and maps existing data:

- Old: `admin`, `viewer`
- New: `super_admin`, `roles_admin`, `dba`, `app_admin`
- Migration mapping: `admin` -> `super_admin`, `viewer` -> `dba`

### 2. Permission-Based RBAC

Replace role-name checks with permission groups:

| Permission | Roles Granted |
|------------|---------------|
| `user_management` | super_admin, roles_admin |
| `instance_management` | super_admin, dba |
| `alert_management` | super_admin, dba |
| `view_all` | all 4 roles |
| `self_management` | all 4 roles |

Middleware checks permissions, not role names. Endpoint-to-permission mapping:

| Endpoint | Permission Required |
|----------|-------------------|
| GET /api/v1/health | public or any authenticated |
| GET /api/v1/instances | view_all |
| POST /api/v1/instances | instance_management |
| PUT /api/v1/instances/:id | instance_management |
| DELETE /api/v1/instances/:id | instance_management |
| GET /api/v1/instances/:id/metrics | view_all |
| GET /api/v1/alerts | view_all |
| POST /api/v1/alerts/rules | alert_management |
| PUT /api/v1/alerts/rules/:id | alert_management |
| DELETE /api/v1/alerts/rules/:id | alert_management |
| POST /api/v1/alerts/test | alert_management |
| POST /api/v1/auth/login | public |
| POST /api/v1/auth/refresh | public, requires valid refresh token |
| POST /api/v1/auth/register | user_management |
| GET /api/v1/auth/users | user_management |
| PUT /api/v1/auth/users/:id | user_management |
| GET /api/v1/auth/me | self_management |
| PUT /api/v1/auth/me/password | self_management |

### 3. New Endpoints

**POST /api/v1/auth/refresh**
- Request: `{ "refresh_token": "..." }`
- Response: `{ "access_token": "...", "expires_in": 900 }`
- Validates refresh token, issues new access token
- Refresh token itself is NOT rotated (simplicity for MVP)

**GET /api/v1/auth/users**
- Response: `[{ "id", "username", "role", "active", "created_at", "last_login" }]`
- Requires: user_management permission

**PUT /api/v1/auth/users/:id**
- Request: `{ "role": "dba", "active": false }` (partial update)
- Requires: user_management permission
- Cannot deactivate own account
- Only super_admin can change someone TO super_admin

**PUT /api/v1/auth/me/password**
- Request: `{ "current_password": "...", "new_password": "..." }`
- Requires: self_management (any authenticated user)
- Must verify current password before changing

### 4. Dual Token Response

Update POST /api/v1/auth/login response:

```json
{
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "expires_in": 900,
  "user": {
    "id": "uuid",
    "username": "admin",
    "role": "super_admin"
  }
}
```

Access token claims: `{ sub, username, role, permissions: [...], exp, iat }`
Refresh token claims: `{ sub, type: "refresh", exp, iat }`

### 5. CSP Headers

Add middleware that sets on ALL responses:

```
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; frame-ancestors 'none'
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
```

Note: `'unsafe-inline'` for style-src is needed because Tailwind may inject inline styles. If Tailwind 4.1 avoids this, tighten to `'self'` only.

---

## Frontend Changes

### 1. Login Page (`/login`)

- Dark-themed full-page form (outside app shell, as designed in M5_01)
- Fields: username, password
- Submit -> POST /api/v1/auth/login
- On success: store tokens, redirect to /fleet
- On error: show inline message ("Invalid credentials" or "Account deactivated")
- On 429: show "Too many attempts, try again in X seconds"
- PGPulse logo + version string on the login page

### 2. Auth Store (Zustand)

Replace the placeholder auth store with:

```typescript
interface AuthState {
  accessToken: string | null;       // in memory only
  refreshToken: string | null;      // synced to localStorage
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  permissions: string[];            // derived from user.role

  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  refresh: () => Promise<boolean>;  // returns false if refresh failed
  initialize: () => Promise<void>;  // called on app mount, reads localStorage
}
```

On app mount (`initialize`):
1. Read refresh token from localStorage
2. If present, call POST /auth/refresh to get fresh access token
3. If refresh succeeds, populate user + accessToken + permissions
4. If refresh fails (expired/invalid), clear localStorage, redirect to /login

Permission derivation (client-side, mirrors backend):

```typescript
const ROLE_PERMISSIONS: Record<string, string[]> = {
  super_admin:  ['user_management', 'instance_management', 'alert_management', 'view_all', 'self_management'],
  roles_admin:  ['user_management', 'view_all', 'self_management'],
  dba:          ['instance_management', 'alert_management', 'view_all', 'self_management'],
  app_admin:    ['view_all', 'self_management'],
};
```

### 3. API Client Update

Update `apiFetch` to:
- Attach `Authorization: Bearer <accessToken>` on every request
- On 401 response: attempt silent refresh via POST /auth/refresh
- If refresh succeeds: retry the original request with new token
- If refresh fails: clear auth state, redirect to /login
- Queue concurrent requests during refresh (don't fire 5 refresh calls simultaneously)

### 4. Route Guards

**ProtectedRoute** wraps all authenticated routes:
- If `isLoading` (initialize in progress): show Spinner
- If `!isAuthenticated`: redirect to /login
- Otherwise: render children

**PermissionGate** for conditional rendering based on permissions:
```tsx
<PermissionGate requires="user_management">
  <AdminUsersPage />
</PermissionGate>
```
If user lacks permission: show 403 page or redirect to /fleet.

### 5. Sidebar Update

Nav items conditionally visible based on permissions:

| Nav Item | Required Permission | Roles That See It |
|----------|-------------------|-------------------|
| Fleet Overview | view_all | all |
| Alerts | view_all | all |
| Alert Rules | alert_management | super_admin, dba |
| Admin / Users | user_management | super_admin, roles_admin |

### 6. User Management Page (`/admin/users`)

- DataTable listing all users: username, role, active status, created_at, last_login
- "Create User" button -> modal with: username, password, role (dropdown of 4 roles)
- "Deactivate" button per row (with confirmation dialog)
- Role change dropdown per row (inline or via modal)
- Cannot deactivate self; badge on own row
- Only super_admin sees option to assign super_admin role

### 7. Change Password

- Accessible from top bar user menu (all roles)
- Modal: current password, new password, confirm new password
- Client-side validation: min 8 chars, passwords match
- Submit -> PUT /api/v1/auth/me/password

### 8. Token Refresh Timer

- Set a timer for `(expires_in - 60)` seconds after login/refresh
- When timer fires, silently call POST /auth/refresh
- If tab is backgrounded, refresh on next focus (document.visibilitychange)

---

## Files Created/Modified

### Backend (Go)
| File | Action | Description |
|------|--------|-------------|
| migrations/00X_expand_roles.sql | Create | Role enum expansion + data migration |
| internal/auth/rbac.go | Modify | Permission-based checks, 4-role map |
| internal/auth/jwt.go | Modify | Dual token generation (access + refresh) |
| internal/api/auth.go | Modify | Add refresh, users list, user update, password change endpoints |
| internal/api/middleware.go | Modify | Add CSP headers middleware |

### Frontend (TypeScript/React)
| File | Action | Description |
|------|--------|-------------|
| web/src/stores/authStore.ts | Rewrite | Real auth store with dual tokens |
| web/src/lib/api.ts | Modify | Token attachment, 401 refresh retry, queue |
| web/src/lib/permissions.ts | Create | Role-to-permission mapping |
| web/src/pages/LoginPage.tsx | Rewrite | Real login form connected to API |
| web/src/components/auth/ProtectedRoute.tsx | Create | Auth check wrapper |
| web/src/components/auth/PermissionGate.tsx | Create | Permission-based conditional render |
| web/src/pages/admin/UsersPage.tsx | Create | User management table + CRUD |
| web/src/components/admin/CreateUserModal.tsx | Create | User creation form |
| web/src/components/admin/DeactivateUserDialog.tsx | Create | Confirmation dialog |
| web/src/components/auth/ChangePasswordModal.tsx | Create | Password change form |
| web/src/components/layout/Sidebar.tsx | Modify | Permission-based nav visibility |
| web/src/components/layout/TopBar.tsx | Modify | User menu with logout + change password |
| web/src/hooks/useAuth.ts | Create | Convenience hook for auth state |
| web/src/hooks/useUsers.ts | Create | TanStack Query hooks for user CRUD |
| web/src/App.tsx | Modify | Wrap routes in ProtectedRoute |

---

## Acceptance Criteria

1. Login flow works end-to-end: username/password -> backend -> tokens -> redirect to /fleet
2. Page refresh preserves session: refresh token in localStorage -> silent renewal -> no re-login
3. 401 triggers refresh: expired access token -> automatic refresh -> retry -> transparent to user
4. Unauthenticated redirect: any protected route without valid session -> /login
5. User management works: admin-tier user can create user, list users, deactivate user, change role
6. Password change works: any user can change own password with current password verification
7. Nav respects roles: admin nav items hidden from non-admin roles
8. CSP headers present: all responses include Content-Security-Policy
9. Backend 4-role model: migration applied, roles recognized, permission checks enforced
10. Build clean: npm run build + lint + typecheck + go build + go test all pass
