# M5_07 — User Management UI: Design

**Iteration:** M5_07_03082026_user-management-ui
**Date:** 2026-03-08

---

## Backend Design

### New Handler: handleDeleteUser

Location: `internal/api/auth.go` (append to existing file)

```go
func (s *APIServer) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid user ID")
        return
    }

    // Prevent self-deletion.
    claims := auth.ClaimsFromContext(r.Context())
    if claims != nil && claims.UserID == id {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "cannot delete your own account")
        return
    }

    // Fetch user to check role before deleting.
    target, err := s.userStore.GetByID(r.Context(), id)
    if err != nil {
        if errors.Is(err, auth.ErrUserNotFound) {
            writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
            return
        }
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch user")
        return
    }

    // Prevent deleting last super_admin.
    if target.Role == string(auth.RoleSuperAdmin) {
        users, err := s.userStore.List(r.Context())
        if err != nil {
            writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to check super_admin count")
            return
        }
        superAdminCount := 0
        for _, u := range users {
            if u.Role == string(auth.RoleSuperAdmin) && u.Active {
                superAdminCount++
            }
        }
        if superAdminCount <= 1 {
            writeError(w, http.StatusBadRequest, "BAD_REQUEST", "cannot delete the last active super_admin")
            return
        }
    }

    if err := s.userStore.Delete(r.Context(), id); err != nil {
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete user")
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
```

**Note:** `UserStore.Delete` does not exist yet. Must be added to the interface and PGUserStore:

```go
// Add to UserStore interface in internal/auth/store.go:
Delete(ctx context.Context, id int64) error

// Add to PGUserStore in internal/auth/store.go:
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

### New Handler: handleAdminResetPassword

Location: `internal/api/auth.go` (append to existing file)

```go
// adminResetPasswordRequest is the JSON body for PUT /api/v1/users/{id}/password.
type adminResetPasswordRequest struct {
    NewPassword string `json:"new_password"`
}

func (s *APIServer) handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid user ID")
        return
    }

    var req adminResetPasswordRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
        return
    }

    if len(req.NewPassword) < 8 {
        writeError(w, http.StatusBadRequest, "BAD_REQUEST", "password must be at least 8 characters")
        return
    }

    // Verify target user exists.
    if _, err := s.userStore.GetByID(r.Context(), id); err != nil {
        if errors.Is(err, auth.ErrUserNotFound) {
            writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
            return
        }
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch user")
        return
    }

    hash, err := auth.HashPassword(req.NewPassword, s.authCfg.BcryptCost)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
        return
    }

    if err := s.userStore.UpdatePassword(r.Context(), id, hash); err != nil {
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to reset password")
        return
    }

    writeJSON(w, http.StatusOK, map[string]string{"status": "password reset"})
}
```

### Route Registration in router.go

Verify that the following routes exist under the authenticated + user_management-gated group.
Add any that are missing:

```go
// User management routes (requires PermUserManagement: super_admin, roles_admin)
r.Group(func(r chi.Router) {
    r.Use(auth.RequirePermission(auth.PermUserManagement, writeError))
    r.Get("/users", s.handleListUsers)
    r.Post("/users", s.handleRegister)
    r.Put("/users/{id}", s.handleUpdateUser)
    r.Delete("/users/{id}", s.handleDeleteUser)           // NEW
    r.Put("/users/{id}/password", s.handleAdminResetPassword) // NEW
})
```

---

## Frontend Design

### TypeScript Types (models.ts)

Confirm or add `UserRecord` type:

```typescript
export interface UserRecord {
  id: number;
  username: string;
  role: 'super_admin' | 'roles_admin' | 'dba' | 'app_admin';
  active: boolean;
  permissions: string[];
  last_login?: string | null;  // ISO timestamp or null
  created_at?: string;
}

export type UserRole = UserRecord['role'];

