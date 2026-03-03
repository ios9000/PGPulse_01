# Session: 2026-03-03 — M5_01 Frontend Scaffold & Application Shell

## Goal
Initialize React + TypeScript frontend with complete build tooling, dark-first theme,
application shell, component library skeleton, and go:embed integration. Frontend builds,
embeds into Go binary, serves navigable application shell at http://localhost:8080/.

## Agent Team Configuration
- Team Lead: Opus 4.6
- Specialists: 2 (Frontend + Go Backend)
- Duration: ~3 hours (including 4 fix rounds)
- Build verification: hybrid mode (agents create files, developer runs bash)

## Planning Phase (Claude.ai)

### Framework Selection Research
- Deep research into React vs Vue 3 for monitoring dashboards
- Researched Datadog, pganalyze, Percona PMM, pgwatch UI patterns
- Researched Apache ECharts 6 capabilities

### Key Decisions Made

| ID | Decision | Rationale |
|---|---|---|
| D81 | 4-role RBAC (super_admin, roles_admin, dba, app_admin) | Finer-grained than admin/viewer; matches real DBA team structures |
| D82 | Dark-first design, Bloomberg Terminal density | DBAs live in dark terminals; information density over pretty cards |
| D83 | Zustand (client state) + TanStack Query v5 (server state) | Clean separation, no Redux overhead |
| D84 | React 18 + TypeScript (not Vue/Svelte) | Complex state management, larger ecosystem for dashboards |
| D85 | Apache ECharts via echarts-for-react | Native dark mode, rich chart types, tree-shakeable |
| D86 | Zustand + TanStack Query (confirmed) | Client vs server state separation |
| D87 | Lucide React for icons | MIT, tree-shakeable, consistent |
| D88 | go:embed via web/embed.go package export | Single binary deployment |
| D89 | Tailwind utilities only, CSS variables for theme | No CSS-in-JS, consistent design tokens |

### Deliverables Produced
- `docs/iterations/M5_01_03022026_frontend-scaffold/M5_01_requirements.md`
- `docs/iterations/M5_01_03022026_frontend-scaffold/M5_01_design.md`
- `docs/iterations/M5_01_03022026_frontend-scaffold/M5_01_team-prompt.md`

## Implementation Phase (Claude Code Agent Teams)

### Specialist 1 — Frontend (React + TypeScript)
Created ~35 files in web/ directory:
- Project config: package.json, tsconfig.json, vite.config.ts, tailwind.config.ts, eslint.config.js
- Core: main.tsx, App.tsx, index.css with CSS variables for dark/light themes
- Components: AppShell, Sidebar, TopBar, Breadcrumb, StatusBar (layout)
- Components: StatusBadge, MetricCard, DataTable, PageHeader, Spinner, EmptyState, ErrorBoundary, ThemeToggle, EChartWrapper (UI)
- Stores: themeStore (dark/light/system), layoutStore (sidebar), authStore (placeholder)
- Hooks: useHealthCheck (TanStack Query → /api/v1/health)
- Pages: FleetOverview (showcase with mock data), ServerDetail, DatabaseDetail, AlertsDashboard, AlertRules, Administration, UserManagement, Login, NotFound
- Types: models.ts, api.ts
- Lib: api.ts (fetch wrapper), echarts-setup.ts (tree-shaken), echarts-theme.ts, constants.ts

### Specialist 2 — Go Backend (Embedding + CORS)
- Created: web/embed.go (go:embed all:dist)
- Created: internal/api/static.go (static file handler with SPA fallback)
- Modified: internal/api/server.go (mount static handler, optional CORS)
- Modified: internal/config/config.go (CORSEnabled, CORSOrigin)
- Modified: configs/pgpulse.example.yml (cors settings)

## Build Fix Rounds

### Round 1 — 3 errors
- vite.config.ts: `path` and `__dirname` not available in ESM → replaced with `node:url` + `import.meta.url`
- tsconfig.node.json: missing `composite: true`, had `noEmit` → fixed
- Go scanning web/node_modules/ → changed Makefile to `./cmd/... ./internal/...`

### Round 2 — 122 errors (3 root causes)
- Missing `@types/node` → added to devDependencies
- tsconfig.json lib too low → set ES2020 + DOM + DOM.Iterable
- tsconfig.node.json missing Node types → added `"types": ["node"]`
- DataTable generic constraint → MockServer extends Record<string, unknown>

### Round 3 — ESLint 9 flat config
- ESLint 9.x dropped .eslintrc.* support → migrated to eslint.config.js flat config
- Added missing deps: @eslint/js, globals, typescript-eslint
- Updated lint script to remove deprecated flags

### Round 4 — Server exits on orchestrator failure
- Server process died when orchestrator had no connected instances
- Changed from fatal error to warning → HTTP server continues serving UI

## Final Build Output
```
✓ 2249 modules transformed
dist/index.html            1.02 kB │ gzip:   0.49 kB
dist/assets/index.css     13.19 kB │ gzip:   3.47 kB
dist/assets/index.js      41.10 kB │ gzip:  12.34 kB
dist/assets/query.js      46.54 kB │ gzip:  14.53 kB
dist/assets/react-vendor  156.15 kB │ gzip:  51.21 kB
dist/assets/echarts     1,045.71 kB │ gzip: 347.34 kB
Total gzipped: ~429 KB (under 500KB target excluding echarts)
```

## Verification Results
- `npm run build` ✅ — 2249 modules, dist/ produced
- `npm run lint` ✅ — zero warnings
- `npm run typecheck` ✅ — zero errors
- `go build ./cmd/pgpulse-server` ✅ — compiles with embedded frontend
- `go test ./cmd/... ./internal/...` ✅ — all 8 packages pass, zero regressions
- Vite dev server (localhost:5173) ✅ — app shell renders with mock data
- Go server (localhost:8080) ✅ — embedded frontend serves through chi router
- SPA routing ✅ — / redirects to /fleet via React Router
- Dark theme ✅ — default dark, toggle works
- ECharts ✅ — connections mock chart renders in dark mode
- /api/v1/health ✅ — returns 200, StatusBar shows result
- Request logging ✅ — all HTTP requests logged with IDs

## Commits
1. `feat(web): initialize React frontend with application shell (M5_01)` — main scaffold
2. `fix(server): continue serving HTTP when orchestrator has no instances` — graceful degradation

## Environment Notes
- PostgreSQL 16 installed locally on Windows (native installer, no Docker)
- DSN requires `sslmode=disable` for local dev
- `use_timescaledb: false` for plain PostgreSQL
- Vite dev server on :5173 (proxies /api to :8080) for frontend development
- Go server on :8080 serves embedded dist/ in production mode

## Not Done / Next Iteration (M5_02)
- [ ] Auth UI: Login page with JWT flow, token management
- [ ] 4-role RBAC UI: Route guards, permission-based component visibility
- [ ] User management page: CRUD for users, role assignment
- [ ] Migration: user_permissions table for server/database-level grants
- [ ] Light mode polish (dark is primary, light needs review)
- [ ] ECharts tree-shaking optimization (echarts chunk is 347KB gzipped)
- [ ] Sidebar server tree: connect to real API data
