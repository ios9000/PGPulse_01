# M5_07 — User Management UI: Team Prompt

**Iteration:** M5_07_03082026_user-management-ui
**Date:** 2026-03-08
**Paste this into Claude Code after updating CLAUDE.md "Current Iteration" section.**

---

## Context

We are building the Users tab on the Administration page for PGPulse. This completes
the MVP admin experience — the Instances tab is already done. Read the design doc at
`docs/iterations/M5_07_03082026_user-management-ui/design.md` for all interfaces,
component specs, and code snippets before writing any code.

## Critical: What NOT to Touch

The following are complete and tested — do not modify them:
- `internal/auth/jwt.go`, `password.go`, `ratelimit.go`, `rbac.go`, `middleware.go`
- Existing handlers in `internal/api/auth.go`: handleLogin, handleRefresh, handleMe,
  handleListUsers, handleRegister, handleUpdateUser, handleChangePassword
- All existing `*_test.go` files in `internal/auth/`
- All existing frontend components outside `web/src/components/admin/` and
  `web/src/hooks/useUserManagement.ts`

---

## Create a team of 2 specialists:

---

### SPECIALIST 1 — BACKEND (API & Auth)

**Your scope:** `internal/auth/store.go`, `internal/api/auth.go`, `internal/api/router.go`

**Task 1: Add Delete to UserStore**

In `internal/auth/store.go`, add `Delete(ctx context.Context, id int64) error` to:
1. The `UserStore` interface (after `UpdateLastLogin`)
2. The `PGUserStore` struct as a new method:
   ```go
   func (s *PGUserStore) Delete(ctx context.Context, id int64) error {
       result, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
       if err != nil {
           return err
       }
       if result.RowsAffected() == 0 {
           return ErrUserNotFound
       }
       return nil
   }
   ```

**Task 2: Add handleDeleteUser to internal/api/auth.go**

Append this handler. Guard conditions in order:
1. Parse `id` from URL param — 400 on parse failure
2. Prevent self-deletion: compare `claims.UserID == id` — 400 "cannot delete your own account"
3. Fetch target user — 404 if ErrUserNotFound
4. If target.Role == "super_admin": list all users, count active super_admins. If count <= 1 → 400 "cannot delete the last active super_admin"
5. Call `s.userStore.Delete()` — 500 on error
6. Return 200 `{"status": "deleted"}`

Requires `PermUserManagement`. The route registration (Task 4) applies the middleware.

**Task 3: Add handleAdminResetPassword to internal/api/auth.go**

Add request type `adminResetPasswordRequest { NewPassword string \`json:"new_password"\` }`.

Handler logic:
1. Parse `id` from URL param
2. Decode body — require `new_password`
3. Validate len(req.NewPassword) >= 8 — 400 "password must be at least 8 characters"
4. Verify user exists via `s.userStore.GetByID()` — 404 on ErrUserNotFound
5. Hash with `auth.HashPassword(req.NewPassword, s.authCfg.BcryptCost)`
6. Call `s.userStore.UpdatePassword()` — 500 on error
7. Return 200 `{"status": "password reset"}`

Requires `PermUserManagement`.

**Task 4: Register routes in internal/api/router.go**

Find the user management route group. Confirm that GET/POST/PUT routes exist. Add the two new routes:
```
DELETE /api/v1/users/{id}           → s.handleDeleteUser
PUT    /api/v1/users/{id}/password  → s.handleAdminResetPassword
```
Both must be inside the group gated by `RequirePermission(auth.PermUserManagement, writeError)`.

If the full user management group doesn't exist yet, create it:
```go
r.Group(func(r chi.Router) {
    r.Use(auth.RequirePermission(auth.PermUserManagement, writeError))
    r.Get("/users", s.handleListUsers)
    r.Post("/users", s.handleRegister)
    r.Put("/users/{id}", s.handleUpdateUser)
    r.Delete("/users/{id}", s.handleDeleteUser)
    r.Put("/users/{id}/password", s.handleAdminResetPassword)
})
```

**Task 5: Write backend unit tests**

Create `internal/api/auth_users_test.go` (new file, not modifying existing test files).
Use httptest and a mock UserStore. Test:
- `TestHandleDeleteUser_Self` → 400
- `TestHandleDeleteUser_LastActiveSuperAdmin` → 400
- `TestHandleDeleteUser_NotFound` → 404
- `TestHandleDeleteUser_Success` → 200
- `TestHandleAdminResetPassword_TooShort` → 400
- `TestHandleAdminResetPassword_NotFound` → 404
- `TestHandleAdminResetPassword_Success` → 200

Create a `MockUserStore` in `internal/api/auth_users_test.go` that implements
`auth.UserStore`. Implement only the methods called by the two new handlers; other
methods can panic("not implemented").

---

### SPECIALIST 2 — FRONTEND

**Your scope:** `web/src/` (components/admin/, hooks/, types/, pages/)

**Task 1: Add UserRecord type to web/src/types/models.ts**

Add (or confirm if already present):
```typescript
export interface UserRecord {
  id: number;
  username: string;
  role: 'super_admin' | 'roles_admin' | 'dba' | 'app_admin';
  active: boolean;
  permissions: string[];
  last_login?: string | null;
}

export type UserRole = UserRecord['role'];
```