export interface UsersListResponse {
  users: UserRecord[];
}
```

### Hook: useUserManagement.ts

Location: `web/src/hooks/useUserManagement.ts`

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../lib/api';
import type { UserRecord } from '../types/models';

export function useUsers() {
  return useQuery<UserRecord[]>({
    queryKey: ['users'],
    queryFn: async () => {
      const data = await apiClient.get<{ users: UserRecord[] }>('/users');
      return data.users;
    },
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: { username: string; password: string; role: string }) =>
      apiClient.post('/users', payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...fields }: { id: number; role?: string; active?: boolean }) =>
      apiClient.put(`/users/${id}`, fields),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiClient.delete(`/users/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useResetUserPassword() {
  return useMutation({
    mutationFn: ({ id, newPassword }: { id: number; newPassword: string }) =>
      apiClient.put(`/users/${id}/password`, { new_password: newPassword }),
  });
}
```

Note: `apiClient.delete` may not exist yet — add it to `web/src/lib/api.ts` alongside get/post/put.

### Component: UsersTab.tsx

Location: `web/src/components/admin/UsersTab.tsx`

**Layout:** Match InstancesTab.tsx pattern exactly.

```
┌─ UsersTab ──────────────────────────────────────────────────────────┐
│  Users                                        [+ Add User]          │
│  ─────────────────────────────────────────────────────────          │
│  Username    Role          Status    Last Login    Created  Actions  │
│  ──────────────────────────────────────────────────────────────     │
│  alice       [super_admin] [Active]  2 hours ago   Mar 1   ✏ 🔑 🗑  │
│  bob         [dba]         [Active]  5 days ago    Feb 15  ✏ 🔑 🗑  │
│  carol       [app_admin]   [Inactive] Never        Feb 20  ✏ 🔑 🗑  │
└────────────────────────────────────────────────────────────────────┘
```

**Role badge colors (Tailwind):**
- `super_admin`: `bg-red-100 text-red-800`
- `roles_admin`: `bg-orange-100 text-orange-800`
- `dba`: `bg-blue-100 text-blue-800`
- `app_admin`: `bg-green-100 text-green-800`

**Action buttons:**
- Edit (pencil): always shown, disabled for own row
- Reset Password (key): always shown
- Delete (trash): always shown, disabled for own row or last super_admin

**Own-row detection:** Compare `user.id` against the current user's ID from JWT claims (available via `useAuth()` or the existing auth context).

### Component: UserFormModal.tsx

Location: `web/src/components/admin/UserFormModal.tsx`

Props:
```typescript
interface UserFormModalProps {
  mode: 'create' | 'edit';
  user?: UserRecord;       // populated in edit mode
  onClose: () => void;
  onSuccess: () => void;
  currentUserRole: UserRole;
}
```

**Create mode fields:**
- Username: text input, required
- Password: password input, required, min 8 chars
- Role: select dropdown (options filtered by `currentUserRole`)

**Edit mode fields:**
- Username: read-only display (cannot change)
- Role: select dropdown (same filtering)
- Active: toggle/checkbox

**Role options visible to each caller:**
```typescript
function getAssignableRoles(currentUserRole: UserRole): UserRole[] {
  if (currentUserRole === 'super_admin') {
    return ['super_admin', 'roles_admin', 'dba', 'app_admin'];
  }
  if (currentUserRole === 'roles_admin') {
    return ['dba', 'app_admin'];
  }
  return [];
}
```

**Error handling:** Show inline error below the relevant field. API conflict error (username taken) maps to the username field.

### Component: DeleteUserModal.tsx

Location: `web/src/components/admin/DeleteUserModal.tsx`

Props:
```typescript
interface DeleteUserModalProps {
  user: UserRecord;
  onClose: () => void;
  onSuccess: () => void;
}
```

Content: "Are you sure you want to delete user **{username}**? This action cannot be undone."

If `user.role === 'super_admin'`: add warning "Warning: This user is a super administrator."

### Component: ResetPasswordModal.tsx

Location: `web/src/components/admin/ResetPasswordModal.tsx`

Props:
```typescript
interface ResetPasswordModalProps {
  user: UserRecord;
  onClose: () => void;
}
```

Fields:
- New Password: password input, required, min 8 chars
- Confirm Password: password input, must match

Client-side validation before submit. No "current password" field — this is an admin reset.

---

## Test Coverage Required

### Backend unit tests (append to existing test files or new file)

- `TestHandleDeleteUser_Self` → 400
- `TestHandleDeleteUser_LastSuperAdmin` → 400
- `TestHandleDeleteUser_NotFound` → 404
- `TestHandleDeleteUser_Success` → 200
- `TestHandleAdminResetPassword_TooShort` → 400
- `TestHandleAdminResetPassword_NotFound` → 404
- `TestHandleAdminResetPassword_Success` → 200
- `TestUserStore_Delete_Success` (unit, mock store)
- `TestUserStore_Delete_NotFound` (unit, mock store)

### Frontend (smoke level — just that it renders without crash)
- UsersTab renders with empty list
- UsersTab renders with populated list
- UserFormModal opens and closes in create mode
- UserFormModal opens and closes in edit mode
- DeleteUserModal renders username
- ResetPasswordModal validates password match

---

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Add `Delete` to UserStore interface | Needed for handleDeleteUser; fits the existing store pattern |
| D2 | Admin reset is a separate endpoint from self-service change | Different security model — admin doesn't need current password |
| D3 | Role filtering in UI mirrors backend validation | Belt-and-suspenders; backend is the authority |
| D4 | No soft-delete — hard DELETE from users table | Simplest for MVP; audit log is out of scope |
| D5 | Disable (not hide) action buttons for restricted operations | Better UX than hiding; user understands what exists |
| D6 | Last active super_admin protection is server-enforced | Cannot rely on frontend counting to protect this invariant |
