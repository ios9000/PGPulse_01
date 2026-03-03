# Session: 2026-03-03 — M5_02 Auth + RBAC UI

## Goal
Connect frontend to real backend JWT auth. Login page, dual-token management, route guards, role-based visibility, user management, 4-role expansion.

## Agent Team Configuration
- Team Lead: Opus 4.6
- Specialists: Backend Auth (Go), Frontend Auth (TypeScript/React)
- Duration: 1h 1m 1s
- QA integrated into both agents (no separate QA agent this run)

## Decisions Implemented

| ID | Decision |
|----|----------|
| D90 | 4-role backend migration; frontend uses permission groups |
| D91 | Access token 15min in memory, refresh token 7d in localStorage, strict CSP |
| D92 | User management: create + list + deactivate + change own password |
| D93 | Backend changes included in M5_02 |
| D94 | Simple login form → token → redirect |

## Backend Agent Activity

13 Go files, ~1100 lines:

- **Role migration**: migrations/005_expand_roles.sql — admin→super_admin, viewer→dba, added active + last_login columns
- **Permission system**: internal/auth/rbac.go rewritten — 4 roles, 5 permissions, HasPermission(), PermissionsForRole()
- **Dual token JWT**: internal/auth/jwt.go updated — separate access/refresh secrets, AccessTokenTTL=15min, RefreshTokenTTL=7d
- **New endpoints**: 
  - POST /api/v1/auth/refresh — validate refresh token, check user active, issue new access
  - GET /api/v1/auth/users — list users (user_management)
  - PUT /api/v1/auth/users/:id — update role/active (user_management, self-protection, super_admin promotion guard)
  - PUT /api/v1/auth/me/password — change own password (current password verification)
  - POST /api/v1/auth/register — updated to accept 4-role values
- **Security headers**: CSP, X-Content-Type-Options, X-Frame-Options, Referrer-Policy middleware
- **Router**: permission-based middleware replacing role-based checks
- **Tests**: 73/73 pass (30 auth + 43 api), all updated for 4-role model

## Frontend Agent Activity

18 TypeScript files, ~460 lines:

- **Auth store** (authStore.ts): dual-token with access in Zustand memory, refresh in localStorage, auto-refresh scheduling, visibilitychange handler
- **API client** (api.ts): Bearer token attachment, 401→refresh→retry with queue to prevent multiple simultaneous refreshes
- **Permissions** (permissions.ts): Role/Permission types, ROLE_PERMISSIONS map, hasPermission()
- **Route guards**: ProtectedRoute (auth check + loading spinner), PermissionGate (permission-based access)
- **Login page** (LoginPage.tsx): dark full-page form, real API integration, error handling, rate limit display
- **User management** (UsersPage.tsx): DataTable with create/deactivate/role-change, CreateUserModal, DeactivateUserDialog
- **Change password** (ChangePasswordModal.tsx): current + new password with validation
- **TopBar**: user dropdown with role badge, change password, logout
- **Sidebar**: permission-filtered nav items (Alert Rules → alert_management, Users → user_management)
- **App.tsx**: initialize() on mount, ProtectedRoute wrapping, PermissionGate on admin routes
- **Hooks**: useAuth, useUsers (TanStack Query CRUD)

## Verification Results

| Check | Result |
|-------|--------|
| go build ./cmd/pgpulse-server | ✅ pass |
| go vet ./... | ✅ pass |
| go test ./cmd/... ./internal/... | ✅ 73/73 pass |
| tsc --noEmit | ✅ zero errors |
| eslint | ✅ pass |
| vite build | ✅ pass |
| golangci-lint | ⚠️ 1 pre-existing issue (not from M5_02) |

## Architecture Notes

- Access and refresh tokens use DIFFERENT signing secrets — prevents token type confusion
- Permission model is identical on backend (Go) and frontend (TypeScript) — single source of truth pattern
- 401 retry queue prevents thundering herd on token expiry (only one refresh fires, others wait)
- CSP header blocks inline scripts from third-party domains — primary XSS mitigation for localStorage refresh token
- Migration is backward-compatible: existing admin users become super_admin, viewers become dba

## Not Done / Next Iteration

- [ ] Full 4-role granular UI (currently 2-tier: admin-like vs read-only) — M5_03 or later
- [ ] Token rotation on refresh (refresh token stays static for MVP) — security hardening
- [ ] Rate limiting display on login (429 handling is coded but untested against real backend rate limiter)
- [ ] ECharts chunk optimization (347KB gzipped, known from M5_01)
- [ ] Fleet Overview with real data (currently mock) — M5_03
- [ ] Server detail page with real metrics — M5_03+
