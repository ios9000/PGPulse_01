# M5_01 Design — Frontend Scaffold & Application Shell

**Iteration:** M5_01  
**Folder:** `docs/iterations/M5_01_03022026_frontend-scaffold/`  
**Date:** 2026-03-02  
**Requirements:** M5_01_requirements.md

---

## Architecture Decisions

### [D84] Frontend framework: React 18 + TypeScript + Vite
React for complex dashboard state management, TypeScript for type safety,
Vite for fast builds and HMR. Not Vue/Svelte — React's ecosystem depth wins
for data-heavy monitoring UIs (see Datadog, Atlassian).

### [D85] Charting: Apache ECharts via echarts-for-react
ECharts has native dark theme support, rich chart types (line, bar, gauge,
heatmap, treemap, graph/force-directed for lock trees), excellent performance
with large datasets, and supports dynamic theme switching without re-init
(new in ECharts 6). Tree-shakeable.

### [D86] State: Zustand (client) + TanStack Query v5 (server)
Clean separation — Zustand holds UI state (theme, layout, auth token),
TanStack Query manages all API data (fetching, caching, polling, retries).
No Redux overhead. TanStack Query's `refetchInterval` maps directly to our
collector frequency tiers.

### [D87] Icons: Lucide React
MIT licensed, tree-shakeable, consistent style. Lighter than FontAwesome,
more icons than Heroicons. Each icon imports individually (~200B per icon).

### [D88] go:embed strategy — embed package in web/ directory
The `//go:embed` directive must be in a Go file that's at the same level as
or above the embedded directory. We create `web/embed.go` that exports the
filesystem, then import it from `internal/api/static.go`. This avoids
path-resolution issues with the Go module root.

### [D89] CSS strategy: Tailwind utility classes only
No CSS modules, no styled-components, no CSS-in-JS. Tailwind utilities +
custom theme tokens. Components compose utilities directly. This keeps the
bundle small and avoids runtime CSS overhead.

---

## File Tree (new files this iteration creates)

```
web/                                    ← NEW: entire frontend project
├── embed.go                            ← Go embed directive for dist/
├── package.json
├── package-lock.json
├── tsconfig.json
├── tsconfig.node.json
├── vite.config.ts
├── tailwind.config.ts
├── postcss.config.js
├── .eslintrc.cjs
├── .prettierrc
├── .env.development
├── .env.production
├── index.html                          ← Vite entry HTML
├── public/
│   └── favicon.svg                     ← PGPulse icon (simple DB pulse SVG)
├── src/
│   ├── main.tsx                        ← React root: providers, router
│   ├── App.tsx                         ← Top-level route config
│   ├── index.css                       ← Tailwind directives + global styles
│   ├── vite-env.d.ts                   ← Vite type declarations
│   │
│   ├── components/
│   │   ├── layout/
│   │   │   ├── AppShell.tsx            ← Shell: sidebar + topbar + content
│   │   │   ├── Sidebar.tsx             ← Collapsible sidebar
│   │   │   ├── TopBar.tsx              ← Top navigation bar
│   │   │   ├── Breadcrumb.tsx          ← Dynamic breadcrumb from route
│   │   │   └── StatusBar.tsx           ← Bottom status bar (connection, refresh)
│   │   │
│   │   └── ui/
│   │       ├── StatusBadge.tsx
│   │       ├── MetricCard.tsx
│   │       ├── DataTable.tsx
│   │       ├── PageHeader.tsx
│   │       ├── Spinner.tsx
│   │       ├── EmptyState.tsx
│   │       ├── ErrorBoundary.tsx
│   │       ├── ThemeToggle.tsx
│   │       └── EChartWrapper.tsx       ← ECharts integration wrapper
│   │
│   ├── pages/
│   │   ├── FleetOverview.tsx           ← Placeholder + demo chart
│   │   ├── ServerDetail.tsx            ← Placeholder
│   │   ├── DatabaseDetail.tsx          ← Placeholder
│   │   ├── AlertsDashboard.tsx         ← Placeholder
│   │   ├── AlertRules.tsx              ← Placeholder
│   │   ├── Administration.tsx          ← Placeholder
│   │   ├── UserManagement.tsx          ← Placeholder
│   │   ├── Login.tsx                   ← Placeholder (renders outside shell)
│   │   └── NotFound.tsx                ← 404 page
│   │
│   ├── hooks/
│   │   └── useHealthCheck.ts           ← Example TanStack Query hook
│   │
│   ├── stores/
│   │   ├── themeStore.ts               ← Zustand: dark/light/system
│   │   ├── layoutStore.ts              ← Zustand: sidebar state
│   │   └── authStore.ts                ← Zustand: JWT token + user (placeholder)
│   │
│   ├── lib/
│   │   ├── api.ts                      ← Fetch wrapper with auth headers
│   │   ├── echarts-theme.ts            ← PGPulse ECharts dark/light themes
│   │   └── constants.ts                ← Shared constants (colors, breakpoints)
│   │
│   └── types/
│       ├── api.ts                      ← API response types
│       └── models.ts                   ← Domain model types (Server, Database, Alert, User)
│
├── dist/                               ← Vite build output (gitignored)
│   └── ...
│
└── .gitignore                          ← node_modules/, dist/

internal/api/
├── static.go                           ← NEW: embedded SPA file server
└── server.go                           ← MODIFIED: mount static handler, CORS
```

