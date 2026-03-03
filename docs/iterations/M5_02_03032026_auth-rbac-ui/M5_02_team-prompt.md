# M5_02 Team Prompt — Auth + RBAC UI

Build the authentication UI and role-based access control for PGPulse.
Read CLAUDE.md for project context.
Read docs/iterations/M5_02_03032026_auth-rbac-ui/design.md for full technical design.

**Environment:** Windows 10. Agents CANNOT run bash commands. Create all files; developer runs build/test manually.

Create a team of 3 specialists:

---

## SPECIALIST 1 — BACKEND AUTH (Go)

Read the existing auth code before modifying:
- internal/auth/rbac.go — current role definitions
- internal/auth/jwt.go — current token generation
- internal/auth/store.go — user repository
- internal/api/auth.go — current auth handlers
- internal/api/router.go — current route wiring
- migrations/ — existing migration files (find the latest number)

### Tasks:

**1. Role migration** — Create migrations/00X_expand_roles.sql:
- Inspect existing users table schema to determine if role is VARCHAR or ENUM
- If VARCHAR with CHECK constraint: drop old constraint, update data, add new constraint
- If ENUM type: ALTER TYPE ... ADD VALUE for new values, then update data
- Mapping: admin -> super_admin, viewer -> dba
- Add `active BOOLEAN NOT NULL DEFAULT true` column if not exists
- Add `last_login TIMESTAMPTZ` column if not exists

**2. Permission system** — Update internal/auth/rbac.go:
- Define Permission type and 5 permission constants
- Define Role type and 4 role constants (super_admin, roles_admin, dba, app_admin)
- Create RolePermissions map
- Add HasPermission(role, perm) function
- Add PermissionsForRole(role) function returning []string
- Update any existing RequireRole middleware to use RequirePermission instead
- Keep backward compatibility: if old code references "admin"/"viewer", update those references

**3. Dual token JWT** — Update internal/auth/jwt.go:
- Add AccessClaims struct with username, role, permissions fields
- Add RefreshClaims struct with token_type field
- Add refresh_secret to JWTService config (separate from access secret)
- Add GenerateTokenPair(userID, username, role) returning TokenPair{access, refresh, expires_in}
- Add ValidateRefreshToken(tokenString) returning RefreshClaims
- AccessTokenTTL = 15 minutes, RefreshTokenTTL = 7 days
- Access and refresh tokens MUST use different signing secrets

**4. New endpoints** — Update internal/api/auth.go:
- POST /api/v1/auth/refresh — validate refresh token, check user active, issue new access token
- GET /api/v1/auth/users — list all users (requires user_management)
- PUT /api/v1/auth/users/:id — update role and/or active status (requires user_management)
  - Cannot deactivate own account (compare JWT subject with :id)
  - Only super_admin can set role TO super_admin
- PUT /api/v1/auth/me/password — change own password (requires valid current_password)
- Update POST /api/v1/auth/login to return TokenPair + user object
- Update POST /api/v1/auth/register to accept role field (one of 4 roles)

**5. CSP middleware** — Update internal/api/middleware.go (or create if needed):
- Add SecurityHeaders middleware that sets CSP, X-Content-Type-Options, X-Frame-Options, Referrer-Policy
- Apply at router root level before all other middleware

**6. Router update** — Update internal/api/router.go:
- Wire new endpoints
- Replace role-based middleware with permission-based middleware
- Group routes by permission level as shown in design.md

**7. Config update** — Update config struct and example YAML:
- Add auth.refresh_secret field
- Add auth.access_ttl and auth.refresh_ttl fields with defaults

### Critical Rules:
- Do NOT break existing tests — update test fixtures to use new role names
- All new endpoints must validate input (reject missing/malformed fields)
- Error responses: JSON `{"error": "message"}` with appropriate HTTP status
- Log security events: failed logins, deactivation, role changes (slog.Warn/Info)

---

## SPECIALIST 2 — FRONTEND AUTH (TypeScript/React)

Read the existing frontend code before modifying:
- web/src/stores/authStore.ts — current placeholder store
- web/src/lib/api.ts — current apiFetch implementation
- web/src/pages/LoginPage.tsx — current mock login page
- web/src/components/layout/Sidebar.tsx — current nav items
- web/src/components/layout/TopBar.tsx — current top bar
- web/src/App.tsx — current route structure

### Tasks:

**1. Permission system** — Create web/src/lib/permissions.ts:
- Type definitions: Role, Permission
- ROLE_PERMISSIONS map
- hasPermission() and getPermissions() functions

**2. Auth store** — Rewrite web/src/stores/authStore.ts:
- Access token in memory (Zustand state), refresh token in localStorage
- login(): POST /auth/login, store both tokens, derive permissions from role, schedule refresh timer
- logout(): clear everything, remove localStorage key, redirect to /login
- refresh(): POST /auth/refresh with refresh token, update access token in memory
- initialize(): read refresh token from localStorage, attempt refresh, fetch /auth/me for user data
- scheduleRefresh(): setTimeout for (expires_in - 60) seconds
- localStorage key: 'pgpulse_refresh_token'
- Handle visibility change: refresh on tab focus if authenticated

**3. API client** — Update web/src/lib/api.ts:
- Add skipAuth option to apiFetch
- Attach Bearer token from auth store on every request (unless skipAuth)
- On 401: attempt silent refresh via auth store
- Queue mechanism: if refresh is in progress, queue other 401 retries (don't fire multiple refreshes)
- On refresh failure: logout and redirect to /login
- Default Content-Type: application/json for requests with body

**4. Route guards** — Create web/src/components/auth/ProtectedRoute.tsx:
- While isLoading: show Spinner centered on dark background
- If not authenticated: Navigate to /login
- Otherwise: render children

**5. Permission gate** — Create web/src/components/auth/PermissionGate.tsx:
- Check if user has required permission
- If not: render fallback or Navigate to /fleet
- Props: requires (Permission), children, optional fallback

**6. Login page** — Rewrite web/src/pages/LoginPage.tsx:
- Dark full-page layout, no app shell
- PGPulse logo/text centered above form
- Username + password fields
- Submit button with loading spinner state
- Error message display (inline, below form)
- Rate limit handling: show "Too many attempts" with countdown
- On success: navigate to /fleet via react-router
- Keyboard: Enter submits form

**7. App.tsx update:**
- Call authStore.initialize() on mount (useEffect)
- Add visibilitychange listener for token refresh on tab focus
- Wrap authenticated routes in ProtectedRoute
- Wrap admin routes in PermissionGate
- Login route stays outside ProtectedRoute

**8. Sidebar update** — Modify web/src/components/layout/Sidebar.tsx:
- Add permission field to nav items
- Filter items by user permissions from auth store
- Alert Rules visible only with alert_management
- Users visible only with user_management

**9. TopBar update** — Modify web/src/components/layout/TopBar.tsx:
- Show current username and role badge
- Dropdown menu: "Change Password", "Logout"
- Logout calls authStore.logout()

**10. User management page** — Create web/src/pages/admin/UsersPage.tsx:
- Use DataTable component from M5_01
- Columns: Username, Role, Status (StatusBadge), Created, Last Login, Actions
- "Create User" button opens CreateUserModal
- Actions: role dropdown, deactivate/activate toggle
- Own row: highlighted, deactivate disabled, "You" badge
- super_admin role assignment: only visible to super_admin users

**11. Create user modal** — Create web/src/components/admin/CreateUserModal.tsx:
- Fields: username, password, confirm password, role dropdown
- Validation: username required, password min 8 chars, passwords match
- Role dropdown shows all 4 roles (super_admin only if current user is super_admin)
- Submit via useCreateUser hook

**12. Deactivate dialog** — Create web/src/components/admin/DeactivateUserDialog.tsx:
- Confirmation: "Are you sure you want to deactivate {username}?"
- Submit via useUpdateUser hook with active: false

**13. Change password modal** — Create web/src/components/auth/ChangePasswordModal.tsx:
- Fields: current password, new password, confirm new password
- Validation: all required, new password min 8 chars, new passwords match
- Submit via useChangePassword hook
- On success: close modal, show success toast/message

**14. TanStack Query hooks** — Create web/src/hooks/useUsers.ts:
- useUsers(): GET /auth/users
- useCreateUser(): POST /auth/register, invalidates ['users'] on success
- useUpdateUser(): PUT /auth/users/:id, invalidates ['users'] on success
- useChangePassword(): PUT /auth/me/password

**15. Auth hook** — Create web/src/hooks/useAuth.ts:
- Convenience hook that re-exports commonly used auth store selectors
- hasPermission(perm) check

### Critical Rules:
- Use Tailwind utility classes only (dark-first: bg-gray-950, bg-gray-900, etc.)
- Use Lucide React for all icons
- Use existing UI components: StatusBadge, DataTable, Spinner, PageHeader
- No localStorage for access token — memory only (Zustand state)
- Refresh token in localStorage under key 'pgpulse_refresh_token'
- All API calls go through apiFetch (never raw fetch)
- TypeScript strict mode — no `any` types

---

## SPECIALIST 3 — QA & TESTS

### Backend Tests (Go):

**1. Permission tests** — Create/update internal/auth/rbac_test.go:
- TestRolePermissions: verify all 4 roles have expected permissions
- TestHasPermission: each role checked against all 5 permissions
- TestInvalidRole: unknown role returns false for all permissions
- TestPermissionsForRole: returns correct string slice

**2. JWT tests** — Create/update internal/auth/jwt_test.go:
- TestGenerateTokenPair: both tokens generated, different from each other
- TestAccessTokenClaims: contains username, role, permissions
- TestRefreshTokenClaims: contains type="refresh", subject
- TestAccessTokenExpiry: verify 15min expiry
- TestRefreshTokenExpiry: verify 7d expiry
- TestDifferentSigningSecrets: access token fails validation with refresh secret and vice versa
- TestValidateRefreshToken: valid token returns claims
- TestValidateRefreshToken_Expired: expired token returns error
- TestAccessTokenAsRefresh: access token rejected by ValidateRefreshToken

**3. Auth endpoint tests** — Create/update internal/api/auth_test.go:
- TestLoginReturnsTokenPair: POST /auth/login returns access_token, refresh_token, expires_in, user
- TestRefreshEndpoint: valid refresh token returns new access token
- TestRefreshWithExpiredToken: returns 401
- TestRefreshWithDeactivatedUser: returns 401
- TestRefreshWithAccessToken: returns 401 (wrong token type)
- TestListUsers_WithUserManagement: super_admin and roles_admin get 200
- TestListUsers_WithoutPermission: dba and app_admin get 403
- TestUpdateUser_ChangeRole: works for user_management holders
- TestUpdateUser_DeactivateSelf: returns 400/403
- TestUpdateUser_SuperAdminOnlyPromotion: only super_admin can set role to super_admin
- TestChangePassword_Success: correct current password, new password set
- TestChangePassword_WrongCurrent: returns 400/401
- TestCSPHeaders: check all responses include CSP header

**4. Permission middleware tests** — Create/update internal/api/middleware_test.go:
- TestRequirePermission_Allowed: role with permission gets 200
- TestRequirePermission_Forbidden: role without permission gets 403
- TestRequirePermission_Unauthenticated: no token gets 401

**5. Migration test** (if testcontainers available):
- Verify admin -> super_admin mapping
- Verify viewer -> dba mapping
- Verify new columns (active, last_login) exist

**6. Regression check:**
- Run existing test suite — update any fixtures referencing old "admin"/"viewer" roles
- Verify all previously passing tests still pass with 4-role model
- Scan for string literals "admin" and "viewer" in non-test code that should be updated

### Frontend Verification (manual checklist for developer):
- [ ] Login page renders on /login
- [ ] Invalid credentials show error message
- [ ] Successful login redirects to /fleet
- [ ] Page refresh preserves session (no re-login)
- [ ] Closing and reopening tab preserves session
- [ ] After ~14 min, token refreshes silently (check Network tab)
- [ ] Non-admin user cannot see Users nav item
- [ ] Non-admin user navigating to /admin/users redirected to /fleet
- [ ] Admin can create a new user
- [ ] Admin can deactivate a user
- [ ] Admin cannot deactivate themselves
- [ ] Any user can change their own password
- [ ] Logout clears session, redirects to /login

### Critical Rules:
- All backend tests MUST be table-driven where applicable
- Use t.Parallel() for independent tests
- Test fixtures must use the new 4-role model
- Do NOT skip updating existing tests that reference "admin"/"viewer"

---

## Coordination Notes

- Specialist 1 (Backend) and Specialist 2 (Frontend) can work in parallel
- Specialist 2 depends on knowing the API contract (response shapes) but not the implementation
- Specialist 3 starts writing test stubs immediately, fills assertions when code lands
- All agents: list files created/modified at the end

## Build Verification (Developer runs manually)

```
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

If errors, paste them back and agents will fix.
