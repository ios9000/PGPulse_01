## [M5_02] — 2026-03-03 — Auth + RBAC UI

### Added
- **Permission-based RBAC**: 4 roles (super_admin, roles_admin, dba, app_admin) × 5 permissions replacing 2-role hierarchy
- **Separate JWT refresh secret**: `refresh_secret` config field with backwards-compatible fallback to `jwt_secret`
- **Claims include permissions**: Access tokens carry `perms` array and `type` (access/refresh) discriminator
- **ValidateRefreshToken()**: Dedicated method using refresh secret, rejects access tokens
- **UserStore expanded**: 5 new methods — GetByID, List, Update, UpdatePassword, UpdateLastLogin
- **User active/deactivation**: `active` field on User, deactivated users rejected on login and refresh
- **5 new API endpoints**: POST /auth/register, GET /auth/users, PUT /auth/users/{id}, PUT /auth/me/password (19 total)
- **RequirePermission middleware**: Permission-based route guards replacing RequireRole
- **Security headers middleware**: CSP, X-Frame-Options: DENY, X-Content-Type-Options: nosniff, Referrer-Policy
- **Migration 005_expand_roles.sql**: admin→super_admin, viewer→dba, adds `active` and `last_login` columns
- **Frontend auth store**: Access token in Zustand memory, refresh token in localStorage with auto-refresh
- **Frontend API client**: apiFetch() with 401 auto-refresh and concurrent request queuing
- **Frontend permissions**: hasPermission(), getPermissions() mirroring backend RBAC
- **ProtectedRoute + PermissionGate**: Route guards for authentication and permission checks
- **Login page**: Real form with error display and 429 rate limit countdown
- **User management page**: UsersPage with create, activate/deactivate, role change
- **User dropdown in TopBar**: Username, role badge, change password, sign out
- **Sidebar permission filtering**: Nav items filtered by user permissions
- **CreateUserModal, DeactivateUserDialog, ChangePasswordModal**: Admin UI components
- **TanStack Query hooks**: useUsers, useCreateUser, useUpdateUser, useChangePassword

### Changed
- Login response now includes `user` object alongside token pair
- Login handler checks `active` field, updates `last_login` timestamp
- Refresh handler validates via separate refresh secret and checks user active status
- Alert mutation routes use `RequirePermission(PermAlertManagement)` instead of `RequireRole("admin")`
- Initial admin seed uses `super_admin` role (was: `admin`)
- `api.New()` signature unchanged but internal wiring uses permission middleware
- User.id in frontend models changed from `string` to `number`

## [M5_01] — 2026-03-03 — Frontend Scaffold & Application Shell

### Added
- **React 18 + TypeScript frontend** with Vite build pipeline
- **Dark-first theme system** with CSS variables, light mode toggle, system preference detection
- **Application shell** layout: collapsible sidebar (240px/64px), top bar with breadcrumb, content area, status bar
- **Component library skeleton**: StatusBadge, MetricCard, DataTable, PageHeader, Spinner, EmptyState, ErrorBoundary, ThemeToggle, EChartWrapper
- **Apache ECharts integration** via echarts-for-react with custom dark/light themes and tree-shaking
- **React Router v6** with 9 routes: Fleet Overview, Server Detail, Database Detail, Alerts Dashboard, Alert Rules, Administration, User Management, Login, 404
- **State management**: Zustand (theme, layout, auth stores) + TanStack Query v5 (server data fetching)
- **Fleet Overview showcase page** with mock metric cards, ECharts line chart with data zoom, sortable server table
- **go:embed integration**: frontend builds to web/dist/, embedded into Go binary, served by chi router with SPA fallback
- **CORS middleware** (optional, for Vite dev server proxy during development)
- **Health check hook**: StatusBar shows API connection status via /api/v1/health

### Changed
- Server continues running when orchestrator has no connected instances (graceful degradation)
- Go test commands use `./cmd/... ./internal/...` instead of `./...` to skip web/node_modules/

### Tech Stack
- React 18.3, TypeScript 5.8, Vite 6.4, Tailwind CSS 4.1
- Apache ECharts 5.6 via echarts-for-react 3.0
- Zustand 5.0, TanStack Query 5.75, React Router 7.6
- Lucide React for icons, ESLint 9.x with flat config