---

## Detailed Component Designs

### 1. Go Embedding — `web/embed.go`

```go
package web

import "embed"

// DistFS contains the built frontend assets.
// Build the frontend first: cd web && npm run build
//
//go:embed all:dist
var DistFS embed.FS
```

Key: using `all:dist` includes files starting with `.` and `_` (like `_assets`).

### 2. Static File Server — `internal/api/static.go`

```go
package api

import (
    "io/fs"
    "net/http"
    "strings"

    "github.com/ios9000/PGPulse_01/web"
)

// staticHandler serves the embedded SPA frontend.
// API routes (/api/) are handled separately and take priority.
// All other routes serve the SPA with index.html fallback.
func (s *APIServer) staticHandler() http.Handler {
    // Strip "dist" prefix from embedded filesystem
    distFS, err := fs.Sub(web.DistFS, "dist")
    if err != nil {
        panic("failed to create sub filesystem: " + err.Error())
    }

    fileServer := http.FileServer(http.FS(distFS))

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Path

        // Try to serve the exact file
        if path != "/" && !strings.HasSuffix(path, "/") {
            // Check if file exists in embedded FS
            if f, err := distFS.Open(strings.TrimPrefix(path, "/")); err == nil {
                f.Close()
                // Set cache headers for hashed assets
                if strings.Contains(path, "/assets/") {
                    w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
                }
                fileServer.ServeHTTP(w, r)
                return
            }
        }

        // SPA fallback: serve index.html for all non-file routes
        // This lets React Router handle client-side routing
        w.Header().Set("Cache-Control", "no-cache")
        r.URL.Path = "/"
        fileServer.ServeHTTP(w, r)
    })
}
```

Mount in `server.go`:
```go
// After all /api/v1/* routes are registered:
r.Handle("/*", s.staticHandler())
```

### 3. CORS Middleware (dev mode)

Add to `internal/api/server.go`, enabled only when config flag is set:

```go
func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        w.Header().Set("Access-Control-Allow-Credentials", "true")
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

Config addition to `configs/pgpulse.example.yml`:
```yaml
server:
  cors_enabled: false  # Enable for development with separate Vite dev server
  cors_origin: "http://localhost:5173"