**Task 2: Add delete method to web/src/lib/api.ts**

The existing apiClient likely has get/post/put. Add:
```typescript
delete: async <T = void>(path: string): Promise<T> => {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!res.ok) throw await parseError(res);
  if (res.status === 204 || res.headers.get('content-length') === '0') return undefined as T;
  return res.json();
}
```
Adjust to match the actual pattern in api.ts.

**Task 3: Create web/src/hooks/useUserManagement.ts**

Five hooks using TanStack Query:
- `useUsers()` → GET /users → returns `UserRecord[]`
- `useCreateUser()` → POST /users, invalidates ['users'] on success
- `useUpdateUser()` → PUT /users/:id, invalidates ['users'] on success
- `useDeleteUser()` → DELETE /users/:id, invalidates ['users'] on success
- `useResetUserPassword()` → PUT /users/:id/password (no cache invalidation needed)

**Task 4: Create web/src/components/admin/UserFormModal.tsx**

Props: `mode: 'create' | 'edit'`, `user?: UserRecord`, `onClose`, `onSuccess`, `currentUserRole: UserRole`

Create mode: Username (text, required), Password (password, min 8), Role (select)
Edit mode: Username (read-only text display), Role (select), Active (checkbox/toggle)

Role options filtered by currentUserRole:
- super_admin → ['super_admin', 'roles_admin', 'dba', 'app_admin']
- roles_admin → ['dba', 'app_admin']

Display role labels: super_admin → "Super Admin", roles_admin → "Roles Admin", dba → "DBA", app_admin → "App Admin"

On submit: call useCreateUser or useUpdateUser. On success: call onSuccess(). On API error: show inline error.

Match the modal style used in InstanceForm.tsx (same overlay, header, footer with Cancel/Save buttons).

**Task 5: Create web/src/components/admin/DeleteUserModal.tsx**

Props: `user: UserRecord`, `onClose`, `onSuccess`

Show: "Are you sure you want to delete **{username}**? This action cannot be undone."
If user.role === 'super_admin': add warning paragraph "Warning: This user is a super administrator."
Footer: Cancel button + Delete button (red/danger style).
On submit: call useDeleteUser(user.id). On success: call onSuccess().
Handle API error inline (e.g., "cannot delete the last active super_admin").

**Task 6: Create web/src/components/admin/ResetPasswordModal.tsx**

Props: `user: UserRecord`, `onClose`

Fields: New Password (password input), Confirm Password (password input)
Client-side validation: both required, min 8 chars, must match.
On submit: call useResetUserPassword({ id: user.id, newPassword }).
On success: show brief success state then call onClose().
Title: "Reset Password — {username}"

**Task 7: Create web/src/components/admin/UsersTab.tsx**

Structure mirrors InstancesTab.tsx pattern.

Table columns: Username | Role | Status | Last Login | Created | Actions

Role badge colors (Tailwind bg+text):
- super_admin: `bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400`
- roles_admin: `bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400`
- dba: `bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400`
- app_admin: `bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400`

Status badge: Active (green) / Inactive (gray)

Last Login: format as relative time ("2 hours ago", "5 days ago", "Never" if null)

Action buttons per row (icon buttons with tooltips):
- Edit (pencil icon): disabled if row is current user
- Reset Password (key icon): always enabled
- Delete (trash icon): disabled if row is current user OR (user.role === 'super_admin' AND superAdminCount <= 1)

"Add User" button in the tab header (top right, matches Instances tab).

State management: track which modal is open and for which user:
```typescript
const [formModal, setFormModal] = useState<{mode: 'create' | 'edit'; user?: UserRecord} | null>(null);
const [deleteModal, setDeleteModal] = useState<UserRecord | null>(null);
const [resetModal, setResetModal] = useState<UserRecord | null>(null);
```

Get current user ID from auth context (use whatever hook/context is used elsewhere, e.g., useAuth()).

Loading state: skeleton rows (3 rows, same style as InstancesTab loading state).
Empty state: centered "No users found" message with Add User button.

**Task 8: Wire UsersTab into AdministrationPage.tsx**

In `web/src/pages/AdministrationPage.tsx`:
1. Import UsersTab
2. Replace the placeholder Users tab content with `<UsersTab />`
3. Pass `currentUserRole` from auth context if UsersTab requires it (it does — for UserFormModal)

---

## Coordination Rules

- Both specialists work in parallel — backend and frontend are independent
- Backend Specialist must complete Task 1 (store.Delete) first as it unblocks Task 5 (tests)
- Frontend Specialist can start all tasks immediately
- No shared files — backend owns `internal/`, frontend owns `web/src/`
- Team Lead: merge backend first, then frontend, then run build verification

## Build Verification (run after both specialists finish)

```bash
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
```

All four commands must pass with zero errors before committing.

## Commit Messages

Backend: `feat(api): add user delete and admin password reset endpoints`
Frontend: `feat(ui): add user management tab to administration page`
Tests: `test(api): add tests for user delete and admin reset handlers`
