# M5_01 Requirements — Frontend Scaffold & Application Shell

**Iteration:** M5_01  
**Folder:** `docs/iterations/M5_01_03022026_frontend-scaffold/`  
**Date:** 2026-03-02  
**Milestone:** M5 — Web UI (MVP)  
**Previous:** M4_03 (commit 7c6ab36 — alert pipeline wiring)

---

## Goal

Initialize the React + TypeScript frontend project with complete build tooling,
dark-first theme system, application shell layout, and component library skeleton.
After this iteration, the frontend builds, embeds into the Go binary via `go:embed`,
and serves a navigable (but empty) application shell at `http://localhost:8080/`.

**This iteration produces NO data views, NO auth flow, NO API integration.**
It delivers the empty container that all subsequent M5 iterations fill.

---

## Success Criteria

1. `cd web && npm run build` produces optimized output in `web/dist/`
2. `go build ./cmd/pgpulse-server` compiles with `//go:embed web/dist` — single binary serves frontend
3. Navigating to `http://localhost:8080/` serves the application shell
4. Dark mode is the default; light mode toggle works and persists in localStorage
5. Application shell renders: collapsible sidebar, top bar, breadcrumb, content area
6. React Router navigates between placeholder pages without full-page reload
7. At least one ECharts demo chart renders correctly in dark mode
8. TanStack Query provider and Zustand store are wired (no real queries yet)
9. `npm run lint` and `npm run typecheck` pass with zero errors
10. Total production bundle size < 500KB gzipped

---

## Functional Requirements

### FR-01: Project Initialization

- Initialize React 18 + TypeScript project using Vite in `web/` directory
- Configure TypeScript strict mode
- Configure ESLint + Prettier for code quality
- Configure path aliases (`@/components`, `@/hooks`, `@/stores`, etc.)
- Create `.env.development` with `VITE_API_BASE_URL=http://localhost:8080/api/v1`
- Create `.env.production` with `VITE_API_BASE_URL=/api/v1`

### FR-02: Tailwind CSS + Dark Theme System