```

### 4. Tailwind Configuration — `web/tailwind.config.ts`

```typescript
import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        pgp: {
          // Backgrounds
          'bg-primary': 'var(--pgp-bg-primary)',
          'bg-secondary': 'var(--pgp-bg-secondary)',
          'bg-card': 'var(--pgp-bg-card)',
          'bg-hover': 'var(--pgp-bg-hover)',
          'border': 'var(--pgp-border)',
          // Accent
          'accent': 'var(--pgp-accent)',
          'accent-hover': 'var(--pgp-accent-hover)',
          // Status
          'ok': 'var(--pgp-ok)',
          'warning': 'var(--pgp-warning)',
          'critical': 'var(--pgp-critical)',
          'info': 'var(--pgp-info)',
          // Text
          'text-primary': 'var(--pgp-text-primary)',
          'text-secondary': 'var(--pgp-text-secondary)',
          'text-muted': 'var(--pgp-text-muted)',
        },
      },
      width: {
        'sidebar': '240px',
        'sidebar-collapsed': '64px',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
    },
  },
  plugins: [],
} satisfies Config
```

CSS variables defined in `index.css`:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    /* Light mode */
    --pgp-bg-primary: #ffffff;
    --pgp-bg-secondary: #f8fafc;
    --pgp-bg-card: #ffffff;
    --pgp-bg-hover: #f1f5f9;
    --pgp-border: #e2e8f0;
    --pgp-accent: #3b82f6;
    --pgp-accent-hover: #2563eb;
    --pgp-ok: #14b8a6;
    --pgp-warning: #f59e0b;
    --pgp-critical: #ef4444;
    --pgp-info: #6366f1;
    --pgp-text-primary: #0f172a;
    --pgp-text-secondary: #475569;
    --pgp-text-muted: #94a3b8;
  }

  .dark {
    --pgp-bg-primary: #0f1117;
    --pgp-bg-secondary: #1a1d23;
    --pgp-bg-card: #21252b;
    --pgp-bg-hover: #2a2f38;
    --pgp-border: #2e3440;
    --pgp-accent: #3b82f6;
    --pgp-accent-hover: #60a5fa;
    --pgp-ok: #14b8a6;
    --pgp-warning: #f59e0b;
    --pgp-critical: #ef4444;
    --pgp-info: #818cf8;
    --pgp-text-primary: #f8fafc;
    --pgp-text-secondary: #94a3b8;
    --pgp-text-muted: #64748b;
  }
}
```

### 5. ECharts Dark Theme — `web/src/lib/echarts-theme.ts`

```typescript
import type { EChartsOption } from 'echarts'

export const pgpDarkTheme = {
  backgroundColor: 'transparent',
  textStyle: { color: '#94a3b8' },
  title: { textStyle: { color: '#f8fafc' }, subtextStyle: { color: '#64748b' } },
  legend: { textStyle: { color: '#94a3b8' } },
  tooltip: {
    backgroundColor: '#21252b',
    borderColor: '#2e3440',
    textStyle: { color: '#f8fafc' },
  },
  xAxis: {
    axisLine: { lineStyle: { color: '#2e3440' } },
    axisTick: { lineStyle: { color: '#2e3440' } },
    axisLabel: { color: '#64748b' },
    splitLine: { lineStyle: { color: '#1e2128' } },
  },
  yAxis: {
    axisLine: { lineStyle: { color: '#2e3440' } },
    axisTick: { lineStyle: { color: '#2e3440' } },
    axisLabel: { color: '#64748b' },
    splitLine: { lineStyle: { color: '#1e2128' } },
  },
  // Color palette: accent blue, teal, amber, indigo, rose, cyan
  color: ['#3b82f6', '#14b8a6', '#f59e0b', '#6366f1', '#f43f5e', '#06b6d4'],
}

export const pgpLightTheme = {
  backgroundColor: 'transparent',
  textStyle: { color: '#475569' },
  title: { textStyle: { color: '#0f172a' }, subtextStyle: { color: '#94a3b8' } },
  legend: { textStyle: { color: '#475569' } },
  tooltip: {
    backgroundColor: '#ffffff',
    borderColor: '#e2e8f0',
    textStyle: { color: '#0f172a' },
  },
  xAxis: {
    axisLine: { lineStyle: { color: '#e2e8f0' } },
    axisTick: { lineStyle: { color: '#e2e8f0' } },
    axisLabel: { color: '#94a3b8' },
    splitLine: { lineStyle: { color: '#f1f5f9' } },
  },
  yAxis: {
    axisLine: { lineStyle: { color: '#e2e8f0' } },
    axisTick: { lineStyle: { color: '#e2e8f0' } },
    axisLabel: { color: '#94a3b8' },
    splitLine: { lineStyle: { color: '#f1f5f9' } },
  },
  color: ['#3b82f6', '#14b8a6', '#f59e0b', '#6366f1', '#f43f5e', '#06b6d4'],
}
```

### 6. EChartWrapper Component — `web/src/components/ui/EChartWrapper.tsx`

