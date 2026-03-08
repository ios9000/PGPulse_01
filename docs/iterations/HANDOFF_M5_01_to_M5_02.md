# PGPulse - Iteration Handoff: M5_01 to M5_02

## DO NOT RE-DISCUSS

These decisions are FINAL:

1. D84 React 18 + TypeScript (not Vue, not Svelte)
2. D85 Apache ECharts via echarts-for-react
3. D86 Zustand (client state) + TanStack Query v5 (server state)
4. D87 Lucide React for icons
5. D88 go:embed via web/embed.go
6. D89 Tailwind utility classes only, CSS variables for theme tokens
7. D81 4-role RBAC: super_admin, roles_admin, dba, app_admin
8. D82 Dark-first design language
9. Frontend stack: React 18.3, TS 5.8, Vite 6.4, Tailwind 4.1, ECharts 5.6
10. ESLint 9.x with flat config (eslint.config.js)
11. Go test scope: ./cmd/... ./internal/... (skip node_modules)
12. Server continues running when orchestrator has no instances

## What Exists Now

### Backend (M0-M4 Complete)
- 13 basic collectors + replication + progress monitoring
- PostgreSQL storage with TimescaleDB hypertables, batch writes via COPY
- JWT auth with RBAC (admin/viewer), rate limiting
- 19 alert rules, email notifier, async dispatcher
- 14 REST API endpoints: health, instances, auth, alerts

### Frontend (M5_01 Complete)
- React 18 + TS + Vite pipeline, go:embed serving through chi
- Dark-first theme with CSS variables, toggle (dark/light/system)
- App shell: collapsible sidebar, top bar, breadcrumb, status bar
- Components: StatusBadge, MetricCard, DataTable, PageHeader, Spinner, EmptyState, ErrorBoundary, ThemeToggle, EChartWrapper
- Fleet Overview with mock data
- Zustand stores: theme, layout, auth (placeholder)
- TanStack Query: useHealthCheck hook
- API client: apiFetch with Bearer token, 401 handling

### Routes
/login (outside shell), /fleet (index), /servers/:id, /databases/:id, /alerts, /alerts/rules, /admin, /admin/users, * (404)

### Auth API
POST /api/v1/auth/login - returns token + user
POST /api/v1/auth/register - creates user (admin only)
Current roles: admin, viewer

## What Was Just Completed (M5_01)
Frontend scaffold with 35+ files. 4 build fix rounds. Verified: build, lint, typecheck, go build, go test, embedded serving at localhost:8080.

## Known Issues
1. ECharts chunk 347KB gzipped (optimize in M5_03)
2. Example instances in config have invalid DSN (sample data)
3. configs/pgpulse.yml not in git (local credentials)

## Environment
Windows 10, Go 1.24.0, Node 22.14.0, Claude Code v2.1.63, PostgreSQL 16 local

## Next Task: M5_02 Auth + RBAC UI

### Goal
Connect frontend auth to backend JWT system. Login page, token management, route guards, role-based visibility.

### Key Items
1. Login page: real JWT flow replacing mock form
2. Auth store: token persistence, login/logout, token refresh
3. Route guards: ProtectedRoute, RoleGuard components
4. API client: verify auth headers, 401 redirect
5. User management page: user CRUD for authorized roles
6. Sidebar: role-based nav item visibility
7. Evaluate: expand admin/viewer to 4-role model or defer

### Build Commands
cd web && npm run build && npm run lint && npm run typecheck
cd .. && go build ./cmd/pgpulse-server && go test ./cmd/... ./internal/...
