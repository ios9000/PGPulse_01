# M5_01 Team Prompt — Frontend Scaffold & Application Shell

> **Paste this into Claude Code to start the M5_01 iteration.**
> Read CLAUDE.md first for full project context.
> Design doc: `docs/iterations/M5_01_03022026_frontend-scaffold/design.md`

---

## Context

PGPulse is a PostgreSQL monitoring tool written in Go. We have completed M0–M4:
a working Go backend with collectors, TimescaleDB storage, JWT auth, REST API
(14 endpoints), and an alerting pipeline. Now we need a frontend.

This iteration sets up the React + TypeScript project, build pipeline, dark-first
theme, application shell, component library skeleton, and Go embedding. No data
views, no auth flow, no real API calls beyond a health check demo.

**Agents cannot run bash on this platform.** All shell commands (npm install,
npm run build, go build, go test) must be run manually by the developer.
Focus entirely on creating and editing files. At the end, list all files created
so the developer can run the build.

---

## Create a team of 2 specialists:

### SPECIALIST 1 — FRONTEND (React + TypeScript)

Initialize and build the complete frontend project in `web/` directory.

**Step 1: Project files** (create these first)

Create `web/package.json`:
```json
{
  "name": "pgpulse-web",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "lint": "eslint . --ext ts,tsx --report-unused-disable-directives --max-warnings 0",
    "typecheck": "tsc --noEmit",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.28.0",
    "@tanstack/react-query": "^5.62.0",
    "@tanstack/react-query-devtools": "^5.62.0",
    "zustand": "^5.0.0",
    "echarts": "^5.5.1",
    "echarts-for-react": "^3.0.2",
    "lucide-react": "^0.460.0"
  },
  "devDependencies": {
    "@types/react": "^18.3.12",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.4",
    "typescript": "~5.6.3",
    "vite": "^6.0.0",
    "tailwindcss": "^3.4.16",
    "postcss": "^8.4.49",
    "autoprefixer": "^10.4.20",
    "eslint": "^9.15.0",
    "@typescript-eslint/eslint-plugin": "^8.15.0",
    "@typescript-eslint/parser": "^8.15.0",
    "eslint-plugin-react-hooks": "^5.0.0",
    "eslint-plugin-react-refresh": "^0.4.14",
    "prettier": "^3.4.1"
  }
}
```

Create `web/tsconfig.json`:
- `strict: true`
- `paths`: `@/*` → `./src/*`
- `target`: `ES2020`
- `module`: `ESNext`
- `jsx`: `react-jsx`

Create `web/tsconfig.node.json` for Vite config.

Create `web/vite.config.ts`:
- React plugin
- Path alias: `@` → `./src`
- Build output: `dist/`
- Manual chunks: `echarts`, `react-vendor`, `query`
- Dev server proxy: `/api` → `http://localhost:8080`

Create `web/tailwind.config.ts`:
- `darkMode: 'class'`
- Extended colors using CSS variables (`pgp` prefix)
- Custom widths for sidebar states
- Font family: Inter (sans), JetBrains Mono (mono)

Create `web/postcss.config.js` (tailwindcss + autoprefixer)

Create `web/.eslintrc.cjs` (TypeScript + React Hooks + React Refresh rules)

Create `web/.prettierrc` (`semi: false`, `singleQuote: true`, `trailingComma: 'all'`)

Create `web/.env.development`: `VITE_API_BASE_URL=http://localhost:8080/api/v1`

Create `web/.env.production`: `VITE_API_BASE_URL=/api/v1`

Create `web/.gitignore`: `node_modules/`, `dist/`, `*.local`

Create `web/index.html`:
- `<html lang="en" class="dark">` (dark by default)
- Import Inter and JetBrains Mono from Google Fonts CDN
- `<div id="root"></div>`
- `<script type="module" src="/src/main.tsx"></script>`

Create `web/public/favicon.svg`: Simple SVG icon — database symbol with pulse line

**Step 2: Core source files**