```typescript
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'
import { useThemeStore } from '@/stores/themeStore'
import { pgpDarkTheme, pgpLightTheme } from '@/lib/echarts-theme'

interface EChartWrapperProps {
  option: EChartsOption
  height?: string | number
  loading?: boolean
  className?: string
}

export function EChartWrapper({ option, height = 300, loading, className }: EChartWrapperProps) {
  const resolvedTheme = useThemeStore((s) => s.resolvedTheme)
  const theme = resolvedTheme === 'dark' ? pgpDarkTheme : pgpLightTheme

  return (
    <ReactECharts
      option={{ ...option, backgroundColor: 'transparent' }}
      theme={theme}
      style={{ height, width: '100%' }}
      opts={{ renderer: 'canvas' }}
      showLoading={loading}
      className={className}
      notMerge={true}
    />
  )
}
```

### 7. ECharts Tree-Shaking

To keep bundle small, import only required components in a barrel file:

```typescript
// web/src/lib/echarts-setup.ts
import * as echarts from 'echarts/core'
import { LineChart, BarChart, PieChart, GaugeChart, TreemapChart, GraphChart } from 'echarts/charts'
import {
  TitleComponent, TooltipComponent, GridComponent,
  LegendComponent, DataZoomComponent, ToolboxComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  LineChart, BarChart, PieChart, GaugeChart, TreemapChart, GraphChart,
  TitleComponent, TooltipComponent, GridComponent,
  LegendComponent, DataZoomComponent, ToolboxComponent,
  CanvasRenderer,
])

export default echarts
```

Import this once in `main.tsx` before any ECharts usage.

### 8. Theme Store — `web/src/stores/themeStore.ts`

```typescript
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type ThemeMode = 'dark' | 'light' | 'system'
type ResolvedTheme = 'dark' | 'light'

interface ThemeState {
  theme: ThemeMode
  resolvedTheme: ResolvedTheme
  setTheme: (theme: ThemeMode) => void
}

function resolveTheme(theme: ThemeMode): ResolvedTheme {
  if (theme === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return theme
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      theme: 'dark',  // dark-first default
      resolvedTheme: 'dark',
      setTheme: (theme) => {
        const resolved = resolveTheme(theme)
        // Apply class to <html>
        document.documentElement.classList.toggle('dark', resolved === 'dark')
        set({ theme, resolvedTheme: resolved })
      },
    }),
    { name: 'pgp-theme' }
  )
)

// Initialize theme on app load
export function initializeTheme() {
  const { theme, setTheme } = useThemeStore.getState()
  setTheme(theme)

  // Listen for system theme changes
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    const current = useThemeStore.getState().theme
    if (current === 'system') {
      useThemeStore.getState().setTheme('system')
    }
  })
}
```

### 9. API Client — `web/src/lib/api.ts`

```typescript
const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api/v1'

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function apiFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = localStorage.getItem('pgp-auth-token')

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  }

  if (token) {
    ;(headers as Record<string, string>)['Authorization'] = `Bearer ${token}`
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  })

  if (response.status === 401) {
    // Token expired or invalid — will be handled by auth guard in M5_02
    localStorage.removeItem('pgp-auth-token')
    window.location.href = '/login'
    throw new ApiError(401, 'Unauthorized')
  }

  if (!response.ok) {
    const body = await response.text()
    throw new ApiError(response.status, body || response.statusText)
  }

  return response.json()
}
```

### 10. App Entry Point — `web/src/main.tsx`

```typescript
import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { BrowserRouter } from 'react-router-dom'
import { App } from './App'
import { initializeTheme } from '@/stores/themeStore'
import '@/lib/echarts-setup'
import './index.css'

// Initialize theme before render to prevent flash
initializeTheme()

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: true,
      retry: 2,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryClientProvider>
  </React.StrictMode>
)
```

### 11. Route Configuration — `web/src/App.tsx`

```typescript
import { Routes, Route, Navigate } from 'react-router-dom'
import { AppShell } from '@/components/layout/AppShell'
import { FleetOverview } from '@/pages/FleetOverview'
import { ServerDetail } from '@/pages/ServerDetail'
import { DatabaseDetail } from '@/pages/DatabaseDetail'
import { AlertsDashboard } from '@/pages/AlertsDashboard'
import { AlertRules } from '@/pages/AlertRules'
import { Administration } from '@/pages/Administration'
import { UserManagement } from '@/pages/UserManagement'
import { Login } from '@/pages/Login'
import { NotFound } from '@/pages/NotFound'

export function App() {
  return (
    <Routes>
      {/* Login renders OUTSIDE the shell */}
      <Route path="/login" element={<Login />} />

      {/* All other routes render INSIDE the shell */}
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/fleet" replace />} />
        <Route path="fleet" element={<FleetOverview />} />
        <Route path="servers/:serverId" element={<ServerDetail />} />
        <Route path="servers/:serverId/databases/:dbName" element={<DatabaseDetail />} />
        <Route path="alerts" element={<AlertsDashboard />} />
        <Route path="alerts/rules" element={<AlertRules />} />
        <Route path="admin" element={<Administration />} />
        <Route path="admin/users" element={<UserManagement />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}
```

