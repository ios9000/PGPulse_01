# M5_07 — User Management UI: Requirements

**Iteration:** M5_07_03082026_user-management-ui
**Date:** 2026-03-08
**Milestone:** M5 (Web UI MVP)
**Scope:** Complete the Administration page by implementing the Users tab

---

## Goal

Implement the Users tab on the Administration page so admins can manage PGPulse
user accounts without touching the database directly. This completes the MVP
Administration page (Instances tab already done in M5_06).

---

## Context: What Already Exists

### Backend (DO NOT REWRITE)

All of the following are complete, tested, and must not change:

| Component | File | Status |
|-----------|------|--------|
| UserStore interface | `internal/auth/store.go` | ✅ Done |
| PGUserStore (8 methods) | `internal/auth/store.go` | ✅ Done |
| handleLogin, handleRefresh, handleMe | `internal/api/auth.go` | ✅ Done |
| handleListUsers | `internal/api/auth.go` | ✅ Done |
| handleRegister (create user) | `internal/api/auth.go` | ✅ Done |
| handleUpdateUser (role + active) | `internal/api/auth.go` | ✅ Done |
| handleChangePassword (self, requires current) | `internal/api/auth.go` | ✅ Done |
| RBAC: 4 roles, 5 permissions | `internal/auth/rbac.go` | ✅ Done |
| JWT, bcrypt, rate limiter, middleware | `internal/auth/` | ✅ Done |

### Roles (existing, do not add or rename)

| Role | Permissions |
|------|-------------|
| `super_admin` | user_management, instance_management, alert_management, view_all, self_management |
| `roles_admin` | user_management, view_all, self_management |
| `dba` | instance_management, alert_management, view_all, self_management |
| `app_admin` | alert_management, view_all, self_management |

---

## What Must Be Built

### Backend (minimal additions only)

**1. `handleDeleteUser` — DELETE /api/v1/users/:id**

New handler in `internal/api/auth.go`:
- Requires `PermUserManagement`
- Prevent self-deletion (return 400)
- Prevent deleting the last `super_admin` (return 400 with clear error)
- Return 404 if user not found
- Return 200 `{"status": "deleted"}`

**2. `handleAdminResetPassword` — PUT /api/v1/users/:id/password**

New handler in `internal/api/auth.go`:
- Admin sets a new password for another user without knowing the current one
- Requires `PermUserManagement`
- Cannot use `handleChangePassword` (that requires current_password and is for self-service)
- Body: `{"new_password": "..."}`
- Validate password length ≥ 8 characters
- Hash with bcrypt via `s.authCfg.BcryptCost`
- Call `s.userStore.UpdatePassword()`
- Return 200 `{"status": "password reset"}`

**3. Register routes in `internal/api/router.go`**

The user management routes need to be registered. Confirm or add:
```
GET    /api/v1/users                  → handleListUsers          (PermUserManagement)
POST   /api/v1/users                  → handleRegister           (PermUserManagement)
PUT    /api/v1/users/{id}             → handleUpdateUser         (PermUserManagement)
DELETE /api/v1/users/{id}             → handleDeleteUser         (PermUserManagement) [NEW]
PUT    /api/v1/users/{id}/password    → handleAdminResetPassword (PermUserManagement) [NEW]
```

Self-service endpoints (already registered, no change):
```
PUT    /api/v1/auth/me/password       → handleChangePassword     (any authenticated user)
```

### Frontend (main work)

**New files:**

| File | Description |
|------|-------------|
| `web/src/components/admin/UsersTab.tsx` | Main users tab with list + action buttons |
| `web/src/components/admin/UserFormModal.tsx` | Create and edit user modal |
| `web/src/components/admin/DeleteUserModal.tsx` | Delete confirmation modal |
| `web/src/components/admin/ResetPasswordModal.tsx` | Admin password reset modal |
| `web/src/hooks/useUserManagement.ts` | TanStack Query hooks for all user CRUD operations |

**Modified files:**

| File | Change |
|------|--------|
| `web/src/pages/AdministrationPage.tsx` | Wire in UsersTab (replace placeholder) |
| `web/src/types/models.ts` | Add/confirm UserRecord type |

---

## Functional Requirements

### FR-1: User List

- Display all users in a table
- Columns: Username, Role (badge with color), Status (Active/Inactive badge), Last Login (relative time), Created, Actions
- Role badges: color-coded per role (super_admin=red, roles_admin=orange, dba=blue, app_admin=green)
- Inactive users shown with dimmed row or "Inactive" badge
- Empty state: "No users found" with Add User button
- Loading skeleton while fetching
- Current user row highlighted (cannot delete self, cannot deactivate self)

### FR-2: Create User

- "Add User" button opens `UserFormModal` in create mode
- Fields: Username (required), Password (required, min 8 chars), Role (select dropdown)
- Role dropdown shows only roles the current user can assign:
  - `super_admin`: can assign all 4 roles
  - `roles_admin`: can assign `dba` and `app_admin` only
- On success: close modal, refetch list, show success toast
- On error (duplicate username): show inline error "Username already exists"

### FR-3: Edit User

- Edit button (pencil icon) opens `UserFormModal` in edit mode
- Editable fields: Role (select), Active (toggle)
- Password field NOT shown in edit mode (separate Reset Password action)
- Cannot demote/deactivate own account (edit button disabled for current user's own row)
- Role constraint: same rules as create (roles_admin cannot promote to super_admin/roles_admin)

### FR-4: Delete User

- Delete button (trash icon) opens `DeleteUserModal`
- Modal shows username and role of user being deleted
- Warning if deleting a super_admin
- Delete button disabled + tooltip if:
  - Attempting to delete own account
  - User is the last super_admin
- On success: close modal, refetch list

### FR-5: Admin Password Reset

- "Reset Password" button (key icon) in actions column, or inside Edit modal as a secondary action
- Opens `ResetPasswordModal`
- Single field: New Password (required, min 8 chars) + Confirm Password
- Client-side validation: passwords must match
- On success: close modal, show toast "Password reset successfully"
- Does NOT require the user's current password

### FR-6: Access Control (UI enforcement)

- Users tab only visible to users with `user_management` permission (super_admin, roles_admin)
- Action buttons rendered based on current user's role:
  - `roles_admin` cannot see delete/edit for super_admin or roles_admin users
  - Current user's own row: no delete, no deactivate
- These are UI guards only — backend enforces the real rules

### FR-7: Self-Service Password Change

- Existing functionality (not in this iteration): the current user can change their own password via profile/settings. **Out of scope for M5_07 — do not implement.**

---

## Non-Functional Requirements

- Match the visual style of `InstancesTab.tsx` exactly (same table structure, same modal patterns, same button styles)
- Use TanStack Query for all data fetching and mutations
- Optimistic updates not required — refetch on success is sufficient
- All modals accessible via keyboard (focus trap, Escape to close)
- Consistent error handling: API errors surfaced as inline messages in modals

---

## Out of Scope (noted for future)

- Self-service password change UI (FR-7 above)
- LDAP/AD integration (future M8+)
- User profile page
- Audit log of user management actions
- One-time password reset tokens
- Email-based password reset
