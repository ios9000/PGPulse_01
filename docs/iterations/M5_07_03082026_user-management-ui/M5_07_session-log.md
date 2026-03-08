# Session: 2026-03-08 — M5_07 User Management UI

## Goal
Complete the Administration page by implementing the Users tab — user list, create,
edit, delete, and admin password reset. This completes the MVP Administration page
(Instances tab was done in M5_06).

## Agent Team Configuration
- Team Lead: Claude Code
- Specialists: Backend (API & Auth) + Frontend
- Duration: 1 session
- Build result: all green (go build, go vet, golangci-lint 0 issues, npm run build)

## What Was Built

### Backend (internal/auth/store.go, internal/api/auth.go, internal/api/server.go)

**New UserStore methods (interface + PGUserStore):**
- `CountActiveByRole(ctx, role)` — counts active users by role; used for last-super-admin guard
- `Delete(ctx, id)` — hard DELETE, returns ErrUserNotFound if absent

**New API handlers:**
- `handleDeleteUser` — DELETE /api/v1/auth/users/{id}
  Guards: no self-deletion, no deleting last active super_admin
- `handleAdminResetPassword` — PUT /api/v1/auth/users/{id}/password
  Min 8 char validation; does not require current password

**Route registration (internal/api/server.go):**
Both new routes registered in the PermUserManagement group.

**Tests updated:**
- `internal/api/auth_test.go` — mockUserStore updated with CountActiveByRole() and Delete()

### Frontend

**New components:**
- `web/src/components/admin/UsersTab.tsx` — table with role badges, status badges, action buttons (edit/reset/delete), self-user detection, last-super-admin protection
- `web/src/components/admin/UserFormModal.tsx` — create/edit modal, role filtering by current user's role
- `web/src/components/admin/DeleteUserModal.tsx` — confirmation dialog with super_admin warning
- `web/src/components/admin/ResetPasswordModal.tsx` — password reset with confirmation field

**Modified:**
- `web/src/hooks/useUsers.ts` — added useDeleteUser() and useResetUserPassword()
- `web/src/pages/Administration.tsx` — replaced placeholder with UsersTab

## Architecture Decisions
- `CountActiveByRole` added to store rather than counting in handler after List() — cleaner, single query
- Admin reset is a separate endpoint from self-service change-password — different security model (no current password required)
- Last-super-admin guard is server-enforced; UI disables the button as a UX hint only
- Hard delete (no soft delete) — simplest for MVP; audit log is out of scope

## Build Verification
- `go build ./...` ✅
- `go vet ./...` ✅
- `golangci-lint run` ✅ 0 issues
- `npm run build` ✅ frontend clean

## Deferred / Not Done
- Self-service password change UI (user changes own password from profile) — out of scope for M5
- LDAP/AD integration — noted for future (M8+)
- Audit log of user management actions — out of scope for MVP

## What's Next
M5 is now complete. Next milestone: M6 — OS Agent (procfs metrics, Patroni/ETCD status).