Create `web/src/index.css`:
- Tailwind directives (@tailwind base/components/utilities)
- CSS variables in `:root` (light) and `.dark` (dark)
- Light mode colors: bg-primary #ffffff, bg-secondary #f8fafc, bg-card #ffffff, bg-hover #f1f5f9, border #e2e8f0, accent #3b82f6, ok #14b8a6, warning #f59e0b, critical #ef4444, info #6366f1, text-primary #0f172a, text-secondary #475569, text-muted #94a3b8
- Dark mode colors: bg-primary #0f1117, bg-secondary #1a1d23, bg-card #21252b, bg-hover #2a2f38, border #2e3440, accent #3b82f6, accent-hover #60a5fa, ok #14b8a6, warning #f59e0b, critical #ef4444, info #818cf8, text-primary #f8fafc, text-secondary #94a3b8, text-muted #64748b
- Body styles: `font-family: Inter, system-ui, sans-serif; margin: 0;`
- Scrollbar styling for dark mode

Create `web/src/vite-env.d.ts`

Create `web/src/lib/constants.ts`:
- Status color map
- Sidebar widths
- Polling intervals

Create `web/src/lib/api.ts`:
- `apiFetch<T>(path, options)` function
- Reads VITE_API_BASE_URL
- Attaches Bearer token from localStorage
- Handles 401 → redirect to /login
- Custom ApiError class

Create `web/src/lib/echarts-setup.ts`:
- Tree-shaken ECharts imports (LineChart, BarChart, PieChart, GaugeChart, GraphChart)
- Required components (Title, Tooltip, Grid, Legend, DataZoom, Toolbox)
- CanvasRenderer