### 12. Application Shell — `web/src/components/layout/AppShell.tsx`

```typescript
import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import { StatusBar } from './StatusBar'
import { useLayoutStore } from '@/stores/layoutStore'

export function AppShell() {
  const collapsed = useLayoutStore((s) => s.sidebarCollapsed)

  return (
    <div className="flex h-screen bg-pgp-bg-primary text-pgp-text-primary">
      <Sidebar />
      <div className={`flex flex-col flex-1 transition-all duration-200 ${
        collapsed ? 'ml-[64px]' : 'ml-[240px]'
      }`}>
        <TopBar />
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
        <StatusBar />
      </div>
    </div>
  )
}
```

### 13. Sidebar Design — `web/src/components/layout/Sidebar.tsx`

Structure:
- Fixed position, left edge, full height
- Top section: PGPulse logo/brand
- Middle section: navigation items (Fleet, Alerts, Admin)
- Bottom section: server tree (placeholder in M5_01)
- Smooth width transition (240px ↔ 64px)

Navigation items:
```typescript
const navItems = [
  { label: 'Fleet Overview', icon: LayoutGrid, path: '/fleet' },
  { label: 'Alerts', icon: Bell, path: '/alerts' },
  { label: 'Administration', icon: Settings, path: '/admin' },
]
```

Active detection: match current route with `useLocation()`.
Active item gets: left border accent color + subtle background highlight.

Server tree (placeholder):
```typescript
const mockServers = [
  { id: 'prod-1', name: 'prod-primary', status: 'ok' },
  { id: 'prod-2', name: 'prod-replica', status: 'ok' },
  { id: 'stg-1', name: 'staging', status: 'warning' },
]
```

### 14. Component Specifications

#### StatusBadge
```typescript
interface StatusBadgeProps {
  status: 'ok' | 'warning' | 'critical' | 'info' | 'unknown'
  label?: string      // Optional text label
  pulse?: boolean     // Animated pulse for critical
  size?: 'sm' | 'md'
}
```
Visual: colored dot + optional text. Critical with `pulse` shows animated ping effect.

#### MetricCard
```typescript
interface MetricCardProps {
  label: string
  value: string | number
  unit?: string
  trend?: 'up' | 'down' | 'flat'
  trendValue?: string      // e.g., "+5.2%"
  status?: 'ok' | 'warning' | 'critical'
  sparklineData?: number[] // Mini chart (optional)
}
```
Visual: card with subtle border. Label top (muted), value large center (primary),
unit beside value (muted), trend arrow bottom-right, sparkline bottom.
Status colors the left border.

#### DataTable
```typescript
interface Column<T> {
  key: keyof T | string
  label: string
  sortable?: boolean
  render?: (row: T) => React.ReactNode
  width?: string
}

interface DataTableProps<T> {
  columns: Column<T>[]
  data: T[]
  loading?: boolean
  emptyMessage?: string
  onRowClick?: (row: T) => void
  sortColumn?: string
  sortDirection?: 'asc' | 'desc'
  onSort?: (column: string) => void
}
```
Visual: dark-themed table with hover rows. Header row with sort indicators.
Monospace font for numeric columns.

---

## Development Workflow

### Local Development (dual process)

Terminal 1 — Vite dev server (HMR, fast refresh):
```bash
cd web
npm install
npm run dev
# Serves on http://localhost:5173
```

Terminal 2 — Go API server:
```bash
go run ./cmd/pgpulse-server --config configs/pgpulse.yml
# API on http://localhost:8080
```

Vite proxies API calls to Go in development via `vite.config.ts`:
```typescript
export default defineConfig({
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
```

This is PREFERRED over CORS middleware because it avoids configuration complexity.
The CORS middleware in Go is a fallback if proxy doesn't work in someone's setup.