- Install and configure Tailwind CSS v3 with `darkMode: 'class'`
- Define PGPulse color palette as Tailwind theme extension:
  - Background: `pgp-bg-primary` (#0f1117), `pgp-bg-secondary` (#1a1d23), `pgp-bg-card` (#21252b)
  - Accent: `pgp-accent` (#3b82f6 — electric blue)
  - Status: `pgp-ok` (#14b8a6 — teal), `pgp-warning` (#f59e0b — amber), `pgp-critical` (#ef4444 — coral red), `pgp-info` (#6366f1 — indigo)
  - Text: `pgp-text-primary` (#f8fafc), `pgp-text-secondary` (#94a3b8), `pgp-text-muted` (#64748b)
  - Light mode equivalents for all colors
- Zustand store for theme state (dark/light/system)
- Theme toggle component in top bar
- `<html>` class toggles `dark` based on store

### FR-03: Application Shell Layout

The shell is the persistent frame around all page content:

```
┌──────────────────────────────────────────────────────────┐
│  Top Bar: [☰ toggle] [breadcrumb]     [🔍] [🔔] [👤]    │
├──────────┬───────────────────────────────────────────────┤
│          │                                               │
│ Sidebar  │  Content Area                                 │
│          │                                               │
│ • Fleet  │  (React Router <Outlet/>)                     │
│ • Alerts │                                               │
│ • Admin  │                                               │
│          │                                               │
│ Servers: │                                               │
│ ▸ prod-1 │                                               │
│ ▸ prod-2 │                                               │
│ ▸ stg-1  │                                               │
│          │                                               │
├──────────┴───────────────────────────────────────────────┤
│  Status Bar (optional): connected | last refresh 5s ago  │
└──────────────────────────────────────────────────────────┘
```

**Sidebar (left):**
- Collapsible (hamburger icon in top bar)
- Collapsed state shows icons only, expanded shows icons + labels
- Width: 240px expanded, 64px collapsed
- Collapse state persisted in Zustand
- Navigation sections:
  - Fleet Overview (icon: grid/server)
  - Alerts (icon: bell)
  - Administration (icon: gear) — placeholder for M5_02
  - Server tree (icon: database) — placeholder list, populated in M5_03
- Active route highlighted with accent color left border

**Top Bar:**
- Left: hamburger toggle + breadcrumb trail
- Right: global search icon (placeholder), notification bell (placeholder), theme toggle, user avatar dropdown (placeholder)
- Height: 48px
- Sticky/fixed at top

**Content Area:**
- Renders `<Outlet/>` from React Router
- Scrollable independently from sidebar
- Responsive padding

### FR-04: React Router Configuration

Routes (all render placeholder pages in M5_01):

| Path | Page | Sidebar Item |
|---|---|---|
| `/` | Redirect to `/fleet` | — |
| `/fleet` | Fleet Overview (placeholder) | Fleet Overview |
| `/servers/:serverId` | Server Detail (placeholder) | Server tree item |
| `/servers/:serverId/databases/:dbName` | Database Detail (placeholder) | — |
| `/alerts` | Alerts Dashboard (placeholder) | Alerts |
| `/alerts/rules` | Alert Rules (placeholder) | — |
| `/admin` | Administration (placeholder) | Administration |
| `/admin/users` | User Management (placeholder) | — |
| `/login` | Login Page (placeholder, outside shell) | — |
| `*` | 404 Not Found page | — |

- Login page renders WITHOUT the application shell (full-screen)
- All other routes render WITHIN the application shell
- Placeholder pages show: page title, route params, "Coming in M5_0X" note

### FR-05: Apache ECharts Integration

- Install `echarts` and `echarts-for-react`
- Create `<EChartWrapper>` component that:
  - Auto-applies dark/light theme based on Zustand theme store
  - Auto-resizes on container resize (ResizeObserver)
  - Accepts standard ECharts option as prop
  - Provides loading state
- Create a demo chart on the Fleet Overview placeholder page:
  - Simple line chart with mock time-series data (e.g., "Connections over time")
  - Demonstrates dark mode theming works
  - Demonstrates responsive resize

### FR-06: TanStack Query Setup

- Install `@tanstack/react-query` and `@tanstack/react-query-devtools`
- Create `QueryClientProvider` at app root with defaults:
  - `staleTime: 30_000` (30s)
  - `refetchOnWindowFocus: true`
  - `retry: 2`
- Create `web/src/lib/api.ts` — base fetch wrapper:
  - Reads `VITE_API_BASE_URL` from env
  - Attaches JWT token from Zustand auth store (header: `Authorization: Bearer <token>`)
  - Returns typed JSON responses
  - Handles 401 → redirect to login
- Create one example query hook: `useHealthCheck()` that calls `GET /api/v1/health`
  - Demonstrate the hook works on Fleet Overview placeholder
  - Show connection status in the status bar

### FR-07: Zustand Stores

Create initial stores (minimal, expanded in later iterations):

| Store | State | Purpose |
|---|---|---|
| `useThemeStore` | `theme: 'dark' \| 'light' \| 'system'`, `resolvedTheme: 'dark' \| 'light'` | Theme preference |
| `useLayoutStore` | `sidebarCollapsed: boolean`, `sidebarWidth: number` | Layout state |
| `useAuthStore` | `token: string \| null`, `user: User \| null`, `isAuthenticated: boolean` | Auth state (placeholder, populated in M5_02) |

- All stores use Zustand `persist` middleware for localStorage

### FR-08: Build Pipeline & Go Embedding

- Vite builds to `web/dist/` (production output)
- Go server serves `web/dist/` via `//go:embed` directive
- Create `internal/api/static.go`:
  - `//go:embed web/dist` with proper path from module root
  - Serves index.html for all non-API routes (SPA fallback)
  - Serves static assets with correct MIME types and cache headers
  - Assets with hash in filename: `Cache-Control: public, max-age=31536000, immutable`
  - `index.html`: `Cache-Control: no-cache` (always fresh)
- API routes (`/api/v1/*`) take priority over static file serving
- Development mode: Vite dev server on port 5173, Go API on port 8080, CORS enabled

### FR-09: Component Library Skeleton

Create reusable base components (styled, not yet connected to data):

| Component | File | Purpose |
|---|---|---|
| `StatusBadge` | `components/ui/StatusBadge.tsx` | Colored badge: ok/warning/critical/info/unknown |
| `MetricCard` | `components/ui/MetricCard.tsx` | KPI display: label, value, unit, optional sparkline, optional trend arrow |
| `DataTable` | `components/ui/DataTable.tsx` | Sortable, paginated table shell (no data yet) |
| `PageHeader` | `components/ui/PageHeader.tsx` | Page title + optional actions area + breadcrumb integration |
| `Spinner` | `components/ui/Spinner.tsx` | Loading indicator |
| `EmptyState` | `components/ui/EmptyState.tsx` | "No data" placeholder with icon and message |
| `ErrorBoundary` | `components/ui/ErrorBoundary.tsx` | React error boundary with fallback UI |
| `ThemeToggle` | `components/ui/ThemeToggle.tsx` | Dark/light/system toggle button |

Each component:
- Has TypeScript props interface
- Supports dark mode via Tailwind `dark:` variants
- Is self-contained (no external data dependencies)

---

## Non-Functional Requirements

### NFR-01: Bundle Size
- Production build (gzipped): < 500KB total
- ECharts tree-shaking: import only used chart types and components

### NFR-02: TypeScript Strictness
- `strict: true` in tsconfig.json
- No `any` types except in third-party type patches
- All component props explicitly typed

### NFR-03: Code Quality
- ESLint with `@typescript-eslint/recommended`
- Prettier for formatting
- `npm run lint` and `npm run typecheck` must pass with zero errors/warnings

### NFR-04: Accessibility
- Semantic HTML throughout
- Keyboard navigation for sidebar and top bar
- `aria-label` on icon-only buttons
- Focus visible indicators
- Minimum 4.5:1 contrast ratio for text

### NFR-05: Responsive Design
- Desktop first (1280px+), but sidebar collapses gracefully below 1024px
- Content area uses fluid width

---

## Out of Scope

- Authentication flow (M5_02)
- Real API data fetching beyond health check (M5_03+)
- Server list population from API (M5_03)
- Alert views (M5_05)
- User management UI (M5_02)
- SSE/WebSocket streaming (deferred post-MVP)
- i18n/localization

---

## Dependencies

### From Existing Codebase
- `GET /api/v1/health` endpoint (exists since M2)
- `internal/api/server.go` — needs modification to add static file serving
- `cmd/pgpulse-server/main.go` — no changes needed in this iteration

### New npm Packages
- react, react-dom (18.x)
- typescript (5.x)
- vite, @vitejs/plugin-react
- tailwindcss, postcss, autoprefixer
- echarts, echarts-for-react
- @tanstack/react-query, @tanstack/react-query-devtools
- zustand
- react-router-dom (6.x)
- lucide-react (icons)
- eslint, prettier, @typescript-eslint/*

---

## Risk Assessment

| Risk | Impact | Mitigation |
|---|---|---|
| go:embed path resolution from module root | Build breaks | Test embed path early; may need embed directive in web/ package |
| ECharts bundle too large | >500KB limit | Tree-shake: import only Line, Bar, Pie + required components |
| Tailwind dark mode class not propagating | Broken theme | Test class strategy on `<html>` element early |
| Vite dev server + Go API CORS issues | Dev workflow blocked | Configure CORS middleware in Go (dev mode only) |
| SPA fallback serving conflicts with API routes | 404 on API calls | Ensure API route prefix match before static fallback |