Create `web/src/lib/echarts-theme.ts`:
- `pgpDarkTheme` object: transparent bg, #94a3b8 text, dark tooltip, dark axis lines
- `pgpLightTheme` object: transparent bg, light colors
- Color palette: [#3b82f6, #14b8a6, #f59e0b, #6366f1, #f43f5e, #06b6d4]

Create `web/src/types/models.ts`:
- `Server` interface (id, name, host, port, status, pg_version, is_primary)
- `Database` interface (name, server_id, size_bytes, cache_hit_ratio, connections)
- `Alert` interface (id, rule_slug, severity, state, message, instance_id, fired_at, resolved_at)
- `User` interface (id, username, role, email)
- `UserRole` type: 'super_admin' | 'roles_admin' | 'dba' | 'app_admin'
- `HealthResponse` interface (status, version, uptime)

Create `web/src/types/api.ts`:
- Generic API response wrapper types

**Step 3: Zustand stores**

Create `web/src/stores/themeStore.ts`:
- State: `theme` ('dark'|'light'|'system'), `resolvedTheme` ('dark'|'light')
- Actions: `setTheme(theme)`
- Persist to localStorage key 'pgp-theme'
- `initializeTheme()` function: apply on load + listen for system changes
- Default: 'dark'

Create `web/src/stores/layoutStore.ts`:
- State: `sidebarCollapsed` (boolean, default false)
- Actions: `toggleSidebar()`, `setSidebarCollapsed(boolean)`
- Persist to localStorage key 'pgp-layout'

Create `web/src/stores/authStore.ts`:
- State: `token` (string|null), `user` (User|null)
- Computed: `isAuthenticated` getter
- Actions: `setAuth(token, user)`, `clearAuth()`
- Persist to localStorage key 'pgp-auth'
- Placeholder only — populated in M5_02

**Step 4: Components**

Create `web/src/components/ui/Spinner.tsx`:
- Animated SVG spinner
- Props: `size?: 'sm' | 'md' | 'lg'`, `className?: string`
- Uses pgp-accent color

Create `web/src/components/ui/StatusBadge.tsx`:
- Props: `status: 'ok'|'warning'|'critical'|'info'|'unknown'`, `label?: string`, `pulse?: boolean`, `size?: 'sm'|'md'`
- Colored dot (rounded-full) + optional text label
- Critical + pulse: animated ping effect using Tailwind animate-ping
- Status colors: ok→pgp-ok, warning→pgp-warning, critical→pgp-critical, info→pgp-info, unknown→gray-500

Create `web/src/components/ui/MetricCard.tsx`:
- Props: `label`, `value`, `unit?`, `trend?: 'up'|'down'|'flat'`, `trendValue?`, `status?`, `sparklineData?: number[]`
- Card: bg-pgp-bg-card, rounded-lg, border border-pgp-border, p-4
- Left border colored by status (2px)
- Label: text-sm text-pgp-text-muted
- Value: text-2xl font-semibold text-pgp-text-primary + unit in text-sm
- Trend: arrow icon (TrendingUp/TrendingDown from lucide) + percentage
- Sparkline: tiny inline ECharts line chart (64x24px, no axis, no tooltip)

Create `web/src/components/ui/DataTable.tsx`:
- Generic component `DataTable<T>`
- Props: `columns`, `data`, `loading?`, `emptyMessage?`, `onRowClick?`, `sortColumn?`, `sortDirection?`, `onSort?`
- Column definition: `key`, `label`, `sortable?`, `render?`, `width?`, `align?`
- Header: sticky, bg-pgp-bg-secondary, sortable columns show chevron icons
- Rows: hover bg-pgp-bg-hover, cursor-pointer if onRowClick
- Empty state: EmptyState component
- Loading state: Spinner overlay
- Font: monospace for numeric columns (detect by column type or explicit flag)

Create `web/src/components/ui/PageHeader.tsx`:
- Props: `title`, `subtitle?`, `actions?: ReactNode`
- Layout: flex justify-between. Title left (text-2xl font-semibold), actions right.
- Subtitle below title in text-pgp-text-muted

Create `web/src/components/ui/EmptyState.tsx`:
- Props: `icon?: LucideIcon`, `title`, `description?`, `action?: ReactNode`
- Centered layout with icon (48px, muted), title, optional description, optional CTA button

Create `web/src/components/ui/ErrorBoundary.tsx`:
- React class component error boundary
- Fallback UI: "Something went wrong" with error message and retry button
- Uses pgp-critical color for error styling

Create `web/src/components/ui/ThemeToggle.tsx`:
- Three-state toggle: dark (Moon icon), light (Sun icon), system (Monitor icon)
- Cycles through: dark → light → system → dark
- Uses useThemeStore
- Icon-only button with aria-label

Create `web/src/components/ui/EChartWrapper.tsx`:
- Props: `option: EChartsOption`, `height?: string|number`, `loading?: boolean`, `className?`
- Uses echarts-for-react
- Reads resolvedTheme from useThemeStore, applies pgpDarkTheme/pgpLightTheme
- Sets backgroundColor transparent
- Responsive: width 100%

**Step 5: Layout components**

Create `web/src/components/layout/AppShell.tsx`:
- Uses Sidebar, TopBar, StatusBar, Outlet
- Flex container, full viewport height
- Content area shifts left margin based on sidebar state
- Smooth transition on sidebar collapse

Create `web/src/components/layout/Sidebar.tsx`:
- Fixed position, left-0, top-0, h-screen
- Width transitions between 240px (expanded) and 64px (collapsed)
- Background: bg-pgp-bg-secondary with right border
- Top: PGPulse logo (collapsed: just icon, expanded: icon + "PGPulse" text)
- Middle: nav items with icons from lucide-react
  - Fleet Overview (LayoutGrid icon) → /fleet
  - Alerts (Bell icon) → /alerts
  - Administration (Settings icon) → /admin
  - Active route: left-2 border-pgp-accent + bg-pgp-bg-hover
  - Collapsed: show icon only, centered, with tooltip on hover showing label
- Divider line
- Bottom: Server tree section
  - Section header: "Servers" (hidden when collapsed)
  - Mock server list (3 items): each shows StatusBadge dot + name
  - Click navigates to /servers/:id
  - Collapsed: show dots only
- z-index: 40 (above content, below modals)

Create `web/src/components/layout/TopBar.tsx`:
- Sticky top-0, h-12, bg-pgp-bg-secondary, border-b
- Left side: hamburger button (Menu icon from lucide) → toggleSidebar(), then Breadcrumb
- Right side: Search icon button (placeholder), Bell icon button (placeholder), ThemeToggle, user avatar circle (placeholder)
- z-index: 30

Create `web/src/components/layout/Breadcrumb.tsx`:
- Auto-generates breadcrumb from current route using useLocation()
- Route-to-label mapping:
  - `/fleet` → "Fleet Overview"
  - `/servers/:id` → "Servers" > "{serverId}" (from params)
  - `/servers/:id/databases/:db` → "Servers" > "{serverId}" > "{dbName}"
  - `/alerts` → "Alerts"
  - `/alerts/rules` → "Alerts" > "Rules"
  - `/admin` → "Administration"
  - `/admin/users` → "Administration" > "Users"
- Separator: ChevronRight icon (small, muted)
- Each crumb is a Link (clickable) except the last (plain text)

Create `web/src/components/layout/StatusBar.tsx`:
- Fixed bottom-0, h-8, bg-pgp-bg-secondary, border-t
- Left: connection status dot + "Connected" / "Disconnected" (from useHealthCheck)
- Right: "Last refresh: Xs ago" (from TanStack Query's dataUpdatedAt)
- Small text (text-xs), muted color
- Full width under content area

**Step 6: Hooks**

Create `web/src/hooks/useHealthCheck.ts`:
- Uses @tanstack/react-query `useQuery`
- Key: `['health']`
- Fetches `GET /api/v1/health` via `apiFetch<HealthResponse>('/health')`
- `refetchInterval: 30_000` (30s)
- Returns `{ isConnected, status, version, uptime, dataUpdatedAt }`

**Step 7: Pages (all placeholders)**

Create `web/src/pages/FleetOverview.tsx`:
- PageHeader: title "Fleet Overview"
- Row of 4 MetricCards with mock data:
  - Servers: 3 (ok), Active Alerts: 2 (warning), Avg Cache Hit: 99.2% (ok), Connections: 47/200 (ok)
- EChartWrapper with mock line chart:
  - Title: "Connections (mock data)"
  - 24 data points, random values 30-60
  - Shows dark theme integration works
- DataTable with mock server data (3 rows):
  - Columns: Name, Host:Port, PG Version, Status (StatusBadge), Connections
  - Sort by Name default
- This page is the SHOWCASE that proves all components work together

Create `web/src/pages/ServerDetail.tsx`:
- PageHeader: "Server: {serverId}" (from useParams)
- Body: "Server detail view coming in M5_03"

Create `web/src/pages/DatabaseDetail.tsx`:
- PageHeader: "Database: {dbName} on {serverId}"
- Body: "Database detail view coming in M5_04"

Create `web/src/pages/AlertsDashboard.tsx`:
- PageHeader: "Alerts"
- Body: "Alerts dashboard coming in M5_05"

Create `web/src/pages/AlertRules.tsx`:
- PageHeader: "Alert Rules"
- Body: "Alert rules management coming in M5_05"

Create `web/src/pages/Administration.tsx`:
- PageHeader: "Administration"
- Body: "Administration coming in M5_02"

Create `web/src/pages/UserManagement.tsx`:
- PageHeader: "User Management"
- Body: "User management coming in M5_02"

Create `web/src/pages/Login.tsx`:
- Full-screen centered layout
- Dark background (bg-pgp-bg-primary)
- PGPulse logo + "PGPulse" title
- Mock login form: username input, password input, login button
- Button is non-functional (M5_02)
- Subtle text: "PostgreSQL Health & Activity Monitor"

Create `web/src/pages/NotFound.tsx`:
- Centered: "404" (large), "Page Not Found" (medium)
- Link: "Return to Fleet Overview" → /fleet

**Step 8: App entry**

Create `web/src/main.tsx`:
- Import echarts-setup (side effect)
- Import index.css
- Call initializeTheme() before render
- Wrap app in: React.StrictMode > QueryClientProvider > BrowserRouter
- Include ReactQueryDevtools (dev only)
- Render App component

Create `web/src/App.tsx`:
- Route tree with AppShell as layout route
- Login route OUTSIDE AppShell
- Index redirects to /fleet
- Catch-all → NotFound

---

### SPECIALIST 2 — GO BACKEND (Embedding + CORS)

Modify the Go backend to serve the embedded frontend.

**Create `web/embed.go`:**
```go
package web

import "embed"

// DistFS contains the built frontend assets from web/dist/.
// To build: cd web && npm run build
//
//go:embed all:dist
var DistFS embed.FS
```

**IMPORTANT**: This file will only compile after `web/dist/` exists (i.e., after
`npm run build` has been run). For the initial Go build before the frontend exists,
the developer may need to create a minimal `web/dist/index.html` placeholder.

Create `web/dist/.gitkeep` as an empty file so the directory exists for Go compilation.
Actually, better approach: create a build tag or conditional embed. Simplest: create
a minimal placeholder `web/dist/index.html`:
```html
<!DOCTYPE html>
<html><body>Run npm build in web/ directory</body></html>
```

**Create `internal/api/static.go`:**
- Import `web` package for `web.DistFS`
- `func (s *APIServer) staticHandler() http.Handler`
- Use `fs.Sub(web.DistFS, "dist")` to strip prefix
- Try to serve exact file path first
- If file not found, serve `index.html` (SPA fallback)
- Set `Cache-Control: public, max-age=31536000, immutable` for `/assets/*` paths
- Set `Cache-Control: no-cache` for index.html

**Modify `internal/api/server.go`:**
- Add a catch-all route AFTER all API routes: `r.Handle("/*", s.staticHandler())`
- Add optional CORS middleware:
  - Read `CORSEnabled` and `CORSOrigin` from config
  - If enabled, add middleware that sets CORS headers
  - Only used in development (Vite dev server on different port)

**Modify `internal/config/config.go`:**
- Add to `ServerConfig`:
  - `CORSEnabled bool   \`yaml:"cors_enabled"\``
  - `CORSOrigin  string \`yaml:"cors_origin"\``

**Modify `internal/config/load.go`:**
- Default `CORSEnabled: false`
- Default `CORSOrigin: "http://localhost:5173"`

**Modify `configs/pgpulse.example.yml`:**
- Add under `server:` section:
  ```yaml
  cors_enabled: false  # Enable for development with separate Vite dev server
  cors_origin: "http://localhost:5173"
  ```

**Update `.gitignore` at project root:**
- Add: `web/node_modules/`
- Add: `web/dist/` (but NOT web/dist/.gitkeep)

---

## Coordination Notes

- Frontend Specialist creates all web/ files (React project)
- Go Specialist creates embed.go, static.go, and modifies server.go + config
- No dependency between them — they can work in parallel
- Frontend does NOT need Go running to develop (Vite dev server is standalone)
- Go does NOT need frontend built to compile (placeholder dist/index.html)

## Files List for Developer

After both specialists finish, the developer needs to run:

```bash
# 1. Install frontend dependencies
cd web && npm install

# 2. Build frontend
npm run build

# 3. Verify frontend
npm run lint
npm run typecheck

# 4. Build Go binary (now includes embedded frontend)
cd ..
go build ./cmd/pgpulse-server

# 5. Run existing tests (verify zero regressions)
go test ./...

# 6. Test manually
./pgpulse-server --config configs/pgpulse.yml
# Open http://localhost:8080/ — should see PGPulse app shell

# 7. Commit
git add .
git commit -m "feat(web): initialize React frontend with application shell (M5_01)"
git push
```