### Production Build
```bash
cd web && npm run build    # → web/dist/
cd .. && go build ./cmd/pgpulse-server  # Embeds web/dist/ into binary
./pgpulse-server           # Serves both API and frontend on :8080
```

---

## .gitignore additions

Add to `web/.gitignore`:
```
node_modules/
dist/
*.local
```

Add to root `.gitignore`:
```
web/node_modules/
web/dist/
```

---

## Vite Configuration — `web/vite.config.ts`

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: {
          'echarts': ['echarts'],
          'react-vendor': ['react', 'react-dom', 'react-router-dom'],
          'query': ['@tanstack/react-query'],
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
```

Manual chunks keep ECharts in a separate lazy-loaded bundle — only fetched when
a page with charts is loaded.

---

## Type Definitions — `web/src/types/models.ts`

```typescript
// Domain models used across the frontend.
// Expanded in subsequent iterations as API endpoints are integrated.

export interface Server {
  id: string
  name: string
  host: string
  port: number
  status: 'online' | 'offline' | 'degraded' | 'unknown'
  pg_version?: string
  is_primary?: boolean
}

export interface Database {
  name: string
  server_id: string
  size_bytes: number
  cache_hit_ratio?: number
  connections: number
}

export type AlertSeverity = 'critical' | 'warning' | 'info'
export type AlertState = 'firing' | 'pending' | 'resolved'

export interface Alert {
  id: string
  rule_slug: string
  severity: AlertSeverity
  state: AlertState
  message: string
  instance_id: string
  fired_at: string
  resolved_at?: string
}

export type UserRole = 'super_admin' | 'roles_admin' | 'dba' | 'app_admin'

export interface User {
  id: string
  username: string
  role: UserRole
  email?: string
}

export interface HealthResponse {
  status: string
  version: string
  uptime: string
}
```

---

## Go-Side Changes Summary

### Modified: `internal/api/server.go`

1. Add `web.DistFS` import
2. Add static handler mount as wildcard catch-all AFTER all API routes
3. Add optional CORS middleware (disabled by default)

### Modified: `internal/config/config.go`

1. Add `CORSEnabled bool` and `CORSOrigin string` to ServerConfig

### New: `internal/api/static.go`

1. `staticHandler()` method on APIServer
2. SPA fallback logic (non-API routes → index.html)
3. Cache-Control headers for hashed assets

### New: `web/embed.go`

1. Single `//go:embed all:dist` directive exporting `DistFS`

---

## Demo Content for Fleet Overview Placeholder

The FleetOverview page demonstrates the component library works:

1. **PageHeader**: "Fleet Overview" with subtitle "Coming in M5_03"
2. **MetricCard row** (4 cards with mock data):
   - Servers: 3 (status: ok)
   - Active Alerts: 2 (status: warning)
   - Avg Cache Hit: 99.2% (status: ok)
   - Connections: 47/200 (status: ok)
3. **EChartWrapper**: Line chart "Mock Connections Over Time"
   - X axis: last 24 data points (mock timestamps)
   - Y axis: random values 30-60
   - Shows dark theme working
4. **DataTable**: Mock server list (3 rows)
   - Columns: Name, Host, Port, PG Version, Status, Connections
   - Demonstrates sort, hover, StatusBadge inside cells
5. **StatusBar**: Shows health check result from `useHealthCheck()` hook

---

## Verification Checklist

After implementation, verify:

- [ ] `cd web && npm run build` succeeds, output in `web/dist/`
- [ ] `cd web && npm run lint` — zero errors
- [ ] `cd web && npm run typecheck` — zero errors
- [ ] `go build ./cmd/pgpulse-server` — compiles with embedded frontend
- [ ] `go test ./...` — all existing tests still pass (zero regressions)
- [ ] Navigate to `http://localhost:8080/` — app shell renders
- [ ] Sidebar collapses/expands, state persists on refresh
- [ ] Dark/light toggle works, persists on refresh
- [ ] React Router navigation between placeholder pages works
- [ ] ECharts demo chart renders in both themes
- [ ] 404 page renders for unknown routes
- [ ] `/login` renders without shell
- [ ] Bundle size check: `ls -la web/dist/assets/*.js | awk '{sum+=$5} END {print sum/1024 "KB"}'`
